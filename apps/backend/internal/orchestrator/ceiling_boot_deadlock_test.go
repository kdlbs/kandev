package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-AGENTS-SESSION-CEILING-001.5
func TestCeilingReplayReleasesAdmissionBeforeProviderDispatch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "replay-task", "replay-session", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "replay-session", "replay-task", "replay-execution")
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "replay-task", v1.TaskStateScheduling)
	agent := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agent)
	providerEntered := make(chan bool, 1)
	providerRelease := make(chan struct{})
	var releaseOnce sync.Once
	releaseProvider := func() { releaseOnce.Do(func() { close(providerRelease) }) }
	agent.promptAgentFunc = func(callCtx context.Context, _, _ string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
		providerEntered <- ceilingEntryAdmissionLockHeld(callCtx, "replay-task")
		<-providerRelease
		return &executor.PromptResult{}, nil
	}
	result := make(chan ceilingReplayOutcome, 1)
	go func() {
		result <- svc.replayCeilingDeferral(ctx, &models.Task{ID: "replay-task"}, models.CeilingDeferral{
			Kind: models.CeilingLaunchPromptEnsure,
			Payload: map[string]interface{}{
				metaKeySessionID: "replay-session", metaKeyPrompt: "resume the accepted work",
			},
		})
	}()
	t.Cleanup(func() {
		releaseProvider()
		select {
		case <-result:
		case <-time.After(5 * time.Second):
			t.Error("replay did not finish during cleanup")
		}
	})
	select {
	case held := <-providerEntered:
		if held {
			t.Error("provider dispatch inherited ownership of the replay admission lock")
		}
	case outcome := <-result:
		result <- outcome
		t.Fatalf("replay ended before provider dispatch: %v", outcome)
	case <-time.After(5 * time.Second):
		t.Fatal("replay did not reach provider dispatch")
	}
	admitted := make(chan struct{})
	go func() {
		release := svc.acquireCeilingEntryAdmissionLock("replay-task")
		release()
		close(admitted)
	}()
	t.Cleanup(func() { releaseProvider(); <-admitted })
	select {
	case <-admitted:
	case <-time.After(time.Second):
		t.Error("provider dispatch blocks task admission needed by lifecycle callbacks")
	}
	releaseProvider()
	outcome := <-result
	result <- outcome
	require.Equal(t, ceilingReplaySucceeded, outcome)
}

// @covers AC-AGENTS-SESSION-CEILING-001.2
func TestReviewAdmissionWaitDoesNotBlockIndependentScheduling(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "review-task", "review-session", models.TaskSessionStateWaitingForInput)
	seedTaskAndSession(t, repo, "new-task", "new-session", models.TaskSessionStateCreated)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "review-task", v1.TaskStateInProgress)
	seedMockTaskState(taskRepo, "new-task", v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	release := svc.acquireCeilingEntryAdmissionLock("review-task")
	var releaseOnce sync.Once
	releaseAdmission := func() { releaseOnce.Do(release) }
	reviewDone := make(chan struct{})
	go func() {
		svc.writeTaskReviewState(ctx, "review-task", "review-session")
		close(reviewDone)
	}()
	t.Cleanup(func() { releaseAdmission(); <-reviewDone })
	// Observe the waiter at the exact admission boundary before scheduling.
	require.Eventually(t, func() bool {
		svc.ceilingEntryAdmissionLocksMu.Lock()
		defer svc.ceilingEntryAdmissionLocksMu.Unlock()
		return svc.ceilingEntryAdmissionLocks["review-task"].refs == 2
	}, 5*time.Second, time.Millisecond)
	scheduled := make(chan error, 1)
	go func() { scheduled <- svc.scheduleTaskForSession(ctx, "new-task", "new-session") }()
	t.Cleanup(func() { releaseAdmission(); <-scheduled })
	select {
	case err := <-scheduled:
		scheduled <- err
		require.NoError(t, err)
		taskRepo.mu.Lock()
		state := taskRepo.tasks["new-task"].State
		taskRepo.mu.Unlock()
		require.Equal(t, v1.TaskStateScheduling, state)
	case <-time.After(time.Second):
		t.Error("a task-local admission wait blocked independent scheduling")
	}
}
