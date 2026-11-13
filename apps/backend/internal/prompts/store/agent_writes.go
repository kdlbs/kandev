package store

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/prompts/models"
)

var ErrPromptWriteRejected = errors.New("shared prompt changed; read it again before saving")

func (r *sqliteRepository) UpdatePromptForAgent(ctx context.Context, prompt *models.Prompt, expectedName string) error {
	updatedAt := time.Now().UTC()
	// Identity, built-in status, and the observed revision must still match.
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
  UPDATE custom_prompts SET name = ?, content = ?, updated_at = ?
  WHERE id = ? AND name = ? AND builtin = ? AND updated_at = ?
 `), prompt.Name, prompt.Content, updatedAt, prompt.ID, expectedName, 0, prompt.UpdatedAt)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrPromptWriteRejected
	}
	prompt.UpdatedAt = updatedAt
	return nil
}
