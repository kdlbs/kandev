package messagequeue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/kandev/kandev/internal/task/previewfeedback"
	"github.com/kandev/kandev/internal/task/repository/plancommenttx"
	"github.com/kandev/kandev/internal/task/repository/previewfeedbacktx"
)

// InsertWithTaskFeedback admits one caller-owned queue row and consumes exact
// plan-comment and preview-feedback versions in the same transaction.
func (r *sqliteRepository) InsertWithTaskFeedback(
	ctx context.Context,
	identity QueueSessionIdentity,
	msg *QueuedMessage,
	planRefs []models.TaskPlanCommentRef,
	previewRefs []models.TaskPreviewFeedbackRef,
	requirePrimary bool,
	claim *QueueAttachmentClaim,
	maxPerSession int,
) (*models.TaskPlanCommentSnapshot, *models.TaskPreviewFeedbackSnapshot, bool, error) {
	if err := validateTaskFeedbackQueueRequest(identity, msg, previewRefs); err != nil {
		return nil, nil, false, err
	}
	release, err := plancommenttx.AcquireLocalAdmission(ctx, msg.TaskID)
	if err != nil {
		return nil, nil, false, err
	}
	defer release()
	candidate := *msg
	tx, err := r.beginPlanCommentQueueAdmission(ctx, identity, &candidate)
	if err != nil {
		return nil, nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	existing, replayed, err := r.commitTaskFeedbackQueueReplay(
		ctx, tx, &candidate, planRefs, previewRefs,
	)
	if err != nil {
		return nil, nil, false, err
	}
	if replayed {
		*msg = *existing
		return nil, nil, true, nil
	}
	if err := r.ensureQueueCapacity(ctx, tx, candidate.SessionID, maxPerSession); err != nil {
		return nil, nil, false, err
	}
	if err := claimOptionalMessageAttachmentsTx(
		ctx, tx, &identity, claim, candidate.TaskID, candidate.SessionID,
	); err != nil {
		return nil, nil, false, err
	}
	planResolution, previewResolution, err := r.resolveQueuedTaskFeedback(
		ctx, tx, &candidate, planRefs, previewRefs, requirePrimary,
	)
	if err != nil {
		return nil, nil, false, err
	}
	if err := r.transferPreviewScreenshotsToQueue(
		ctx, tx, previewResolution, candidate.SessionID,
	); err != nil {
		return nil, nil, false, err
	}
	candidate.Content = previewResolution.Content
	if err := r.insertCoalesced(ctx, tx, &candidate, 0); err != nil {
		return nil, nil, false, err
	}
	planSnapshot, previewSnapshot, err := consumeQueuedTaskFeedback(
		ctx, tx, r.db, planResolution, previewResolution,
	)
	if err != nil {
		return nil, nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, false, fmt.Errorf("commit task-feedback queue admission: %w", err)
	}
	*msg = candidate
	return planSnapshot, previewSnapshot, false, nil
}

func validateTaskFeedbackQueueRequest(
	identity QueueSessionIdentity,
	msg *QueuedMessage,
	previewRefs []models.TaskPreviewFeedbackRef,
) error {
	if msg == nil || msg.ID == "" || len(previewRefs) == 0 {
		return errors.New("client queue id and preview feedback are required")
	}
	if msg.TaskID != identity.TaskID || msg.SessionID != identity.SessionID {
		return ErrSessionIdentityMismatch
	}
	return nil
}

func (r *sqliteRepository) commitTaskFeedbackQueueReplay(
	ctx context.Context,
	tx *sqlx.Tx,
	candidate *QueuedMessage,
	planRefs []models.TaskPlanCommentRef,
	previewRefs []models.TaskPreviewFeedbackRef,
) (*QueuedMessage, bool, error) {
	existing, err := r.findQueuedMessageByIDTx(ctx, tx, candidate.ID)
	if err != nil || existing == nil {
		return existing, false, err
	}
	if !sameTaskFeedbackQueueRequest(existing, candidate, planRefs, previewRefs) {
		return nil, false, ErrQueueIDConflict
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit task-feedback queue replay: %w", err)
	}
	return existing, true, nil
}

func (r *sqliteRepository) resolveQueuedTaskFeedback(
	ctx context.Context,
	tx *sqlx.Tx,
	candidate *QueuedMessage,
	planRefs []models.TaskPlanCommentRef,
	previewRefs []models.TaskPreviewFeedbackRef,
	requirePrimary bool,
) (*plancommenttx.Resolution, *previewfeedbacktx.Resolution, error) {
	content := candidate.Content
	var planResolution *plancommenttx.Resolution
	var err error
	if len(planRefs) > 0 {
		planResolution, err = plancommenttx.ResolveQueue(
			ctx, tx, r.db, candidate.TaskID, candidate.SessionID, content, planRefs, requirePrimary,
		)
		if err != nil {
			return nil, nil, err
		}
		content = planResolution.Content
	} else if err := plancommenttx.ValidateTargetSession(
		ctx, tx, r.db, candidate.TaskID, candidate.SessionID, requirePrimary, "",
	); err != nil {
		return nil, nil, err
	}
	previewResolution, err := previewfeedbacktx.Resolve(
		ctx, tx, r.db, candidate.TaskID, content, previewRefs,
	)
	return planResolution, previewResolution, err
}

func consumeQueuedTaskFeedback(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	planResolution *plancommenttx.Resolution,
	previewResolution *previewfeedbacktx.Resolution,
) (*models.TaskPlanCommentSnapshot, *models.TaskPreviewFeedbackSnapshot, error) {
	var planSnapshot *models.TaskPlanCommentSnapshot
	var err error
	if planResolution != nil {
		planSnapshot, err = plancommenttx.Consume(ctx, tx, db, planResolution)
		if err != nil {
			return nil, nil, err
		}
	}
	previewSnapshot, err := previewfeedbacktx.Consume(ctx, tx, db, previewResolution)
	return planSnapshot, previewSnapshot, err
}

func (r *sqliteRepository) transferPreviewScreenshotsToQueue(
	ctx context.Context,
	tx interface {
		ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	},
	resolved *previewfeedbacktx.Resolution,
	sessionID string,
) error {
	for _, item := range resolved.Items {
		if item.Kind != models.TaskPreviewFeedbackScreenshot {
			continue
		}
		result, err := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE task_message_attachments
			SET session_id = ?, updated_at = ?
			WHERE id = ? AND task_id = ? AND state = ?
			  AND session_id = '' AND message_id = '' AND queue_id = ''
		`), sessionID, time.Now().UTC(), item.ScreenshotAttachmentID,
			resolved.TaskID, models.AttachmentStateClaimed)
		if err != nil {
			return fmt.Errorf("transfer preview screenshot to queue: %w", err)
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return models.ErrAttachmentClaimConflict
		}
	}
	return nil
}

func sameTaskFeedbackQueueRequest(
	existing, candidate *QueuedMessage,
	planRefs []models.TaskPlanCommentRef,
	previewRefs []models.TaskPreviewFeedbackRef,
) bool {
	want, _ := candidate.Metadata[plancomments.MetadataRequestFingerprint].(string)
	got, _ := existing.Metadata[plancomments.MetadataRequestFingerprint].(string)
	return want != "" && got == want &&
		plancomments.MetadataRefsMatch(existing.Metadata, planRefs) &&
		previewfeedback.MetadataRefsMatch(existing.Metadata, previewRefs)
}
