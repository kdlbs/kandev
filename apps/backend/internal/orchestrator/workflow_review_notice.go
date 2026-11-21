package orchestrator

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/review"
	"github.com/kandev/kandev/internal/workflow/engine"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type lifecycleReviewNoticeWriter interface {
	CreateLifecycleSessionMessageIdempotent(
		context.Context, string, string, string, string, string, map[string]interface{},
	) error
}

func (s *Service) recordWorkflowReviewNoChanges(ctx context.Context, in engine.ActionInput, cause error) {
	if err := s.writeWorkflowReviewNoChanges(ctx, in, cause); err != nil {
		s.logger.Warn("could not persist skipped workflow review notice",
			zap.String("task_id", in.State.TaskID), zap.String("workflow_entry_id", in.EntryID), zap.Error(err))
	}
}

func (s *Service) writeWorkflowReviewNoChanges(ctx context.Context, in engine.ActionInput, cause error) error {
	writer, ok := s.messageCreator.(lifecycleReviewNoticeWriter)
	if !ok {
		return errors.New("lifecycle review notice writer is not configured")
	}
	sessionID := in.State.SessionID
	var noChanges *review.NoChangesError
	if errors.As(cause, &noChanges) && noChanges.SessionID != "" {
		sessionID = noChanges.SessionID
	}
	if sessionID == "" {
		return errors.New("skipped review has no session for its task notice")
	}
	messageID := uuid.NewString()
	if in.EntryID != "" {
		messageID = "workflow-review-no-changes:" + in.State.TaskID + ":" + in.EntryID
	}
	return writer.CreateLifecycleSessionMessageIdempotent(
		ctx, messageID, in.State.TaskID,
		"Automatic code review skipped because the task has no changed files to review.",
		sessionID, string(v1.MessageTypeStatus), map[string]interface{}{
			"kind":              "workflow_review_skipped",
			"reason":            review.CodeNoChanges,
			"status":            "skipped",
			"workflow_step_id":  in.Step.ID,
			"workflow_entry_id": in.EntryID,
		},
	)
}
