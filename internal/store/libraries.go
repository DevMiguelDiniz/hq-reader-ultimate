package store

import (
	"context"
	"database/sql"
	"errors"
	"sort"
)

// Library é uma pasta da biblioteca. Uma raiz tem ParentID == "" e
// RootID == ID; sub-bibliotecas apontam para o pai e para a raiz.
type Library struct {
	ID, Path, Name, ParentID, RootID string
	AddedAt                          int64
	// ScannedAt é o instante do último scan concluído da raiz (0 = nunca).
	ScannedAt int64
}

// Stats resume uma biblioteca: livros (não ausentes) na subárvore inteira e
// pastas filhas diretas.
type Stats struct {
	Books   int
	Folders int
}

const libraryColumns = "id, path, name, COALESCE(parent_id, ''), root_id, added_at, scanned_at"

func scanLibrary(sc interface{ Scan(...any) error }) (Library, error) {
	var l Library
	err := sc.Scan(&l.ID, &l.Path, &l.Name, &l.ParentID, &l.RootID, &l.AddedAt, &l.ScannedAt)
	return l, err
}

// nullIfEmpty grava "" como NULL (raiz).
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// UpsertLibraries grava as bibliotecas numa transação. Os pais precisam vir
// antes dos filhos na lista (a chave estrangeira é verificada na hora). Um
// reupsert atualiza nome, caminho, pai e raiz, mas nunca added_at nem
// scanned_at.
func (s *Store) UpsertLibraries(ctx context.Context, libs []Library) error {
	if len(libs) == 0 {
		return nil
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `
INSERT INTO libraries (id, path, name, parent_id, root_id, added_at, scanned_at)
VALUES (?, ?, ?, ?, ?, ?, 0)
ON CONFLICT(id) DO UPDATE SET
  path = excluded.path, name = excluded.name,
  parent_id = excluded.parent_id, root_id = excluded.root_id`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, l := range libs {
			if _, err := stmt.ExecContext(ctx, l.ID, l.Path, l.Name, nullIfEmpty(l.ParentID), l.RootID, l.AddedAt); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetLibrary devolve a biblioteca; ok é false se não existir.
func (s *Store) GetLibrary(ctx context.Context, id string) (Library, bool, error) {
	l, err := scanLibrary(s.db.QueryRowContext(ctx, "SELECT "+libraryColumns+" FROM libraries WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Library{}, false, nil
	}
	if err != nil {
		return Library{}, false, err
	}
	return l, true, nil
}

func (s *Store) queryLibraries(ctx context.Context, query string, args ...any) ([]Library, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Library
	for rows.Next() {
		l, err := scanLibrary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// sortLibraries ordena por nome em ordem natural, sem diferenciar maiúsculas
// (desempate por caminho e id, para ser determinístico).
func sortLibraries(libs []Library) {
	sort.SliceStable(libs, func(i, j int) bool {
		a, b := libs[i], libs[j]
		if c := naturalCompare(a.Name, b.Name); c != 0 {
			return c < 0
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.ID < b.ID
	})
}

// ListRoots devolve as bibliotecas raiz em ordem natural por nome.
func (s *Store) ListRoots(ctx context.Context) ([]Library, error) {
	libs, err := s.queryLibraries(ctx, "SELECT "+libraryColumns+" FROM libraries WHERE parent_id IS NULL")
	if err != nil {
		return nil, err
	}
	sortLibraries(libs)
	return libs, nil
}

// ListChildren devolve as sub-bibliotecas diretas de parentID em ordem natural
// por nome.
func (s *Store) ListChildren(ctx context.Context, parentID string) ([]Library, error) {
	libs, err := s.queryLibraries(ctx, "SELECT "+libraryColumns+" FROM libraries WHERE parent_id = ?", parentID)
	if err != nil {
		return nil, err
	}
	sortLibraries(libs)
	return libs, nil
}

// Ancestors devolve o caminho da raiz até a própria biblioteca (raiz primeiro,
// id por último). Id desconhecido devolve lista vazia.
func (s *Store) Ancestors(ctx context.Context, id string) ([]Library, error) {
	return s.queryLibraries(ctx, `
WITH RECURSIVE up(id, path, name, parent_id, root_id, added_at, scanned_at, depth) AS (
  SELECT id, path, name, parent_id, root_id, added_at, scanned_at, 0 FROM libraries WHERE id = ?
  UNION ALL
  SELECT l.id, l.path, l.name, l.parent_id, l.root_id, l.added_at, l.scanned_at, up.depth + 1
  FROM libraries l JOIN up ON l.id = up.parent_id
)
SELECT id, path, name, COALESCE(parent_id, ''), root_id, added_at, scanned_at
FROM up ORDER BY depth DESC`, id)
}

// DeleteRoot apaga a raiz e, em cascata, suas sub-bibliotecas, livros e
// progresso. Só atua em raízes (um id de sub-biblioteca é ignorado) e não é
// erro a raiz não existir. Nunca toca nos arquivos do disco.
func (s *Store) DeleteRoot(ctx context.Context, rootID string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM libraries WHERE id = ? AND parent_id IS NULL", rootID)
		return err
	})
}

// MarkScanned registra a conclusão de um scan da raiz. ErrNotFound se a raiz
// não existe (por exemplo, foi removida durante o scan).
func (s *Store) MarkScanned(ctx context.Context, rootID string, at int64) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "UPDATE libraries SET scanned_at = ? WHERE id = ? AND parent_id IS NULL", at, rootID)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// Stats conta, na subárvore de libraryID, os livros não ausentes e, entre os
// filhos diretos, as pastas. Biblioteca desconhecida devolve zeros.
func (s *Store) Stats(ctx context.Context, libraryID string) (Stats, error) {
	var st Stats
	err := s.db.QueryRowContext(ctx, `
WITH RECURSIVE sub(id) AS (
  SELECT id FROM libraries WHERE id = ?1
  UNION ALL
  SELECT l.id FROM libraries l JOIN sub ON l.parent_id = sub.id
)
SELECT
  (SELECT COUNT(*) FROM books WHERE missing = 0 AND library_id IN (SELECT id FROM sub)),
  (SELECT COUNT(*) FROM libraries WHERE parent_id = ?1)`, libraryID).Scan(&st.Books, &st.Folders)
	return st, err
}
