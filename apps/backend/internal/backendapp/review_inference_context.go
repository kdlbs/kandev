package backendapp

import (
	"context"
	"fmt"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/review"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

const reviewInferenceOwnerKind = "review"

type reviewInferenceIdentityReader interface {
	GetTaskSession(context.Context, string) (*taskmodels.TaskSession, error)
	GetTaskReviewRun(context.Context, string) (*taskmodels.TaskReviewRun, error)
}

func reviewInferenceContext(reader reviewInferenceIdentityReader) func(context.Context, review.RunRequest, string, review.ReviewerIdentity) (context.Context, error) {
	return func(ctx context.Context, req review.RunRequest, runID string, reviewer review.ReviewerIdentity) (context.Context, error) {
		session, err := reader.GetTaskSession(ctx, req.SessionID)
		if err != nil {
			return ctx, fmt.Errorf("verify review session: %w", err)
		}
		run, err := reader.GetTaskReviewRun(ctx, runID)
		if err != nil {
			return ctx, fmt.Errorf("verify review run: %w", err)
		}
		if session == nil || run == nil || session.TaskID != req.TaskID || run.TaskID != req.TaskID ||
			run.SessionID != session.ID || run.ID != runID || session.ID != req.SessionID {
			return ctx, fmt.Errorf("review run and session ownership do not match")
		}
		return agentruntime.WithInferenceIdentity(ctx, agentruntime.InferenceIdentity{
			TaskID: run.TaskID, SessionID: session.ID, LogicalProfileID: reviewer.ProfileID, ReviewRunID: run.ID, OwnerKind: reviewInferenceOwnerKind,
		}), nil
	}
}
