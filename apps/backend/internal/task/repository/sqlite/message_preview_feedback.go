package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/admission"
	"github.com/kandev/kandev/internal/task/repository/plancommenttx"
	"github.com/kandev/kandev/internal/task/repository/previewfeedbacktx"
)

// ValidateMessageTaskFeedback repeats exact route, plan-comment, preview, and
// final prompt validation without accepting a message.
func (r *Repository) ValidateMessageTaskFeedback(
	ctx context.Context,
	taskID, sessionID, content string,
	planRefs []models.TaskPlanCommentRef,
	previewRefs []models.TaskPreviewFeedbackRef,
	requirePrimary bool,
	expectedState models.TaskSessionState,
) error {
	tx, release, err := r.beginPlanCommentTx(ctx, taskID)
	if err != nil {
		return err
	}
	defer release()
	defer func() { _ = tx.Rollback() }()
	if err := r.guardActivePlanCommentTaskTx(ctx, tx, taskID); err != nil {
		return err
	}
	if len(planRefs) > 0 {
		resolved, err := plancommenttx.ResolveDirect(
			ctx, tx, r.db, taskID, sessionID, content, planRefs, requirePrimary, expectedState,
		)
		if err != nil {
			return err
		}
		content = resolved.Content
	} else if err := plancommenttx.ValidateTargetSession(
		ctx, tx, r.db, taskID, sessionID, requirePrimary, expectedState,
	); err != nil {
		return err
	}
	_, err = previewfeedbacktx.Resolve(ctx, tx, r.db, taskID, content, previewRefs)
	return err
}

// CreateMessageWithTaskFeedback atomically persists a user message and
// consumes its exact plan-comment and preview-feedback snapshots.
func (r *Repository) CreateMessageWithTaskFeedback(
	ctx context.Context,
	message *models.Message,
	planRefs []models.TaskPlanCommentRef,
	previewRefs []models.TaskPreviewFeedbackRef,
	requirePrimary bool,
	expectedState models.TaskSessionState,
	claim *messagequeue.QueueAttachmentClaim,
) (*models.TaskPlanCommentSnapshot, *models.TaskPreviewFeedbackSnapshot, error) {
	return r.createMessageWithTaskFeedback(
		ctx, message, nil, planRefs, previewRefs, requirePrimary, expectedState, claim,
	)
}

// CreateMessageWithTaskFeedbackWithInitialTaskBrief includes first-prompt
// selection in the same context-consumption transaction.
func (r *Repository) CreateMessageWithTaskFeedbackWithInitialTaskBrief(
	ctx context.Context,
	message *models.Message,
	candidate *admission.InitialTaskBriefCandidate,
	planRefs []models.TaskPlanCommentRef,
	previewRefs []models.TaskPreviewFeedbackRef,
	requirePrimary bool,
	expectedState models.TaskSessionState,
	claim *messagequeue.QueueAttachmentClaim,
) (*models.TaskPlanCommentSnapshot, *models.TaskPreviewFeedbackSnapshot, error) {
	return r.createMessageWithTaskFeedback(
		ctx, message, candidate, planRefs, previewRefs, requirePrimary, expectedState, claim,
	)
}

func (r *Repository) createMessageWithTaskFeedback(
	ctx context.Context,
	message *models.Message,
	candidate *admission.InitialTaskBriefCandidate,
	planRefs []models.TaskPlanCommentRef,
	previewRefs []models.TaskPreviewFeedbackRef,
	requirePrimary bool,
	expectedState models.TaskSessionState,
	claim *messagequeue.QueueAttachmentClaim,
) (*models.TaskPlanCommentSnapshot, *models.TaskPreviewFeedbackSnapshot, error) {
	if message == nil || len(previewRefs) == 0 {
		return nil, nil, fmt.Errorf("preview feedback and user message are required")
	}
	messageType, requestsInput, err := normalizeTaskFeedbackMessage(message)
	if err != nil {
		return nil, nil, err
	}
	original := *message
	originalCandidateSelected := taskFeedbackCandidateSelected(candidate)
	tx, release, err := r.beginPlanCommentTx(ctx, message.TaskID)
	if err != nil {
		return nil, nil, fmt.Errorf("begin task-feedback message creation: %w", err)
	}
	defer release()
	defer func() {
		_ = tx.Rollback()
		if err != nil {
			*message = original
			if candidate != nil {
				candidate.Selected = originalCandidateSelected
			}
		}
	}()
	if err = r.guardActivePlanCommentTaskTx(ctx, tx, message.TaskID); err != nil {
		return nil, nil, err
	}
	if err = lockSessionTurnWrites(ctx, tx, r.db.DriverName(), message.TaskSessionID); err != nil {
		return nil, nil, err
	}

	content, err := r.applyTaskFeedbackInitialBrief(ctx, tx, message, candidate)
	if err != nil {
		return nil, nil, err
	}
	planResolution, previewResolution, err := r.resolveDirectTaskFeedback(
		ctx, tx, message, content, planRefs, previewRefs, requirePrimary, expectedState,
	)
	if err != nil {
		return nil, nil, err
	}
	if err = r.persistDirectTaskFeedbackMessage(
		ctx, tx, message, previewResolution, claim, messageType, requestsInput,
	); err != nil {
		return nil, nil, err
	}
	planSnapshot, previewSnapshot, err := consumeDirectTaskFeedback(
		ctx, tx, r.db, planResolution, previewResolution,
	)
	if err != nil {
		return nil, nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("commit task-feedback message creation: %w", err)
	}
	return planSnapshot, previewSnapshot, nil
}

// CreateMessageWithTaskFeedbackAndQueue persists the transcript row, durable
// queue receipt, attachment claims, and exact feedback consumption together.
func (r *Repository) CreateMessageWithTaskFeedbackAndQueue(
	ctx context.Context,
	message *models.Message,
	queued *messagequeue.QueuedMessage,
	planRefs []models.TaskPlanCommentRef,
	previewRefs []models.TaskPreviewFeedbackRef,
	requirePrimary bool,
	expectedState models.TaskSessionState,
	claim *messagequeue.QueueAttachmentClaim,
	maxPerSession int,
) (*models.TaskPlanCommentSnapshot, *models.TaskPreviewFeedbackSnapshot, error) {
	return r.createMessageWithTaskFeedbackAndQueue(
		ctx, message, queued, nil, planRefs, previewRefs, requirePrimary,
		expectedState, claim, maxPerSession,
	)
}

// CreateMessageWithTaskFeedbackAndQueueWithInitialTaskBrief includes first-prompt
// selection in the same queued context-consumption transaction.
func (r *Repository) CreateMessageWithTaskFeedbackAndQueueWithInitialTaskBrief(
	ctx context.Context,
	message *models.Message,
	queued *messagequeue.QueuedMessage,
	candidate *admission.InitialTaskBriefCandidate,
	planRefs []models.TaskPlanCommentRef,
	previewRefs []models.TaskPreviewFeedbackRef,
	requirePrimary bool,
	expectedState models.TaskSessionState,
	claim *messagequeue.QueueAttachmentClaim,
	maxPerSession int,
) (*models.TaskPlanCommentSnapshot, *models.TaskPreviewFeedbackSnapshot, error) {
	return r.createMessageWithTaskFeedbackAndQueue(
		ctx, message, queued, candidate, planRefs, previewRefs, requirePrimary,
		expectedState, claim, maxPerSession,
	)
}

func (r *Repository) createMessageWithTaskFeedbackAndQueue(
	ctx context.Context,
	message *models.Message,
	queued *messagequeue.QueuedMessage,
	candidate *admission.InitialTaskBriefCandidate,
	planRefs []models.TaskPlanCommentRef,
	previewRefs []models.TaskPreviewFeedbackRef,
	requirePrimary bool,
	expectedState models.TaskSessionState,
	claim *messagequeue.QueueAttachmentClaim,
	maxPerSession int,
) (*models.TaskPlanCommentSnapshot, *models.TaskPreviewFeedbackSnapshot, error) {
	if err := validateQueuedTaskFeedbackMessage(message, queued, previewRefs); err != nil {
		return nil, nil, err
	}
	messageType, requestsInput, err := normalizeTaskFeedbackMessage(message)
	if err != nil || queued.QueuedBy != messagequeue.QueuedByUser {
		return nil, nil, fmt.Errorf("queued task feedback requires a user message")
	}
	originalMessage, originalQueued := *message, *queued
	originalCandidateSelected := taskFeedbackCandidateSelected(candidate)
	tx, release, err := r.beginPlanCommentTx(ctx, message.TaskID)
	if err != nil {
		return nil, nil, fmt.Errorf("begin queued task-feedback message creation: %w", err)
	}
	defer release()
	defer func() {
		_ = tx.Rollback()
		if err != nil {
			*message, *queued = originalMessage, originalQueued
			if candidate != nil {
				candidate.Selected = originalCandidateSelected
			}
		}
	}()
	if err = r.guardActivePlanCommentTaskTx(ctx, tx, message.TaskID); err != nil {
		return nil, nil, err
	}
	if err = lockSessionTurnWrites(ctx, tx, r.db.DriverName(), message.TaskSessionID); err != nil {
		return nil, nil, err
	}
	content, err := r.applyTaskFeedbackInitialBrief(ctx, tx, message, candidate)
	if err != nil {
		return nil, nil, err
	}
	planResolution, previewResolution, err := r.resolveDirectTaskFeedback(
		ctx, tx, message, content, planRefs, previewRefs, requirePrimary, expectedState,
	)
	if err != nil {
		return nil, nil, err
	}
	if err = r.persistQueuedTaskFeedbackMessage(
		ctx, tx, message, queued, previewResolution, claim,
		messageType, requestsInput, maxPerSession,
	); err != nil {
		return nil, nil, err
	}
	planSnapshot, previewSnapshot, err := consumeDirectTaskFeedback(
		ctx, tx, r.db, planResolution, previewResolution,
	)
	if err != nil {
		return nil, nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("commit queued task-feedback message creation: %w", err)
	}
	return planSnapshot, previewSnapshot, nil
}

func (r *Repository) persistDirectTaskFeedbackMessage(
	ctx context.Context,
	tx *sqlx.Tx,
	message *models.Message,
	previewResolution *previewfeedbacktx.Resolution,
	claim *messagequeue.QueueAttachmentClaim,
	messageType string,
	requestsInput int,
) error {
	message.Content = previewResolution.Content
	if err := r.claimDirectMessageAttachmentsTx(
		ctx, tx, claim, message.TaskID, message.TaskSessionID, message.ID,
	); err != nil {
		return err
	}
	if err := r.transferPreviewScreenshotsToMessage(
		ctx, tx, previewResolution, message.TaskSessionID, message.ID,
	); err != nil {
		return err
	}
	metadataJSON, err := marshalTaskFeedbackMetadata(message.Metadata)
	if err != nil {
		return fmt.Errorf("serialize task-feedback message metadata: %w", err)
	}
	normalizedTime := dialect.NormalizedMicrosecond(r.db.DriverName(), "created_at")
	if err := r.assignUserMessageBoundary(
		ctx, tx, message, r.db.DriverName(), normalizedTime,
	); err != nil {
		return err
	}
	return r.insertMessageRow(ctx, tx, message, requestsInput, messageType, string(metadataJSON))
}

func (r *Repository) persistQueuedTaskFeedbackMessage(
	ctx context.Context,
	tx *sqlx.Tx,
	message *models.Message,
	queued *messagequeue.QueuedMessage,
	previewResolution *previewfeedbacktx.Resolution,
	claim *messagequeue.QueueAttachmentClaim,
	messageType string,
	requestsInput, maxPerSession int,
) error {
	if err := r.persistDirectTaskFeedbackMessage(
		ctx, tx, message, previewResolution, claim, messageType, requestsInput,
	); err != nil {
		return err
	}
	queued.Content = previewResolution.Content
	return messagequeue.InsertTaskOwnedInTransaction(ctx, tx, r.db, queued, maxPerSession)
}

func normalizeTaskFeedbackMessage(message *models.Message) (string, int, error) {
	if message.ID == "" {
		message.ID = uuid.New().String()
	}
	if message.AuthorType == "" {
		message.AuthorType = models.MessageAuthorUser
	}
	if message.AuthorType != models.MessageAuthorUser {
		return "", 0, fmt.Errorf("task feedback requires a user message")
	}
	messageType := string(message.Type)
	if messageType == "" {
		messageType = string(models.MessageTypeMessage)
	}
	requestsInput := 0
	if message.RequestsInput {
		requestsInput = 1
	}
	return messageType, requestsInput, nil
}

func validateQueuedTaskFeedbackMessage(
	message *models.Message,
	queued *messagequeue.QueuedMessage,
	previewRefs []models.TaskPreviewFeedbackRef,
) error {
	if message == nil || queued == nil || queued.ID == "" || len(previewRefs) == 0 {
		return fmt.Errorf("preview feedback, user message, and queued message are required")
	}
	if message.TaskID != queued.TaskID || message.TaskSessionID != queued.SessionID {
		return fmt.Errorf("queued task-feedback target does not match user message")
	}
	return nil
}

func taskFeedbackCandidateSelected(candidate *admission.InitialTaskBriefCandidate) bool {
	return candidate != nil && candidate.Selected
}

func (r *Repository) applyTaskFeedbackInitialBrief(
	ctx context.Context,
	tx *sqlx.Tx,
	message *models.Message,
	candidate *admission.InitialTaskBriefCandidate,
) (string, error) {
	content := message.Content
	if candidate == nil {
		return content, nil
	}
	if err := r.validateInitialTaskBriefCandidate(ctx, tx, message.TaskID, candidate); err != nil {
		return "", err
	}
	if err := r.selectInitialTaskBriefCandidate(
		ctx, tx, message.TaskSessionID, message, candidate,
	); err != nil {
		return "", err
	}
	if candidate.Selected {
		return message.Content, nil
	}
	return content, nil
}

func (r *Repository) resolveDirectTaskFeedback(
	ctx context.Context,
	tx *sqlx.Tx,
	message *models.Message,
	content string,
	planRefs []models.TaskPlanCommentRef,
	previewRefs []models.TaskPreviewFeedbackRef,
	requirePrimary bool,
	expectedState models.TaskSessionState,
) (*plancommenttx.Resolution, *previewfeedbacktx.Resolution, error) {
	var planResolution *plancommenttx.Resolution
	var err error
	if len(planRefs) > 0 {
		planResolution, err = plancommenttx.ResolveDirect(
			ctx, tx, r.db, message.TaskID, message.TaskSessionID, content,
			planRefs, requirePrimary, expectedState,
		)
		if err != nil {
			return nil, nil, err
		}
		content = planResolution.Content
	} else if err := plancommenttx.ValidateTargetSession(
		ctx, tx, r.db, message.TaskID, message.TaskSessionID, requirePrimary, expectedState,
	); err != nil {
		return nil, nil, err
	}
	previewResolution, err := previewfeedbacktx.Resolve(
		ctx, tx, r.db, message.TaskID, content, previewRefs,
	)
	return planResolution, previewResolution, err
}

func marshalTaskFeedbackMetadata(metadata map[string]interface{}) ([]byte, error) {
	value, err := json.Marshal(metadata)
	if err == nil && string(value) == jsonNull {
		return []byte("{}"), nil
	}
	return value, err
}

func consumeDirectTaskFeedback(
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

func (r *Repository) transferPreviewScreenshotsToMessage(
	ctx context.Context,
	tx interface {
		ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	},
	resolved *previewfeedbacktx.Resolution,
	sessionID, messageID string,
) error {
	for _, item := range resolved.Items {
		if item.Kind != models.TaskPreviewFeedbackScreenshot {
			continue
		}
		result, err := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE task_message_attachments
			SET session_id = ?, message_id = ?, updated_at = ?
			WHERE id = ? AND task_id = ? AND state = ?
			  AND session_id = '' AND message_id = '' AND queue_id = ''
		`), sessionID, messageID, time.Now().UTC(), item.ScreenshotAttachmentID,
			resolved.TaskID, models.AttachmentStateClaimed)
		if err != nil {
			return fmt.Errorf("transfer preview screenshot to message: %w", err)
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return models.ErrAttachmentClaimConflict
		}
	}
	return nil
}
