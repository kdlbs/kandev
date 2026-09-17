package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type gatedPromptAgentManager struct {
	*mockAgentManager
	beforePrompt chan struct{}
	allowPrompt  chan struct{}
	allowOnce    sync.Once
}

func (m *gatedPromptAgentManager) releasePrompt() {
	m.allowOnce.Do(func() { close(m.allowPrompt) })
}

func (m *gatedPromptAgentManager) PromptAgentWithDispatchCallback(
	ctx context.Context,
	executionID, prompt string,
	attachments []v1.MessageAttachment,
	dispatchOnly bool,
	onDispatched func(),
) (*executor.PromptResult, error) {
	close(m.beforePrompt)
	<-m.allowPrompt
	return m.mockAgentManager.PromptAgentWithDispatchCallback(
		ctx, executionID, prompt, attachments, dispatchOnly, onDispatched,
	)
}

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

// @covers AC-AGENTS-SESSION-CEILING-001.5
func TestCeilingReplayCommitsRouteOwnershipBeforeProviderDispatch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSessionWithStep(t, repo, "commit-task", "commit-session", "step-old")
	if err := repo.UpdateTaskSessionState(ctx, "commit-session", models.TaskSessionStateWaitingForInput, ""); err != nil {
		t.Fatalf("update session state: %v", err)
	}
	seedExecutorRunning(t, repo, "commit-session", "commit-task", "commit-execution")
	task, err := repo.GetTask(ctx, "commit-task")
	require.NoError(t, err)
	entryIdentity := (&Service{repo: repo}).workflowEntryIdentity(ctx, task.ID)
	oldRoute := models.WorkflowSessionRoute{
		OperationID:       "route-old",
		DestinationStepID: "step-old",
		EntryIdentity:     entryIdentity,
		TargetKind:        "new_session",
		DestinationID:     "commit-session",
		Phase:             "committed",
	}
	newRoute := oldRoute
	newRoute.OperationID = "route-new"
	newRoute.DestinationStepID = "step-new"
	newRoute.EntryIdentity = "entry:new"
	require.NoError(t, repo.SetTaskMetadataKey(ctx, task.ID, models.MetaKeyWorkflowSessionRoute, oldRoute))

	deferral := models.CeilingDeferral{
		Kind: models.CeilingLaunchPromptEnsure,
		Payload: map[string]interface{}{
			metaKeySessionID: "commit-session",
			metaKeyPrompt:    "old workflow prompt",
			models.CeilingLaunchEntryBindingKey: map[string]interface{}{
				"workflow_id":            "wf1",
				"destination_step_id":    "step-old",
				"route_operation_id":     "route-old",
				"entry_identity":         entryIdentity,
				"destination_session_id": "commit-session",
			},
		},
		Origin:   string(launchOriginAutomatic),
		QueuedAt: time.Now().UTC(),
	}

	stepGetter := newMockStepGetter()
	stepGetter.steps["step-old"] = &wfmodels.WorkflowStep{ID: "step-old", WorkflowID: "wf1"}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, task.ID, v1.TaskStateScheduling)
	baseAgent := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	agent := &gatedPromptAgentManager{
		mockAgentManager: baseAgent,
		beforePrompt:     make(chan struct{}),
		allowPrompt:      make(chan struct{}),
	}
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agent)
	result := make(chan ceilingReplayOutcome, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		result <- svc.replayCeilingDeferral(ctx, &models.Task{ID: task.ID}, deferral)
	}()
	t.Cleanup(func() {
		agent.releasePrompt()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("replay did not finish during cleanup")
		}
	})
	select {
	case <-agent.beforePrompt:
	case outcome := <-result:
		t.Fatalf("replay ended before provider dispatch: %v", outcome)
	case <-time.After(5 * time.Second):
		t.Fatal("replay did not reach the provider dispatch boundary")
	}

	routeErr := svc.persistWorkflowSessionRoute(context.Background(), task.ID, newRoute)
	require.Error(t, routeErr)
	require.True(t, errors.Is(routeErr, ErrCeilingEntryDispatchCommitted), routeErr)
	agent.releasePrompt()
	require.Equal(t, ceilingReplaySucceeded, <-result)

	reloaded, err := repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	route, ok := models.LoadWorkflowSessionRoute(reloaded.Metadata)
	require.True(t, ok)
	require.Equal(t, oldRoute.OperationID, route.OperationID)
}
