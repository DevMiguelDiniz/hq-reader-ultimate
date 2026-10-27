package store

import (
	"context"
	"database/sql"
	"fmt"
)

// migrations[i] leva o esquema da versão i para i+1. Cada script roda numa
// transação junto com a atualização do PRAGMA user_version (que é
// transacional), então uma migração aplica por inteiro ou não aplica.
//
// Nunca edite um script já lançado: acrescente outro e suba SchemaVersion.
var migrations = []string{
	// 1: bibliotecas em árvore, livros, progresso.
	`
CREATE TABLE libraries (
  id         TEXT PRIMARY KEY,
  path       TEXT NOT NULL UNIQUE,
  name       TEXT NOT NULL,
  parent_id  TEXT REFERENCES libraries(id) ON DELETE CASCADE, -- NULL = raiz
  root_id    TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE, -- raiz: o próprio id
  added_at   INTEGER NOT NULL,
  scanned_at INTEGER NOT NULL DEFAULT 0 -- 0 = nenhum scan concluído
);
CREATE INDEX idx_libraries_parent ON libraries(parent_id);
CREATE INDEX idx_libraries_root   ON libraries(root_id);

CREATE TABLE books (
  id            TEXT PRIMARY KEY, -- hash do caminho normalizado
  library_id    TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
  path          TEXT NOT NULL UNIQUE,
  size          INTEGER NOT NULL,
  mtime         INTEGER NOT NULL,
  format        TEXT NOT NULL, -- 'cbz' | 'cbr'
  page_count    INTEGER NOT NULL DEFAULT 0,
  title         TEXT NOT NULL DEFAULT '',
  series        TEXT NOT NULL DEFAULT '',
  number        TEXT NOT NULL DEFAULT '',
  added_at      INTEGER NOT NULL,
  last_seen     INTEGER NOT NULL DEFAULT 0, -- token do último scan que viu o arquivo
  missing       INTEGER NOT NULL DEFAULT 0,
  broken        INTEGER NOT NULL DEFAULT 0,
  broken_reason TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_books_library ON books(library_id);

CREATE TABLE reading_progress (
  book_id       TEXT PRIMARY KEY REFERENCES books(id) ON DELETE CASCADE,
  current_page  INTEGER NOT NULL DEFAULT 0,
  furthest_page INTEGER NOT NULL DEFAULT 0,
  updated_at    INTEGER NOT NULL
);
`,
}

func init() {
	if len(migrations) != SchemaVersion {
		panic(fmt.Sprintf("store: %d migrações para SchemaVersion %d", len(migrations), SchemaVersion))
	}
}

// migrate leva o banco da versão atual (user_version) até SchemaVersion.
func (s *Store) migrate(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var cur int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&cur); err != nil {
		return fmt.Errorf("store: ler versão do esquema: %w", err)
	}
	if cur > SchemaVersion {
		return fmt.Errorf("%w (esquema %d, este app entende até %d)", ErrSchemaTooNew, cur, SchemaVersion)
	}
	for v := cur; v < SchemaVersion; v++ {
		script := migrations[v]
		err := s.writeLocked(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, script); err != nil {
				return err
			}
			// PRAGMA não aceita parâmetros; o valor é um inteiro nosso.
			_, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", v+1))
			return err
		})
		if err != nil {
			return fmt.Errorf("store: migração %d -> %d: %w", v, v+1, err)
		}
	}
	return nil
}
