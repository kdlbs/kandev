package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

const asyncQuestionFeatureDisabledReason = "feature_disabled"

// SettleAsyncQuestionWaitsForStartup cancels active waits when the installation
// gate is off. It keeps the original wait reason and records the cancellation
// cause in released_by, matching other wait cancellation paths.
func (r *Repository) SettleAsyncQuestionWaitsForStartup(ctx context.Context, asyncQuestionsEnabled bool) error {
	if asyncQuestionsEnabled {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin async question wait settlement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_async_question_waits
		   SET status = 'cancelled', released_by = ?, updated_at = ?
		 WHERE status IN ('requested', 'parked', 'release_requested')`),
		asyncQuestionFeatureDisabledReason, r.nowUTC()); err != nil {
		return fmt.Errorf("cancel async question waits: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit async question wait settlement: %w", err)
	}
	return nil
}

// ListPendingAsyncQuestionDeliveryIdentities returns only retryable prompt
// identities. Backoff timestamps do not affect rollback selection.
func (r *Repository) ListPendingAsyncQuestionDeliveryIdentities(
	ctx context.Context,
) ([]models.AsyncQuestionDeliveryIdentity, error) {
	rows, err := r.db.QueryxContext(ctx, r.db.Rebind(`
		SELECT admission_id, question_id
		  FROM task_async_question_deliveries
		 WHERE state = 'pending' AND kind <> ?
		 ORDER BY created_at ASC, admission_id ASC`), string(models.AsyncQuestionDeliveryTranscript))
	if err != nil {
		return nil, fmt.Errorf("list pending async question delivery identities: %w", err)
	}
	defer func() { _ = rows.Close() }()
	identities := make([]models.AsyncQuestionDeliveryIdentity, 0)
	for rows.Next() {
		var identity models.AsyncQuestionDeliveryIdentity
		if err := rows.Scan(&identity.AdmissionID, &identity.QuestionID); err != nil {
			return nil, fmt.Errorf("scan pending async question delivery identity: %w", err)
		}
		identities = append(identities, identity)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read pending async question delivery identities: %w", err)
	}
	return identities, nil
}

// MarkAsyncQuestionDeliveryRetired records the rollback before the queue row
// is removed. It only applies to pending or admitted prompt intents.
func (r *Repository) MarkAsyncQuestionDeliveryRetired(
	ctx context.Context,
	identity models.AsyncQuestionDeliveryIdentity,
	admittedAt *time.Time,
) error {
	query := `
		UPDATE task_async_question_deliveries
		   SET state = 'abandoned', last_error = ?
		 WHERE admission_id = ? AND question_id = ? AND kind <> ?
		   AND state IN ('pending', 'admitted')`
	args := []any{
		asyncQuestionFeatureDisabledReason, identity.AdmissionID, identity.QuestionID,
		string(models.AsyncQuestionDeliveryTranscript),
	}
	if admittedAt != nil {
		query = `
			UPDATE task_async_question_deliveries
			   SET state = 'abandoned', last_error = ?, admitted_at = COALESCE(admitted_at, ?)
			 WHERE admission_id = ? AND question_id = ? AND kind <> ?
			   AND state IN ('pending', 'admitted')`
		args = []any{
			asyncQuestionFeatureDisabledReason, *admittedAt, identity.AdmissionID, identity.QuestionID,
			string(models.AsyncQuestionDeliveryTranscript),
		}
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...)
	return err
}

// IsAsyncQuestionDeliveryRetired is the durable recovery fence for prompts
// whose unsent queue entry was retired while async questions were disabled.
func (r *Repository) IsAsyncQuestionDeliveryRetired(
	ctx context.Context,
	admissionID, questionID string,
) (bool, error) {
	var retired bool
	err := r.db.GetContext(ctx, &retired, r.db.Rebind(`
		SELECT EXISTS (
			SELECT 1 FROM task_async_question_deliveries
			 WHERE admission_id = ? AND question_id = ? AND kind <> ?
			   AND state = 'abandoned' AND last_error = ?
		)
	`), admissionID, questionID, string(models.AsyncQuestionDeliveryTranscript), asyncQuestionFeatureDisabledReason)
	return retired, err
}
