package store

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
)

// Book é um quadrinho. ID é o hash do caminho normalizado (calculado fora do
// store). LastSeen é o token do último scan que viu o arquivo.
type Book struct {
	ID, LibraryID, Path, Format, Title, Series, Number, BrokenReason string
	Size, MTime, AddedAt, LastSeen                                   int64
	PageCount                                                        int
	Missing, Broken                                                  bool
}

// Progress é o progresso de leitura de um livro. FurthestPage é a página mais
// distante já vista e nunca diminui.
type Progress struct {
	BookID                    string
	CurrentPage, FurthestPage int
	UpdatedAt                 int64
}

// BookWithProgress é um livro com o progresso (nil se nunca lido).
type BookWithProgress struct {
	Book
	Progress *Progress
}

// Fingerprint identifica a versão de um arquivo no disco.
type Fingerprint struct {
	Size, MTime int64
}

// Fingerprints devolve tamanho e mtime dos livros da raiz, indexados pelo ID
// do livro. O scanner os usa para pular arquivos inalterados.
func (s *Store) Fingerprints(ctx context.Context, rootID string) (map[string]Fingerprint, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT b.id, b.size, b.mtime FROM books b
JOIN libraries l ON l.id = b.library_id
WHERE l.root_id = ?`, rootID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]Fingerprint)
	for rows.Next() {
		var id string
		var f Fingerprint
		if err := rows.Scan(&id, &f.Size, &f.MTime); err != nil {
			return nil, err
		}
		out[id] = f
	}
	return out, rows.Err()
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// UpsertBooks grava os livros numa transação (tudo ou nada), por ID. Um
// reupsert atualiza todos os campos exceto added_at (e o progresso fica
// intacto), grava last_seen e zera missing.
func (s *Store) UpsertBooks(ctx context.Context, books []Book) error {
	if len(books) == 0 {
		return nil
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `
INSERT INTO books (id, library_id, path, size, mtime, format, page_count, title, series, number,
                   added_at, last_seen, missing, broken, broken_reason)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  library_id = excluded.library_id, path = excluded.path, size = excluded.size,
  mtime = excluded.mtime, format = excluded.format, page_count = excluded.page_count,
  title = excluded.title, series = excluded.series, number = excluded.number,
  last_seen = excluded.last_seen, missing = 0,
  broken = excluded.broken, broken_reason = excluded.broken_reason`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, b := range books {
			if _, err := stmt.ExecContext(ctx, b.ID, b.LibraryID, b.Path, b.Size, b.MTime, b.Format,
				b.PageCount, b.Title, b.Series, b.Number, b.AddedAt, b.LastSeen,
				b2i(b.Broken), b.BrokenReason); err != nil {
				return err
			}
		}
		return nil
	})
}

// TouchBooks marca os livros como vistos no scan de token seen (atualiza
// last_seen e zera missing), sem reabrir os arquivos. IDs desconhecidos são
// ignorados.
func (s *Store) TouchBooks(ctx context.Context, ids []string, seen int64) error {
	if len(ids) == 0 {
		return nil
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, "UPDATE books SET last_seen = ?, missing = 0 WHERE id = ?")
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, id := range ids {
			if _, err := stmt.ExecContext(ctx, seen, id); err != nil {
				return err
			}
		}
		return nil
	})
}

// MarkMissing marca missing = 1 nos livros da raiz (qualquer sub-biblioteca)
// com last_seen < token que ainda não estavam marcados, e devolve quantos
// passaram a ausentes agora. Livros de outras raízes não são tocados.
func (s *Store) MarkMissing(ctx context.Context, rootID string, token int64) (int, error) {
	var n int
	err := s.write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
UPDATE books SET missing = 1
WHERE missing = 0 AND last_seen < ?
  AND library_id IN (SELECT id FROM libraries WHERE root_id = ?)`, token, rootID)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		n = int(affected)
		return err
	})
	if err != nil {
		return 0, err
	}
	return n, nil
}

const bookSelect = `
SELECT b.id, b.library_id, b.path, b.format, b.title, b.series, b.number, b.broken_reason,
       b.size, b.mtime, b.added_at, b.last_seen, b.page_count, b.missing, b.broken,
       p.book_id, p.current_page, p.furthest_page, p.updated_at
FROM books b LEFT JOIN reading_progress p ON p.book_id = b.id`

func scanBook(sc interface{ Scan(...any) error }) (BookWithProgress, error) {
	var (
		bp                  BookWithProgress
		missing, broken     int
		pID                 sql.NullString
		pCur, pFar, pUpdate sql.NullInt64
	)
	b := &bp.Book
	err := sc.Scan(&b.ID, &b.LibraryID, &b.Path, &b.Format, &b.Title, &b.Series, &b.Number, &b.BrokenReason,
		&b.Size, &b.MTime, &b.AddedAt, &b.LastSeen, &b.PageCount, &missing, &broken,
		&pID, &pCur, &pFar, &pUpdate)
	if err != nil {
		return BookWithProgress{}, err
	}
	b.Missing, b.Broken = missing != 0, broken != 0
	if pID.Valid {
		bp.Progress = &Progress{BookID: pID.String, CurrentPage: int(pCur.Int64), FurthestPage: int(pFar.Int64), UpdatedAt: pUpdate.Int64}
	}
	return bp, nil
}

// fileKey é a chave de ordenação: nome do arquivo (último segmento do
// caminho, aceitando "/" e "\").
func fileKey(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		path = path[i+1:]
	}
	return path
}

// ListBooks devolve os livros que estão diretamente em libraryID (não os das
// sub-bibliotecas), em ordem natural pelo nome do arquivo, com o progresso.
func (s *Store) ListBooks(ctx context.Context, libraryID string) ([]BookWithProgress, error) {
	rows, err := s.db.QueryContext(ctx, bookSelect+" WHERE b.library_id = ?", libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BookWithProgress
	for rows.Next() {
		bp, err := scanBook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, bp)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if c := naturalCompare(fileKey(a.Path), fileKey(b.Path)); c != 0 {
			return c < 0
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.ID < b.ID
	})
	return out, nil
}

// GetBook devolve o livro com o progresso; ok é false se não existir.
func (s *Store) GetBook(ctx context.Context, id string) (BookWithProgress, bool, error) {
	bp, err := scanBook(s.db.QueryRowContext(ctx, bookSelect+" WHERE b.id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return BookWithProgress{}, false, nil
	}
	if err != nil {
		return BookWithProgress{}, false, err
	}
	return bp, true, nil
}
