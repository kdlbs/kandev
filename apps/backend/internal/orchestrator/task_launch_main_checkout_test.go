package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestMainCheckoutRelaunchRetiresMatchingError(t *testing.T) {
	for _, successorError := range []bool{false, true} {
		name := "clears captured checkout error"
		if successorError {
			name = "preserves successor error"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			const taskID = "task-main-checkout-relaunch"
			const existingSessionID = "session-main-checkout-existing"
			seedTaskAndSession(t, repo, taskID, existingSessionID, models.TaskSessionStateCompleted)

			prior := models.TaskLaunchError{
				Message:    "The selected checkout has invalid linked-worktree metadata.",
				OccurredAt: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC),
				Scope:      models.ErrorScopeTask, Code: models.LaunchErrorCategoryGenericLaunchFailure,
				StampValue: "main-checkout-misclassification",
			}
			if len(prior.RecoveryActions) != 0 {
				t.Fatalf("seeded recovery actions = %#v, want the retained no-retry failure shape", prior.RecoveryActions)
			}
			if err := repo.SetTaskMetadataKey(ctx, taskID, models.MetaKeyLastLaunchError, prior); err != nil {
				t.Fatalf("seed prior task launch error: %v", err)
			}

			taskRepo := newMockTaskRepo()
			taskRepo.tasks[taskID] = &v1.Task{
				ID: taskID, Title: "Main checkout recovery", Description: "Continue work",
				State: v1.TaskStateInProgress, Metadata: map[string]interface{}{models.MetaKeyLastLaunchError: prior},
			}
			launchCalls := 0
			newer := models.TaskLaunchError{
				Message:    "A newer checkout launch error was recorded.",
				OccurredAt: time.Date(2026, 9, 29, 12, 1, 0, 0, time.UTC),
				Scope:      models.ErrorScopeTask, Code: models.LaunchErrorCategoryGenericLaunchFailure,
				RecoveryActions: []string{models.RecoveryActionRetryLaunch}, StampValue: "successor-checkout-error",
			}
			agentManager := &mockAgentManager{
				launchAgentFunc: func(callCtx context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
					launchCalls++
					if successorError {
						if err := repo.SetTaskMetadataKey(callCtx, taskID, models.MetaKeyLastLaunchError, newer); err != nil {
							return nil, err
						}
					}
					return &executor.LaunchAgentResponse{AgentExecutionID: "exec-main-checkout"}, nil
				},
			}
			svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)

			execution, err := svc.StartTask(ctx, taskID, "profile-main-checkout", "", "", "", "Continue work", "", false, false, nil)
			if err != nil {
				t.Fatalf("StartTask with retained checkout error: %v", err)
			}
			if execution == nil || execution.SessionID == "" {
				t.Fatalf("StartTask execution = %+v, want launched session", execution)
			}
			if launchCalls != 1 {
				t.Fatalf("agent launch calls = %d, want 1", launchCalls)
			}
			if _, err := repo.GetTaskSession(ctx, existingSessionID); err != nil {
				t.Fatalf("existing session was not preserved: %v", err)
			}

			dbTask, err := repo.GetTask(ctx, taskID)
			if err != nil {
				t.Fatalf("reload task: %v", err)
			}
			current, found := models.LoadTaskLaunchError(dbTask.Metadata)
			if successorError {
				if !found || !current.MatchesStamp(newer.Stamp()) {
					t.Fatalf("task launch error = %+v (found %t), want newer stamp %q", current, found, newer.Stamp())
				}
			} else if found {
				t.Fatalf("matching previous task launch error survived successful launch: %+v", current)
			}
		})
	}
}
