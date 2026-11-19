package orchestrator

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// The production callback path persists the launch row through
// persistFullTaskSessionIfCurrent, not through the executor helper. A parked
// workflow switch must be recognized there too.
func TestPersistFullTaskSessionIfCurrent_WorkflowParkIsSuperseded(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	require.NoError(t, repo.SetSessionMetadataKey(
		ctx, "session1", models.SessionMetaKeyWorkflowParking,
		models.WorkflowParking{
			Stamp:           "park-1",
			ParkedAt:        time.Now().UTC(),
			SourceSessionID: "session1",
			DestinationID:   "session2",
		},
	))
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	stale := &models.TaskSession{ID: "session1", TaskID: "task1", State: models.TaskSessionStateCreated}

	err := svc.persistFullTaskSessionIfCurrent(ctx, stale, models.TaskSessionStateCreated)
	if !executor.IsSessionStateSwitched(err) {
		t.Fatalf("persistFullTaskSessionIfCurrent() error = %v, want SessionStateSwitchedError", err)
	}
}

func TestPersistFullTaskSessionIfCurrent_UnexpectedStateWithoutSwitchIsError(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	stale := &models.TaskSession{ID: "session1", TaskID: "task1", State: models.TaskSessionStateCreated}

	err := svc.persistFullTaskSessionIfCurrent(ctx, stale, models.TaskSessionStateCreated)
	if err == nil || executor.IsSessionStateSwitched(err) {
		t.Fatalf("persistFullTaskSessionIfCurrent() error = %v, want a plain unexpected-state failure", err)
	}
}

// A losing launch must not mark the task FAILED or the session FAILED: the
// workflow switch still owns the session and will launch the replacement.
func TestHandleSessionLaunchFailure_WorkflowSwitchSupersededKeepsTaskActive(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	require.NoError(t, repo.SetSessionMetadataKey(
		ctx, "session1", models.SessionMetaKeyWorkflowParking,
		models.WorkflowParking{
			Stamp:           "park-1",
			ParkedAt:        time.Now().UTC(),
			SourceSessionID: "session1",
		},
	))
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", State: v1.TaskStateInProgress}
	svc := createTestService(repo, newMockStepGetter(), taskRepo)

	err := svc.handleSessionLaunchFailure(
		ctx, "task1", "session1",
		&executor.SessionStateSwitchedError{SessionID: "session1", State: models.TaskSessionStateWaitingForInput},
	)
	require.Error(t, err)

	session, getErr := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, getErr)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)

	taskRepo.mu.Lock()
	defer taskRepo.mu.Unlock()
	require.Zero(t, taskRepo.stateWrites["task1"], "a superseded launch must not write a task state")
}

// gatedSessionPersistRepo holds the first launch-state write of one session
// until released, modelling a launch whose workspace is already materialized
// while its session row is not yet persisted (the incident window).
type gatedSessionPersistRepo struct {
	*sqliterepo.Repository
	sessionID atomic.Value
	once      sync.Once
	reached   chan struct{}
	release   chan struct{}
}

func (r *gatedSessionPersistRepo) UpdateTaskSessionIfCurrentState(
	ctx context.Context, session *models.TaskSession, expected models.TaskSessionState,
) (bool, error) {
	if id, _ := r.sessionID.Load().(string); id != "" && session.ID == id {
		r.once.Do(func() {
			close(r.reached)
			<-r.release
		})
	}
	return r.Repository.UpdateTaskSessionIfCurrentState(ctx, session, expected)
}

// A launch that was still persisting its runtime when the real workflow
// profile switch parked its session must exit silently: the switch completes,
// the replacement session owns the step profile, the parked source stays
// answerable, and the task is never written FAILED.
func TestPrepareTaskSession_RealWorkflowSwitchDuringLaunchCompletes(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "existing-session", models.TaskSessionStateCreated)

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{
		ID: "task1", WorkspaceID: "ws1", WorkflowID: "wf1",
		Title: "Test Task", Description: "prepare", State: v1.TaskStateInProgress,
	}

	gated := &gatedSessionPersistRepo{Repository: repo, reached: make(chan struct{}), release: make(chan struct{})}
	stopCalled := make(chan struct{})
	var launches atomic.Int32
	var stopCalledOnce sync.Once
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(launchCtx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			executionID := "exec-step-" + req.SessionID
			if launches.Add(1) == 1 {
				gated.sessionID.Store(req.SessionID)
				executionID = "exec-old"
			}
			// The lifecycle registers the runtime as soon as it launches.
			now := time.Now().UTC()
			if err := repo.UpsertExecutorRunning(launchCtx, &models.ExecutorRunning{
				ID: "running-" + req.SessionID, SessionID: req.SessionID, TaskID: "task1",
				AgentExecutionID: executionID, CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				t.Errorf("register launching runtime: %v", err)
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: executionID}, nil
		},
		stopAgentWithReasonFunc: func(_ context.Context, executionID string, _ string, _ bool) error {
			if executionID == "exec-old" {
				stopCalledOnce.Do(func() { close(stopCalled) })
			}
			return nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	exec := withTestExecutorActor(executor.NewExecutor(agentMgr, gated, testLogger(), executor.ExecutorConfig{}))
	svc.executor = exec
	svc.scheduler = scheduler.NewScheduler(queue.NewTaskQueue(100), exec, taskRepo, testLogger(), scheduler.SchedulerConfig{})

	sourceID, err := svc.PrepareTaskSession(ctx, "task1", "profile-old", "", "", "step1", true)
	require.NoError(t, err)
	select {
	case <-gated.reached:
	case <-time.After(5 * time.Second):
		t.Fatal("prepared launch did not reach runtime persistence")
	}

	// The step's profile differs from the launching session's: run the real
	// switch with the park end policy while the first launch is in flight.
	source, err := repo.GetTaskSession(ctx, sourceID)
	require.NoError(t, err)
	replacement, err := svc.switchSessionForStepWithPolicies(
		ctx, "task1", source, "profile-step",
		models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark,
	)
	require.NoError(t, err)
	require.NotNil(t, replacement)
	require.NotEqual(t, sourceID, replacement.ID)

	parked, err := repo.GetTaskSession(ctx, sourceID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, parked.State)
	marker, ok := models.LoadWorkflowParking(parked.Metadata)
	require.True(t, ok, "the real switch records the parking marker")
	require.Equal(t, sourceID, marker.SourceSessionID)

	// Let the stale launch persist into the parked session.
	close(gated.release)
	select {
	case <-stopCalled:
	case <-time.After(5 * time.Second):
		t.Fatal("the stale launch's runtime was not stopped")
	}

	require.Never(t, func() bool {
		taskRepo.mu.Lock()
		defer taskRepo.mu.Unlock()
		for _, state := range taskRepo.stateHistory["task1"] {
			if state == v1.TaskStateFailed {
				return true
			}
		}
		return false
	}, 500*time.Millisecond, 10*time.Millisecond, "the superseded launch must not fail the task")

	parked, err = repo.GetTaskSession(ctx, sourceID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, parked.State,
		"the parked source stays answerable instead of FAILED")
	stepSession, err := repo.GetTaskSession(ctx, replacement.ID)
	require.NoError(t, err)
	require.Equal(t, "profile-step", stepSession.AgentProfileID)
	require.True(t, stepSession.IsPrimary, "the step profile session owns the task")
	require.NotEqual(t, models.TaskSessionStateFailed, stepSession.State)
}
