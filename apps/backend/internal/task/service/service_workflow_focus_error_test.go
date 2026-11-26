package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
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

func TestService_MoveTaskWorkflowFocusPreservesDeferredLaunch(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		t.Run(map[bool]string{false: "suppressed", true: "deferred"}[deferred], func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			seedMoveWorkflows(t, ctx, repo)
			seedMoveSteps(svc)
			svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
				"step-source": {ID: "step-source", WorkflowID: "wf-source"},
				"step-review-target": {
					ID: "step-review-target", WorkflowID: "wf-source",
					Events: wfmodels.StepEvents{
						OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
					},
				},
			}})
			createMoveTask(t, ctx, repo, "skip-focus", "wf-source", "step-source", nil)
			if deferred {
				must(t, repo.SetTaskMetadataKey(ctx, "skip-focus", models.MetaKeyDeferredLaunch,
					map[string]interface{}{"agent_profile_id": "profile", "intent": "start"}))
			}
			result, err := svc.MoveTaskWithOptions(ctx, "skip-focus", "wf-source", "step-review-target", 0, MoveTaskOptions{
				EntryOptions:           &workflowmove.EntryOptions{SkipStepPrompt: true},
				PreserveDeferredLaunch: true,
			})
			must(t, err)
			expected := workflowFocusDispositionNone
			if deferred {
				expected = ""
			}
			if result.WorkflowFocusDisposition != expected {
				t.Fatalf("focus disposition = %q, want %q", result.WorkflowFocusDisposition, expected)
			}
		})
	}
}
