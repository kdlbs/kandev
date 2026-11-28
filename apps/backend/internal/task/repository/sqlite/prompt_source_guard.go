package sqlite

import (
	"context"
	"database/sql"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) ensurePromptSourceMutable(ctx context.Context, tx *sqlx.Tx, id string) error {
	query := `SELECT source FROM workflows WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += ` FOR UPDATE`
	}
	var source string
	err := tx.QueryRowContext(ctx, tx.Rebind(query), id).Scan(&source)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if source == models.WorkflowSourcePromptSource {
		return models.ErrPromptSourceLocked
	}
	return nil
}
