package messagequeue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/kandev/kandev/internal/task/repository/plancommenttx"
)

// InsertWithPlanComments admits one caller-identified queue row and consumes
// its exact task-owned plan comments in the same transaction. Comment-bearing
// entries deliberately bypass queue auto-merge so their durable replay ID is
// never discarded.
func (r *sqliteRepository) InsertWithPlanComments(
	ctx context.Context,
	msg *QueuedMessage,
	refs []models.TaskPlanCommentRef,
	requirePrimary bool,
	maxPerSession int,
) (*models.TaskPlanCommentSnapshot, bool, error) {
	if msg == nil || msg.ID == "" {
		return nil, false, errors.New("client queue id is required")
	}
	candidate := *msg
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin plan-comment queue admission: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.guardActiveTaskTx(ctx, tx, candidate.TaskID); err != nil {
		return nil, false, err
	}
	if err := r.lockSessionTx(ctx, tx, candidate.SessionID); err != nil {
		return nil, false, err
	}
	existing, err := r.findQueuedMessageByIDTx(ctx, tx, candidate.ID)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		if !samePlanCommentQueueRequest(existing, &candidate, refs) {
			return nil, false, ErrQueueIDConflict
		}
		if err := tx.Commit(); err != nil {
			return nil, false, fmt.Errorf("commit plan-comment queue replay: %w", err)
		}
		*msg = *existing
		return nil, true, nil
	}
	if err := r.ensureQueueCapacity(ctx, tx, candidate.SessionID, maxPerSession); err != nil {
		return nil, false, err
	}
	resolved, err := plancommenttx.Resolve(
		ctx, tx, r.db, candidate.TaskID, candidate.SessionID, candidate.Content, refs, requirePrimary,
	)
	if err != nil {
		return nil, false, err
	}
	candidate.Content = resolved.Content
	if err := r.insertCoalesced(ctx, tx, &candidate, 0); err != nil {
		return nil, false, err
	}
	snapshot, err := plancommenttx.Consume(ctx, tx, r.db, resolved)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit plan-comment queue admission: %w", err)
	}
	*msg = candidate
	return snapshot, false, nil
}

func (r *sqliteRepository) findQueuedMessageByIDTx(
	ctx context.Context,
	tx *sqlx.Tx,
	id string,
) (*QueuedMessage, error) {
	row := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT id, session_id, task_id, position, content, model, plan_mode,
		       attachments_json, metadata_json, queued_at, queued_by
		FROM queued_messages WHERE id = ?
	`), id)
	msg, err := scanQueuedRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read plan-comment queue replay: %w", err)
	}
	return msg, nil
}

func samePlanCommentQueueRequest(
	existing, candidate *QueuedMessage,
	refs []models.TaskPlanCommentRef,
) bool {
	want, _ := candidate.Metadata[plancomments.MetadataRequestFingerprint].(string)
	got, _ := existing.Metadata[plancomments.MetadataRequestFingerprint].(string)
	return want != "" && got == want && plancomments.MetadataRefsMatch(existing.Metadata, refs)
}
