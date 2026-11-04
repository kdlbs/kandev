package orchestrator

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

func TestQueueUserPrompt_RecoveryRacePreservesSuccessor(t *testing.T) {
	for _, newerState := range []models.TaskSessionState{
		models.TaskSessionStateCancelled,
		models.TaskSessionStateRunning,
		models.TaskSessionStateStarting,
	} {
		t.Run(string(newerState), func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
			seedExecutorRunning(t, repo, "s1", "t1", "exec-retained")
			session, err := repo.GetTaskSession(ctx, "s1")
			if err != nil {
				t.Fatal(err)
			}
			session.AgentProfileID = "profile1"
			if err := repo.UpdateTaskSession(ctx, session); err != nil {
				t.Fatal(err)
			}
			var ready, superseded atomic.Bool
			agentMgr := &mockAgentManager{
				repoForExecutionLookup: repo,
				isAgentRunningFn:       func(context.Context, string) bool { return ready.Load() },
				isAgentReadyFn: func(ctx context.Context, id string) bool {
					if !ready.Load() {
						return false
					}
					if superseded.Swap(true) {
						return true
					}
					current, err := repo.GetTaskSession(ctx, id)
					if err != nil {
						t.Error(err)
						return false
					}
					current.State = newerState
					if newerState == models.TaskSessionStateStarting {
						changed, err := repo.UpdateTaskSessionIfCurrentStateWithStartAttempt(
							ctx, current, models.TaskSessionStateStarting, "successor-attempt",
						)
						if err != nil || !changed {
							t.Errorf("claim successor attempt: changed=%t err=%v", changed, err)
							return false
						}
					} else if err := repo.UpdateTaskSession(ctx, current); err != nil {
						t.Error(err)
						return false
					}
					return true
				},
				currentPromptExecutionID: "exec-retained",
				launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
					ready.Store(true)
					return nil, lifecycle.ErrAgentAlreadyRunning
				},
			}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
			svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
			svc.executor.SetOnResumeFailureRollback(svc.rollbackResumeFailureIfCurrentAttempt)
			done := make(chan struct{}, 1)
			svc.onQueuedMessageExecutionComplete = func() { done <- struct{}{} }
			if err := svc.QueueUserPrompt(ctx, "t1", "s1", "after restart", "", false, nil, nil, true); err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("dispatch did not settle")
			}
			current, err := repo.GetTaskSession(ctx, "s1")
			if err != nil {
				t.Fatal(err)
			}
			if current.State != newerState {
				t.Fatalf("state=%s, want %s", current.State, newerState)
			}
			if len(agentMgr.capturedPrompts) != 0 {
				t.Fatalf("dispatched after superseding state: %v", agentMgr.capturedPrompts)
			}
			if newerState != models.TaskSessionStateCancelled {
				entries, _, err := svc.messageQueue.SnapshotSession(ctx, "s1")
				if err != nil {
					t.Fatal(err)
				}
				if len(entries) != 1 || entries[0].Content != "after restart" {
					t.Fatalf("queue after superseded resume = %+v, want original prompt retained", entries)
				}
			}
		})
	}
}
