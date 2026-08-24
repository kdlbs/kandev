package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// The REVIEW transition is written by setSessionWaitingForInput after every
// completed turn. Reaping there would stop the stack between a turn and its
// follow-up prompt, which is exactly the warm-reuse window, so both REVIEW
// writers must leave the stack alone.
func TestAgentStackReaping_ReviewTransitionPreservesWarmStack(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(*Service, context.Context)
	}{
		{name: "turn complete", invoke: func(s *Service, ctx context.Context) {
			s.writeTaskReviewState(ctx, "task-review-warm", "session-review-warm")
		}},
		{name: "turn cancel", invoke: func(s *Service, ctx context.Context) {
			s.writeTaskReviewStateOnCancel(ctx, "task-review-warm", "session-review-warm")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task-review-warm", "session-review-warm", models.TaskSessionStateWaitingForInput)
			seedExecutorRunning(t, repo, "session-review-warm", "task-review-warm", "exec-review-warm")

			taskRepo := newMockTaskRepo()
			seedMockTaskState(taskRepo, "task-review-warm", v1.TaskStateInProgress)
			stopCalls := make(chan stopAgentCall, 1)
			svc := newReapingTestService(t, repo, taskRepo, stopCalls)

			tt.invoke(svc, ctx)

			assertNoAgentStackStop(t, stopCalls)
			require.Equal(t, v1.TaskStateReview, taskStateFromMock(taskRepo, "task-review-warm"))
		})
	}
}

func TestAgentStackReaping_TaskCompletedEventStopsSettledSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-completed-reap", "session-completed-reap", models.TaskSessionStateCompleted)
	seedExecutorRunning(t, repo, "session-completed-reap", "task-completed-reap", "exec-completed-reap")

	stopCalls := make(chan stopAgentCall, 1)
	svc := newReapingTestService(t, repo, newMockTaskRepo(), stopCalls)
	completed := v1.TaskStateCompleted

	svc.handleTaskStateChanged(ctx, watcher.TaskEventData{
		TaskID:   "task-completed-reap",
		NewState: &completed,
	})

	call := waitForAgentStackStop(t, stopCalls)
	require.Equal(t, "exec-completed-reap", call.ExecutionID)
	require.Equal(t, stopReasonAgentStackTaskCompleted, call.Reason)
	require.False(t, call.Force)
}

// StopByTaskID only reaches CREATED/STARTING/RUNNING/WAITING_FOR_INPUT rows,
// so an IDLE office session survives CompleteTask with a live stack unless the
// COMPLETED sweep picks it up.
func TestAgentStackReaping_CompleteTaskStopsIdleSessionStopByTaskIDMisses(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-idle-complete", "session-idle-complete", models.TaskSessionStateIdle)
	seedExecutorRunning(t, repo, "session-idle-complete", "task-idle-complete", "exec-idle-complete")

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-idle-complete", v1.TaskStateInProgress)
	stopCalls := make(chan stopAgentCall, 1)
	svc := newReapingTestService(t, repo, taskRepo, stopCalls)
	svc.executor = executor.NewExecutor(svc.agentManager, repo, testLogger(), executor.ExecutorConfig{})

	require.NoError(t, svc.CompleteTask(ctx, "task-idle-complete"))

	call := waitForAgentStackStop(t, stopCalls)
	require.Equal(t, "exec-idle-complete", call.ExecutionID)
	require.Equal(t, stopReasonAgentStackTaskCompleted, call.Reason)
	require.Equal(t, v1.TaskStateCompleted, taskStateFromMock(taskRepo, "task-idle-complete"))
}

// A failed REVIEW write must not swallow the user's explicit stop: the agents
// still come down, and the state error stays non-fatal.
func TestStopTask_StopsAgentsWhenReviewWriteFails(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-stop-write-fail", "session-stop-write-fail", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "session-stop-write-fail", "task-stop-write-fail", "exec-stop-write-fail")

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-stop-write-fail", v1.TaskStateInProgress)
	taskRepo.updateTaskStateErr = errors.New("state write failed")

	stopCalls := make(chan stopAgentCall, 1)
	svc := newReapingTestService(t, repo, taskRepo, stopCalls)
	svc.executor = executor.NewExecutor(svc.agentManager, repo, testLogger(), executor.ExecutorConfig{})

	require.NoError(t, svc.StopTask(ctx, "task-stop-write-fail", "test stop", false))

	call := waitForAgentStackStop(t, stopCalls)
	require.Equal(t, "exec-stop-write-fail", call.ExecutionID)
}

func TestAgentStackReaping_StoppedEventPreservesCompletedSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-completed-event", "session-completed-event", models.TaskSessionStateCompleted)
	seedExecutorRunning(t, repo, "session-completed-event", "task-completed-event", "exec-completed-event")
	agentMgr := &mockAgentManager{isAgentRunning: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	svc.handleAgentStopped(ctx, watcher.AgentEventData{
		TaskID:           "task-completed-event",
		SessionID:        "session-completed-event",
		AgentExecutionID: "exec-completed-event",
	})

	session, err := repo.GetTaskSession(ctx, "session-completed-event")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCompleted, session.State)
}

func TestAgentStackReaping_FailsClosedForUnsafeGuards(t *testing.T) {
	tests := []struct {
		name           string
		featureOn      bool
		sessionState   models.TaskSessionState
		turnService    TurnService
		seedExecution  bool
		promptAdmitted bool
	}{
		{name: "feature disabled", sessionState: models.TaskSessionStateWaitingForInput, featureOn: false, turnService: &inactiveTurnService{}, seedExecution: true},
		{name: "running session", sessionState: models.TaskSessionStateRunning, featureOn: true, turnService: &inactiveTurnService{}, seedExecution: true},
		{name: "active turn", sessionState: models.TaskSessionStateWaitingForInput, featureOn: true, turnService: &alwaysActiveTurnService{}, seedExecution: true},
		{name: "activity unavailable", sessionState: models.TaskSessionStateWaitingForInput, featureOn: true, turnService: nil, seedExecution: true},
		{name: "execution unavailable", sessionState: models.TaskSessionStateWaitingForInput, featureOn: true, turnService: &inactiveTurnService{}, seedExecution: false},
		{name: "prompt in admission", sessionState: models.TaskSessionStateWaitingForInput, featureOn: true, turnService: &inactiveTurnService{}, seedExecution: true, promptAdmitted: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task-guard", "session-guard", tt.sessionState)
			if tt.seedExecution {
				seedExecutorRunning(t, repo, "session-guard", "task-guard", "exec-guard")
			}
			stopCalls := make(chan stopAgentCall, 1)
			svc := newReapingTestService(t, repo, newMockTaskRepo(), stopCalls)
			svc.config.AgentStackReaping = tt.featureOn
			svc.turnService = tt.turnService
			if tt.promptAdmitted {
				release := svc.beginPromptAdmission("session-guard")
				defer release()
			}

			session, err := repo.GetTaskSession(ctx, "session-guard")
			require.NoError(t, err)
			require.False(t, svc.stopIdleSessionAgentStack(ctx, session, stopReasonAgentStackIdleTTL))
			assertNoAgentStackStop(t, stopCalls)
		})
	}
}

// The admission marker is what makes the guard above reachable from the real
// prompt path: it must be held across ensureSessionRunning, and released once
// the prompt is done.
func TestAgentStackReaping_PromptAdmissionMarkerIsBalanced(t *testing.T) {
	svc := &Service{config: ServiceConfig{AgentStackReaping: true}}
	require.False(t, svc.hasPromptInAdmission("session-admit"))

	releaseFirst := svc.beginPromptAdmission("session-admit")
	releaseSecond := svc.beginPromptAdmission("session-admit")
	require.True(t, svc.hasPromptInAdmission("session-admit"))

	releaseFirst()
	require.True(t, svc.hasPromptInAdmission("session-admit"))

	releaseSecond()
	require.False(t, svc.hasPromptInAdmission("session-admit"))

	// Double release must not underflow into a negative count.
	releaseSecond()
	require.False(t, svc.hasPromptInAdmission("session-admit"))
}

func TestAgentStackReaping_StopFailureDoesNotClaimSuccess(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-stop-failure", "session-stop-failure", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "session-stop-failure", "task-stop-failure", "exec-stop-failure")
	agentMgr := &mockAgentManager{stopAgentWithReasonErr: errors.New("stop failed")}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.config.AgentStackReaping = true
	svc.turnService = &inactiveTurnService{}

	session, err := repo.GetTaskSession(ctx, "session-stop-failure")
	require.NoError(t, err)
	require.False(t, svc.stopIdleSessionAgentStack(ctx, session, stopReasonAgentStackIdleTTL))
	agentMgr.mu.Lock()
	require.Len(t, agentMgr.stopAgentWithReasonArgs, 1)
	agentMgr.mu.Unlock()
}

// newReapingTestService builds a Service with reaping on, an inactive turn
// service, and a started stack sweeper joined at test end so the detached
// sweeps stay inside the test's lifetime.
func newReapingTestService(
	t *testing.T,
	repo *sqliterepo.Repository,
	taskRepo *mockTaskRepo,
	stopCalls chan stopAgentCall,
) *Service {
	t.Helper()
	agentMgr := &mockAgentManager{
		isAgentRunning: true,
		stopAgentWithReasonFunc: func(_ context.Context, executionID, reason string, force bool) error {
			select {
			case stopCalls <- stopAgentCall{ExecutionID: executionID, Reason: reason, Force: force}:
			default:
			}
			return nil
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.config.AgentStackReaping = true
	svc.turnService = &inactiveTurnService{}
	svc.idleReaper = newIdleSessionReaper()
	svc.stackSweeper = newAgentStackSweeper()
	svc.stackSweeper.start(context.Background())
	t.Cleanup(svc.stackSweeper.stop)
	return svc
}

func waitForAgentStackStop(t *testing.T, calls <-chan stopAgentCall) stopAgentCall {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for agent stack stop")
		return stopAgentCall{}
	}
}

func assertNoAgentStackStop(t *testing.T, calls <-chan stopAgentCall) {
	t.Helper()
	select {
	case call := <-calls:
		t.Fatalf("unexpected agent stack stop: %+v", call)
	case <-time.After(100 * time.Millisecond):
	}
}

func taskStateFromMock(repo *mockTaskRepo, taskID string) v1.TaskState {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	return repo.updatedStates[taskID]
}
