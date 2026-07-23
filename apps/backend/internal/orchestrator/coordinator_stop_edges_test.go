package orchestrator

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	orchestratorexec "github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestStopTaskForCoordinator_ReviewGuardsPreserveTaskState(t *testing.T) {
	tests := []struct {
		name         string
		initialState v1.TaskState
		office       bool
		archived     bool
		hasExecution bool
		wantStatus   CoordinatorTaskStopStatus
		wantSession  models.TaskSessionState
	}{
		{
			name: "Office task", initialState: v1.TaskStateInProgress, office: true, hasExecution: true,
			wantStatus: CoordinatorTaskStopStatusStopped, wantSession: models.TaskSessionStateCancelled,
		},
		{
			name: "archived task", initialState: v1.TaskStateInProgress, archived: true, hasExecution: true,
			wantStatus: CoordinatorTaskStopStatusStopped, wantSession: models.TaskSessionStateCancelled,
		},
		{
			name: "non-active task", initialState: v1.TaskStateCompleted, hasExecution: true,
			wantStatus: CoordinatorTaskStopStatusStopped, wantSession: models.TaskSessionStateCancelled,
		},
		{
			name: "not running", initialState: v1.TaskStateInProgress, hasExecution: false,
			wantStatus: CoordinatorTaskStopStatusNotRunning, wantSession: models.TaskSessionStateRunning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			baseRepo := setupTestRepo(t)
			seedTaskAndSession(t, baseRepo, "task-review-guard", "session-review-guard", models.TaskSessionStateRunning)
			var serviceRepo repoStore = baseRepo
			if tt.office {
				serviceRepo = &coordinatorStopRepoHooks{
					repoStore: baseRepo,
					getTaskFunc: func(readCtx context.Context, taskID string) (*models.Task, error) {
						task, err := baseRepo.GetTask(readCtx, taskID)
						if err != nil || task == nil {
							return task, err
						}
						copyTask := *task
						copyTask.IsFromOffice = true
						copyTask.AssigneeAgentProfileID = "office-agent"
						return &copyTask, nil
					},
				}
			}
			if tt.archived {
				require.NoError(t, baseRepo.ArchiveTask(ctx, "task-review-guard"))
			}

			teardownCalled := make(chan struct{}, 1)
			agentManager := &mockAgentManager{
				getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
					if !tt.hasExecution {
						return "", lifecycle.ErrNoExecutionForSession
					}
					return "execution-review-guard", nil
				},
				stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
					teardownCalled <- struct{}{}
					return nil
				},
			}
			taskRepo := newMockTaskRepo()
			seedMockTaskState(taskRepo, "task-review-guard", tt.initialState)
			svc := newCoordinatorStopTestService(serviceRepo, taskRepo, agentManager)

			result, err := svc.StopTaskForCoordinator(ctx, "task-review-guard")

			require.NoError(t, err)
			require.Equal(t, tt.wantStatus, result.Status)
			if tt.hasExecution {
				coordinatorStopAwaitSignal(t, teardownCalled, "guarded task runtime teardown")
			}
			session, err := baseRepo.GetTaskSession(ctx, "session-review-guard")
			require.NoError(t, err)
			require.Equal(t, tt.wantSession, session.State)
			state, history := coordinatorStopTaskStateSnapshot(taskRepo, "task-review-guard")
			require.Equal(t, tt.initialState, state)
			require.Empty(t, history, "task REVIEW guard allowed an ineligible state write")
		})
	}
}

func TestStopTaskForCoordinator_PreRegistrationLaunchCanEscape(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-launch-race", "session-launch-race", models.TaskSessionStateRunning)

	launchStarted := make(chan struct{})
	allowRegistration := make(chan struct{})
	registrationDone := make(chan error, 1)
	var allowRegistrationOnce sync.Once
	releaseRegistration := func() { allowRegistrationOnce.Do(func() { close(allowRegistration) }) }
	t.Cleanup(releaseRegistration)
	go func() {
		close(launchStarted)
		<-allowRegistration
		registrationDone <- repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID: "session-launch-race", SessionID: "session-launch-race", TaskID: "task-launch-race",
			AgentExecutionID: "execution-late", Status: "ready",
		})
	}()
	coordinatorStopAwaitSignal(t, launchStarted, "pre-registration launch start")

	lookupObserved := make(chan struct{}, 1)
	agentManager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			lookupObserved <- struct{}{}
			return "", lifecycle.ErrNoExecutionForSession
		},
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-launch-race", v1.TaskStateInProgress)
	svc := newCoordinatorStopTestService(repo, taskRepo, agentManager)

	result, err := svc.StopTaskForCoordinator(ctx, "task-launch-race")

	require.NoError(t, err)
	require.Equal(t, CoordinatorTaskStopStatusNotRunning, result.Status)
	coordinatorStopAwaitSignal(t, lookupObserved, "absent lifecycle lookup")
	releaseRegistration()
	select {
	case registrationErr := <-registrationDone:
		require.NoError(t, registrationErr)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for late execution registration")
	}
	running, err := repo.GetExecutorRunningBySessionID(ctx, "session-launch-race")
	require.NoError(t, err)
	require.Equal(t, "execution-late", running.AgentExecutionID)
	session, err := repo.GetTaskSession(ctx, "session-launch-race")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, session.State)
	agentManager.mu.Lock()
	stopCalls := append([]stopAgentCall(nil), agentManager.stopAgentWithReasonArgs...)
	agentManager.mu.Unlock()
	require.Empty(t, stopCalls, "v1 has no task-wide fence for a late registration")
	_, history := coordinatorStopTaskStateSnapshot(taskRepo, "task-launch-race")
	require.Empty(t, history)
}

func TestFallbackFreshLaunch_CoordinatorCancellationWinsBeforeResetWrite(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)
	seedTaskAndSession(
		t,
		baseRepo,
		"task-fallback-stop-race",
		"session-fallback-stop-race",
		models.TaskSessionStateWaitingForInput,
	)

	repo := &coordinatorStopRepoHooks{
		repoStore: baseRepo,
		updateFullRowCAS: func(
			writeCtx context.Context,
			session *models.TaskSession,
			expected models.TaskSessionState,
		) (bool, error) {
			changed, _, err := baseRepo.CancelActiveTaskSession(
				writeCtx,
				session.ID,
				coordinatorMCPStopReason,
			)
			require.NoError(t, err)
			require.True(t, changed)
			return baseRepo.UpdateTaskSessionIfCurrentState(writeCtx, session, expected)
		},
	}
	var launchCalled atomic.Bool
	agentManager := &mockAgentManager{
		launchAgentFunc: func(
			context.Context,
			*orchestratorexec.LaunchAgentRequest,
		) (*orchestratorexec.LaunchAgentResponse, error) {
			launchCalled.Store(true)
			return nil, errors.New("unexpected launch")
		},
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-fallback-stop-race", v1.TaskStateReview)
	svc := newCoordinatorStopTestService(repo, taskRepo, agentManager)

	err := svc.fallbackFreshLaunchOnMissingExecution(
		ctx,
		"task-fallback-stop-race",
		"session-fallback-stop-race",
		"replacement prompt",
		false,
		nil,
		nil,
		nil,
	)

	require.ErrorIs(t, err, orchestratorexec.ErrSessionStateSuperseded)
	require.False(t, launchCalled.Load(), "cancelled session started replacement work")
	session, getErr := baseRepo.GetTaskSession(ctx, "session-fallback-stop-race")
	require.NoError(t, getErr)
	require.Equal(t, models.TaskSessionStateCancelled, session.State)
}

func TestFallbackFreshLaunch_DoesNotResetCancellationObservedBeforeGuard(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(
		t,
		repo,
		"task-fallback-already-stopped",
		"session-fallback-already-stopped",
		models.TaskSessionStateWaitingForInput,
	)
	changed, _, err := repo.CancelActiveTaskSession(
		ctx,
		"session-fallback-already-stopped",
		coordinatorMCPStopReason,
	)
	require.NoError(t, err)
	require.True(t, changed)
	cancelled, err := repo.GetTaskSession(ctx, "session-fallback-already-stopped")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCancelled, cancelled.State)

	var launchCalled atomic.Bool
	manager := &mockAgentManager{
		launchAgentFunc: func(
			context.Context,
			*orchestratorexec.LaunchAgentRequest,
		) (*orchestratorexec.LaunchAgentResponse, error) {
			launchCalled.Store(true)
			return nil, errors.New("unexpected launch")
		},
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-fallback-already-stopped", v1.TaskStateReview)
	svc := newCoordinatorStopTestService(repo, taskRepo, manager)

	err = svc.fallbackFreshLaunchOnMissingExecution(
		ctx,
		"task-fallback-already-stopped",
		"session-fallback-already-stopped",
		"replacement prompt",
		false,
		nil,
		nil,
		nil,
	)

	require.ErrorIs(t, err, orchestratorexec.ErrSessionStateSuperseded)
	require.False(t, launchCalled.Load(), "cancelled session started replacement work")
	session, getErr := repo.GetTaskSession(ctx, "session-fallback-already-stopped")
	require.NoError(t, getErr)
	require.Equal(t, models.TaskSessionStateCancelled, session.State)
}

func coordinatorStopAddSession(
	t *testing.T,
	repo *sqliterepo.Repository,
	taskID, sessionID string,
	state models.TaskSessionState,
) {
	t.Helper()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: state, StartedAt: now, UpdatedAt: now,
	}))
}

func coordinatorStopAwaitSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func coordinatorStopAwaitCall(
	t *testing.T,
	done <-chan coordinatorStopCallOutcome,
) coordinatorStopCallOutcome {
	t.Helper()
	select {
	case outcome := <-done:
		return outcome
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for coordinator stop call")
		return coordinatorStopCallOutcome{}
	}
}

func coordinatorStopWaitForGuardRefs(t *testing.T, svc *Service, sessionID string, want int) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		svc.cancelInFlightMu.Lock()
		guard := svc.cancelInFlight[sessionID]
		refs := 0
		if guard != nil {
			refs = guard.refs
		}
		svc.cancelInFlightMu.Unlock()
		if refs >= want {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("timed out waiting for %d cancel guard references; got %d", want, refs)
		default:
			runtime.Gosched()
		}
	}
}

func coordinatorStopWaitForGuardReleased(t *testing.T, svc *Service, sessionID string) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		svc.cancelInFlightMu.Lock()
		_, exists := svc.cancelInFlight[sessionID]
		svc.cancelInFlightMu.Unlock()
		if !exists {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("timed out waiting for cancel guard %q release", sessionID)
		default:
			runtime.Gosched()
		}
	}
}

func coordinatorStopTaskStateSnapshot(
	repo *mockTaskRepo,
	taskID string,
) (v1.TaskState, []v1.TaskState) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	state := v1.TaskState("")
	if task := repo.tasks[taskID]; task != nil {
		state = task.State
	}
	return state, append([]v1.TaskState(nil), repo.stateHistory[taskID]...)
}
