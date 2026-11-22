package sqlite

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// lockClarificationAnswerTask serializes answer claims with turn creation by
// taking the session turn lock before the task row. It is a no-op when the
// caller does not provide attachment scope.
func (r *Repository) lockClarificationAnswerTask(
	ctx context.Context,
	tx *sqlx.Tx,
	plan *clarificationAttachmentPlan,
) error {
	if plan == nil || plan.scope.TaskID == "" {
		return nil
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE tasks SET updated_at = updated_at WHERE id = ?
	`), plan.scope.TaskID)
	if err != nil {
		return fmt.Errorf("lock clarification task %q: %w", plan.scope.TaskID, err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("%w: %s", ErrTaskNotFound, plan.scope.TaskID)
	}
	return nil
}
