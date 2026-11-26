package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

const (
	replayLifecycleTaskID    = "fresh-start-lifecycle-task"
	replayLifecycleSessionID = "fresh-start-lifecycle-session"
)

// seedFailedFirstLaunchForReplay stores a failed bootstrap with a pending
// original submission, the state Retry launch recovers from.
func seedFailedFirstLaunchForReplay(t *testing.T, repo *sqliterepo.Repository) {
	t.Helper()
	ctx := context.Background()
	seedTaskAndSession(t, repo, replayLifecycleTaskID, replayLifecycleSessionID, models.TaskSessionStateFailed)
	task, err := repo.GetTask(ctx, replayLifecycleTaskID)
	require.NoError(t, err)
	task.WorkspaceID = "ws1"
	require.NoError(t, repo.UpdateTask(ctx, task))
	session, err := repo.GetTaskSession(ctx, replayLifecycleSessionID)
	require.NoError(t, err)
	session.AgentProfileID = "fresh-start-lifecycle-profile"
	session.TaskEnvironmentID = "fresh-start-lifecycle-environment"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: session.TaskEnvironmentID, TaskID: replayLifecycleTaskID,
		ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady,
	}))
	seedExecutorRunning(t, repo, replayLifecycleSessionID, replayLifecycleTaskID, "fresh-start-lifecycle-previous")
	require.NoError(t, repo.SetSessionMetadataKey(ctx, replayLifecycleSessionID, models.SessionMetaKeyLastAgentError, models.LastAgentError{
		Message: "agent startup failed", Phase: models.LaunchErrorPhaseBootstrap, StampValue: "fresh-start-lifecycle-error",
	}))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, replayLifecycleSessionID, models.SessionMetaKeyInitialPromptSubmission, map[string]interface{}{
		"version": 1, "content": "original submitted request", "plan_mode": true,
		"attachments": []v1.MessageAttachment{}, "state": "pending",
	}))
}

// replayTurnAgent reports acceptance at once and holds a full-turn prompt until
// the test ends the turn, as the runtime does while a turn is in flight.
type replayTurnAgent struct {
	*mockAgentManager
	startEntered   chan struct{}
	processStarted chan struct{}
	turnEntered    chan struct{}
	turnEnded      chan struct{}
	turnReturned   chan struct{}
	endTurnOnce    sync.Once
}

func newReplayTurnAgent(repo *sqliterepo.Repository) *replayTurnAgent {
	agent := &replayTurnAgent{
		startEntered: make(chan struct{}), processStarted: make(chan struct{}),
		turnEntered: make(chan struct{}), turnEnded: make(chan struct{}), turnReturned: make(chan struct{}),
	}
	agent.mockAgentManager = &mockAgentManager{
		repoForExecutionLookup:     repo,
		isAgentRunning:             true,
		promptAcceptedBeforeResult: true,
		isAgentReadyFn:             func(context.Context, string) bool { return true },
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			return &executor.LaunchAgentResponse{AgentExecutionID: "fresh-start-lifecycle-execution"}, nil
		},
		startAgentProcessFunc: func(context.Context, string) error {
			close(agent.startEntered)
			return nil
		},
		promptAgentFunc: func(_ context.Context, _ string, _ string, _ []v1.MessageAttachment, dispatchOnly bool) (*executor.PromptResult, error) {
			if !dispatchOnly {
				close(agent.turnEntered)
				<-agent.turnEnded
				close(agent.turnReturned)
			}
			return &executor.PromptResult{}, nil
		},
	}
	return agent
}

func (a *replayTurnAgent) endTurn() { a.endTurnOnce.Do(func() { close(a.turnEnded) }) }

// joinTurn releases a started turn on any exit path before database teardown.
func (a *replayTurnAgent) joinTurn() {
	a.endTurn()
	select {
	case <-a.turnEntered:
		<-a.turnReturned
	default:
	}
}

func waitForSignal(t *testing.T, signal <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// @covers AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.8
func TestFreshStartReplayReleasesLifecycleAtAcceptance(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedFailedFirstLaunchForReplay(t, repo)
	agent := newReplayTurnAgent(repo)
	t.Cleanup(agent.joinTurn)

	taskRepo := newMockTaskRepo()
	taskRepo.tasks[replayLifecycleTaskID] = &v1.Task{
		ID: replayLifecycleTaskID, WorkspaceID: "ws1", WorkflowID: "wf1",
		Title: "Fresh start lifecycle", State: v1.TaskStateInProgress,
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agent.mockAgentManager)
	const sourceStepID = "fresh-start-lifecycle-source"
	const targetStepID = "fresh-start-lifecycle-target"
	stepGetter := newMockStepGetter()
	stepGetter.steps[sourceStepID] = &wfmodels.WorkflowStep{ID: sourceStepID, WorkflowID: "wf1", Name: "Source", Position: 0}
	stepGetter.steps[targetStepID] = &wfmodels.WorkflowStep{ID: targetStepID, WorkflowID: "wf1", Name: "Target", Position: 1}
	svc.SetWorkflowStepGetter(stepGetter)
	task, err := repo.GetTask(ctx, replayLifecycleTaskID)
	require.NoError(t, err)
	task.WorkflowStepID = sourceStepID
	require.NoError(t, repo.UpdateTask(ctx, task))
	attachmentService, err := taskservice.NewAttachmentService(repo, t.TempDir(), nil, testLogger())
	require.NoError(t, err)
	svc.SetAttachmentReader(attachmentService)
	svc.messageCreator = &mockMessageCreator{}
	svc.turnService = &repoTurnService{repo: repo}
	svc.executor.SetOnAgentProcessStartFailed(svc.handleAgentProcessStartFailed)
	svc.executor.SetOnAgentProcessStarted(func(callbackCtx context.Context, taskID, sessionID, executionID string) {
		svc.handleAgentProcessStarted(callbackCtx, taskID, sessionID, executionID)
		close(agent.processStarted)
	})

	recoveryCtx, cancelRecovery := context.WithCancel(ctx)
	recoveryDone := make(chan error, 1)
	recoveryFinished := make(chan struct{})
	go func() {
		defer close(recoveryFinished)
		_, recoverErr := svc.RecoverSessionWithOptions(recoveryCtx, replayLifecycleTaskID, replayLifecycleSessionID, "fresh_start", RecoverSessionOptions{})
		recoveryDone <- recoverErr
	}()
	t.Cleanup(func() {
		cancelRecovery()
		agent.endTurn()
		<-recoveryFinished
	})
	waitForSignal(t, agent.startEntered, "fresh recovery startup")
	attempt, ok := svc.resumeAttemptStore().current(replayLifecycleSessionID)
	require.True(t, ok)
	require.Eventually(t, func() bool { return attempt.execution() != "" }, 3*time.Second, 10*time.Millisecond)
	waitForSignal(t, agent.processStarted, "provider process startup")
	svc.handleAgentBootReady(ctx, watcher.AgentEventData{
		TaskID: replayLifecycleTaskID, SessionID: replayLifecycleSessionID,
		AgentExecutionID: "fresh-start-lifecycle-execution", AttemptID: attempt.identity(),
	})

	select {
	case err := <-recoveryDone:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("fresh recovery must finish at provider acceptance, not when the replayed turn ends")
	}

	// The agent is still in its turn, waiting on a question.
	session, err := repo.GetTaskSession(ctx, replayLifecycleSessionID)
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	focusErr := runWithinBudget(t, "focusTaskSession during the replayed turn", nil, func() error {
		_, _, _, err := svc.focusTaskSession(ctx, replayLifecycleTaskID, replayLifecycleSessionID)
		return err
	})
	require.NoError(t, focusErr)
	requireSessionAdmissionFree(t, svc, replayLifecycleSessionID)
	releaseLifecycle, acquired := svc.tryAcquireSessionLifecycleLock(replayLifecycleSessionID)
	require.True(t, acquired, "recovery must not hold the session lifecycle lock for the whole turn")
	releaseLifecycle()

	// A normal terminal event settles the accepted turn and applies a move that
	// arrived while the dispatch-only replay was still running.
	session, err = repo.GetTaskSession(ctx, replayLifecycleSessionID)
	require.NoError(t, err)
	session.State = models.TaskSessionStateRunning
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	require.NoError(t, svc.messageQueue.SetPendingMove(ctx, replayLifecycleSessionID, &messagequeue.PendingMove{
		SessionIncarnationID: session.QueueIncarnationID,
		TaskID:               replayLifecycleTaskID,
		WorkflowID:           "wf1",
		FromStepID:           sourceStepID,
		WorkflowStepID:       targetStepID,
	}))
	activeTurn, err := svc.turnService.GetActiveTurn(ctx, replayLifecycleSessionID)
	require.NoError(t, err)
	require.NotNil(t, activeTurn)
	agent.endTurn()
	svc.handleAgentReady(ctx, watcher.AgentEventData{
		TaskID: replayLifecycleTaskID, SessionID: replayLifecycleSessionID,
		AgentExecutionID: "fresh-start-lifecycle-execution", AgentProfileID: "fresh-start-lifecycle-profile",
		AttemptID: attempt.identity(), PromptGeneration: agent.currentPromptGeneration.Load(), TurnID: activeTurn.ID,
	})
	task, err = repo.GetTask(ctx, replayLifecycleTaskID)
	require.NoError(t, err)
	require.Equal(t, targetStepID, task.WorkflowStepID, "the terminal event applies the pending workflow move")
	_, pendingMoveExists, err := svc.messageQueue.GetPendingMoveWithError(ctx, replayLifecycleSessionID)
	require.NoError(t, err)
	require.False(t, pendingMoveExists, "the terminal event consumes the pending move")
}
