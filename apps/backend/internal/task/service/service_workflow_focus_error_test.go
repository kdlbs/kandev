package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

type moveFocusReadFailure struct {
	repository.SessionRepository
	primary bool
}

func (r moveFocusReadFailure) GetPrimarySessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error) {
	if r.primary {
		return nil, errors.New("primary query unavailable")
	}
	return r.SessionRepository.GetPrimarySessionByTaskID(ctx, taskID)
}

func (r moveFocusReadFailure) GetActiveTaskSessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error) {
	if !r.primary {
		return nil, errors.New("active query unavailable")
	}
	return r.SessionRepository.GetActiveTaskSessionByTaskID(ctx, taskID)
}

func TestService_MoveTaskWorkflowFocusKeepsReadFailureUnknown(t *testing.T) {
	for _, primary := range []bool{true, false} {
		t.Run(map[bool]string{true: "primary", false: "active"}[primary], func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			seedMoveWorkflows(t, ctx, repo)
			seedMoveSteps(svc)
			createMoveTask(t, ctx, repo, "unknown-focus", "wf-source", "step-source", nil)
			svc.sessions = moveFocusReadFailure{SessionRepository: repo, primary: primary}
			result, err := svc.MoveTask(ctx, "unknown-focus", "wf-source", "step-review-target", 0)
			if err != nil {
				t.Fatalf("MoveTask: %v", err)
			}
			if result.WorkflowFocusDisposition != "" {
				t.Fatalf("unavailable recipient evidence claimed disposition %q", result.WorkflowFocusDisposition)
			}
		})
	}
}
