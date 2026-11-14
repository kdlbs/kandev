package sqlite

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
)

const asyncQuestionAuthoritySweepLimit = 100

// StaleAsyncQuestionsForSession atomically stales the session's pending
// requests and settles every gate that depends on them.
func (r *Repository) StaleAsyncQuestionsForSession(
	ctx context.Context,
	sessionID, reason string,
	buildWake models.AsyncQuestionWakeBuilder,
) ([]*models.AsyncQuestion, error) {
	var ids []string
	if err := r.db.SelectContext(ctx, &ids, r.db.Rebind(`
		SELECT id FROM task_async_questions
		 WHERE session_id = ? AND status = 'pending'
		 ORDER BY created_at ASC, id ASC`), sessionID); err != nil {
		return nil, err
	}
	updated := make([]*models.AsyncQuestion, 0, len(ids))
	for _, id := range ids {
		question, changed, err := r.staleAsyncQuestionByID(ctx, id, reason, buildWake)
		if err != nil {
			return updated, err
		}
		if changed {
			updated = append(updated, question)
		}
	}
	if _, err := r.CancelAsyncQuestionWaitsForSession(ctx, sessionID, reason); err != nil {
		return updated, err
	}
	return updated, nil
}

// ReconcileAsyncQuestionAuthority finds a bounded page of requests whose
// task, session, or direct-parent authority changed, then rechecks each inside
// its terminal transition transaction.
func (r *Repository) ReconcileAsyncQuestionAuthority(
	ctx context.Context,
	limit int,
	buildWake models.AsyncQuestionWakeBuilder,
) ([]*models.AsyncQuestion, error) {
	if limit <= 0 || limit > asyncQuestionAuthoritySweepLimit {
		limit = asyncQuestionAuthoritySweepLimit
	}
	var ids []string
	if err := r.db.SelectContext(ctx, &ids, r.db.Rebind(`
		SELECT question.id
		  FROM task_async_questions AS question
		  LEFT JOIN tasks AS child ON child.id = question.task_id
		  LEFT JOIN tasks AS parent ON parent.id = question.parent_task_id
		  LEFT JOIN task_sessions AS author ON author.id = question.session_id
		 WHERE question.status = 'pending'
		   AND (
				child.id IS NULL OR child.archived_at IS NOT NULL OR child.workspace_id <> question.workspace_id
				OR author.id IS NULL OR author.task_id <> question.task_id
				OR author.state IN ('COMPLETED', 'FAILED', 'CANCELLED')
				OR (question.session_incarnation_id <> '' AND author.queue_incarnation_id <> question.session_incarnation_id)
				OR (question.recipient_kind = 'parent' AND (
					child.parent_id IS NULL OR child.parent_id <> question.parent_task_id
					OR parent.id IS NULL OR parent.archived_at IS NOT NULL
				))
		   )
		 ORDER BY question.created_at ASC, question.id ASC
		 LIMIT ?`), limit); err != nil {
		return nil, err
	}
	updated := make([]*models.AsyncQuestion, 0, len(ids))
	for _, id := range ids {
		question, changed, err := r.staleAsyncQuestionIfAuthorityChanged(ctx, id, buildWake)
		if err != nil {
			return updated, err
		}
		if changed {
			updated = append(updated, question)
		}
	}
	return updated, nil
}

func (r *Repository) staleAsyncQuestionByID(
	ctx context.Context,
	id, reason string,
	buildWake models.AsyncQuestionWakeBuilder,
) (*models.AsyncQuestion, bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.acquireAsyncQuestionWriteLockTx(ctx, tx, id); err != nil {
		return nil, false, err
	}
	question, err := r.lockAsyncQuestionTx(ctx, tx, id)
	if err != nil || question == nil {
		return nil, false, err
	}
	if question.Status.Terminal() {
		return nil, false, tx.Commit()
	}
	if err := r.transitionAsyncQuestionStaleTx(ctx, tx, question, reason, buildWake); err != nil {
		return nil, false, err
	}
	updated, err := r.getAsyncQuestionTx(ctx, tx, id)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return updated, updated != nil && updated.Status == models.AsyncQuestionStatusStale, nil
}

func (r *Repository) staleAsyncQuestionIfAuthorityChanged(
	ctx context.Context,
	id string,
	buildWake models.AsyncQuestionWakeBuilder,
) (*models.AsyncQuestion, bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.acquireAsyncQuestionWriteLockTx(ctx, tx, id); err != nil {
		return nil, false, err
	}
	question, err := r.lockAsyncQuestionTx(ctx, tx, id)
	if err != nil || question == nil {
		return nil, false, err
	}
	if question.Status.Terminal() {
		return nil, false, tx.Commit()
	}
	action, parentID := asyncQuestionAuthorityResolveOperator, ""
	if question.Recipient == models.AsyncQuestionRecipientParent {
		action, parentID = asyncQuestionAuthorityResolveParent, question.ParentTaskID
	}
	err = r.validateAsyncQuestionAuthorityTx(ctx, tx, question, action, parentID, buildWake)
	if errors.Is(err, models.ErrAsyncQuestionStale) {
		updated, readErr := r.getAsyncQuestionTx(ctx, tx, id)
		if readErr != nil {
			return nil, false, readErr
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return nil, false, commitErr
		}
		return updated, updated != nil && updated.Status == models.AsyncQuestionStatusStale, nil
	}
	if err != nil {
		return nil, false, err
	}
	return nil, false, tx.Commit()
}

func (r *Repository) MarkStaleAsyncQuestionsForSession(ctx context.Context, sessionID, reason string) (int, error) {
	updated, err := r.StaleAsyncQuestionsForSession(ctx, sessionID, reason, nil)
	return len(updated), err
}
