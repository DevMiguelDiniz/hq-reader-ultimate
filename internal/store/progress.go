package store

import (
	"context"
	"database/sql"
	"errors"
)

// SaveProgress grava a página atual do livro e avança a página mais distante
// (furthest_page = max(existente, furthest), nunca diminui). Livro
// inexistente devolve ErrNotFound. É um "lê e depois escreve", por isso roda
// sob o mutex de escrita do Store.
func (s *Store) SaveProgress(ctx context.Context, bookID string, current, furthest int, at int64) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		var one int
		err := tx.QueryRowContext(ctx, "SELECT 1 FROM books WHERE id = ?", bookID).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO reading_progress (book_id, current_page, furthest_page, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(book_id) DO UPDATE SET
  current_page  = excluded.current_page,
  furthest_page = max(furthest_page, excluded.furthest_page),
  updated_at    = excluded.updated_at`, bookID, current, furthest, at)
		return err
	})
}
