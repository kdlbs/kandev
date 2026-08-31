package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/routing"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type renewingRouteEffectRepository struct {
	mu       sync.Mutex
	renewals int
}

type countingLifecycleRepository struct {
	*sqliterepo.Repository
	mu                 sync.Mutex
	disables           int
	destinationEntries int
}

type failingStepEnterRepository struct {
	*countingLifecycleRepository
}

func (*failingStepEnterRepository) GetTaskSession(
	context.Context, string,
) (*models.TaskSession, error) {
	return nil, errors.New("injected session reload failure")
}

func (r *countingLifecycleRepository) SetSessionMetadataKey(
	ctx context.Context, sessionID, key string, value interface{},
) error {
	if enabled, ok := value.(bool); key == "plan_mode" && ok && !enabled {
		r.mu.Lock()
		r.disables++
		r.mu.Unlock()
	}
	if mode, ok := value.(string); key == models.SessionMetaKeySessionMode && ok && mode == "destination" {
		r.mu.Lock()
		r.destinationEntries++
		r.mu.Unlock()
	}
	return r.Repository.SetSessionMetadataKey(ctx, sessionID, key, value)
}

func (r *countingLifecycleRepository) destinationEntryCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.destinationEntries
}

func (r *countingLifecycleRepository) disableCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.disables
}

func (*renewingRouteEffectRepository) GetWorkflowRouteEffectByTransition(context.Context, string, int64) (routing.Effect, bool, error) {
	return routing.Effect{}, false, nil
}

func (*renewingRouteEffectRepository) GetCurrentWorkflowRouteEffect(context.Context, string, string) (routing.Effect, bool, error) {
	return routing.Effect{}, false, nil
}

func (*renewingRouteEffectRepository) ClaimWorkflowRouteEffect(context.Context, string, string, time.Time, time.Duration) (bool, error) {
	return false, nil
}

func (r *renewingRouteEffectRepository) RenewWorkflowRouteEffect(context.Context, string, string, time.Time) (bool, error) {
	r.mu.Lock()
	r.renewals++
	r.mu.Unlock()
	return true, nil
}

func (*renewingRouteEffectRepository) CompleteWorkflowRouteEffect(context.Context, string, string, time.Time) (bool, error) {
	return false, nil
}

func TestRenewRouteEffectClaimKeepsLongRunningOwnerLive(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repo := &renewingRouteEffectRepository{}
		stop := make(chan struct{})
		stopped := make(chan struct{})
		svc := &Service{logger: testLogger()}
		go svc.renewRouteEffectClaim(context.Background(), repo, "effect", "token", stop, stopped)

		time.Sleep(routeEffectLease + routeEffectLease/2)
		close(stop)
		<-stopped

		repo.mu.Lock()
		renewals := repo.renewals
		repo.mu.Unlock()
		require.GreaterOrEqual(t, renewals, 4,
			"a legitimate on_enter owner must renew well before the one-minute lease expires")
	})
}

func TestManualMoveRecoveryRetriesFreshClaimAfterLeaseExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		baseRepo := setupTestRepo(t)
		seedSession(t, baseRepo, "leased-effect-task", "leased-effect-session", "source-step")
		require.NoError(t, baseRepo.SetSessionMetadataKey(
			ctx, "leased-effect-session", "plan_mode", true,
		))
		repo := &countingLifecycleRepository{Repository: baseRepo}
		task, err := repo.GetTask(ctx, "leased-effect-task")
		require.NoError(t, err)
		task.WorkflowStepID = "destination-step"
		task.State = v1.TaskStateInProgress
		task.Metadata = map[string]interface{}{
			models.MetaKeyManualMoveLifecyclePending: map[string]interface{}{"from_step_id": "source-step"},
		}
		require.NoError(t, repo.UpdateTask(ctx, task))
		require.NoError(t, repo.RecordWorkflowRouteOperation(ctx, routing.Operation{
			ID: "leased-effect-operation", TaskID: task.ID, Producer: routing.ProducerManualMove,
			ExpectedStepID: "source-step", TargetStepID: "destination-step",
			Outcome: routing.OutcomeCommitted, TransitionID: task.WorkflowStepTransitionID,
			EffectID: "leased-effect",
		}))
		claimed, err := repo.ClaimWorkflowRouteEffect(
			ctx, "leased-effect", "dead-process", time.Now().UTC(), routeEffectLease,
		)
		require.NoError(t, err)
		require.True(t, claimed)

		steps := newMockStepGetter()
		steps.steps["source-step"] = &wfmodels.WorkflowStep{
			ID: "source-step", WorkflowID: "wf1", Name: "Source",
			Events: wfmodels.StepEvents{OnExit: []wfmodels.OnExitAction{{
				Type: wfmodels.OnExitDisablePlanMode,
			}}},
		}
		steps.steps["destination-step"] = &wfmodels.WorkflowStep{
			ID: "destination-step", WorkflowID: "wf1", Name: "Destination",
			Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{
				Type: wfmodels.OnEnterSetSessionMode, Config: map[string]interface{}{"mode": "destination"},
			}}},
		}
		svc := createTestService(baseRepo, steps, newMockTaskRepo())
		svc.repo = repo
		svc.startTaskLifecycleRetries()
		t.Cleanup(svc.stopTaskLifecycleRetries)
		session, err := repo.GetTaskSession(ctx, "leased-effect-session")
		require.NoError(t, err)

		svc.processManualMoveLifecycleWithFeederBarrier(
			ctx, task.ID, session, steps.steps["source-step"], steps.steps["destination-step"],
			"source-step", "destination-step", task.Description, 0,
		)
		require.Zero(t, repo.disableCount(),
			"a process that does not own the route effect must not execute source on_exit")
		stored, err := repo.GetTask(ctx, task.ID)
		require.NoError(t, err)
		require.Contains(t, stored.Metadata, models.MetaKeyManualMoveLifecyclePending,
			"a fresh claim owned by the crashed process must remain retryable")

		time.Sleep(routeEffectLease + time.Second)
		synctest.Wait()

		stored, err = repo.GetTask(ctx, task.ID)
		require.NoError(t, err)
		require.NotContains(t, stored.Metadata, models.MetaKeyManualMoveLifecyclePending)
		require.Contains(t, stored.Metadata, models.MetaKeyManualMoveLifecycleCompleted)
		require.Equal(t, 1, repo.disableCount(),
			"lease recovery must execute source on_exit exactly once")
		require.Equal(t, 1, repo.destinationEntryCount(),
			"lease recovery must execute destination on_enter exactly once")
		session, err = repo.GetTaskSession(ctx, "leased-effect-session")
		require.NoError(t, err)
		require.Equal(t, "destination", session.Metadata[models.SessionMetaKeySessionMode])
		var status string
		require.NoError(t, repo.DB().QueryRowContext(ctx,
			`SELECT status FROM workflow_route_effects WHERE id = ?`, "leased-effect").Scan(&status))
		require.Equal(t, routing.EffectCompleted, status)
	})
}

func TestManualMoveLifecycleDoesNotExitOrCompleteWhenStepEnterPreparationFails(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)
	seedSession(t, baseRepo, "failed-entry-task", "failed-entry-session", "source-step")
	repo := &failingStepEnterRepository{countingLifecycleRepository: &countingLifecycleRepository{
		Repository: baseRepo,
	}}
	task, err := baseRepo.GetTask(ctx, "failed-entry-task")
	require.NoError(t, err)
	task.WorkflowStepID = "destination-step"
	task.Metadata = map[string]interface{}{
		models.MetaKeyManualMoveLifecyclePending: map[string]interface{}{"from_step_id": "source-step"},
	}
	require.NoError(t, baseRepo.UpdateTask(ctx, task))

	steps := newMockStepGetter()
	steps.steps["source-step"] = &wfmodels.WorkflowStep{
		ID: "source-step", WorkflowID: "wf1", Name: "Source",
		Events: wfmodels.StepEvents{OnExit: []wfmodels.OnExitAction{{
			Type: wfmodels.OnExitDisablePlanMode,
		}}},
	}
	steps.steps["destination-step"] = &wfmodels.WorkflowStep{
		ID: "destination-step", WorkflowID: "wf1", Name: "Destination",
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{
			Type: wfmodels.OnEnterSetSessionMode, Config: map[string]interface{}{"mode": "destination"},
		}}},
	}
	svc := createTestService(baseRepo, steps, newMockTaskRepo())
	svc.repo = repo
	svc.startTaskLifecycleRetries()
	t.Cleanup(svc.stopTaskLifecycleRetries)
	session, err := baseRepo.GetTaskSession(ctx, "failed-entry-session")
	require.NoError(t, err)

	svc.processManualMoveLifecycleWithFeederBarrier(
		ctx, task.ID, session, steps.steps["source-step"], steps.steps["destination-step"],
		"source-step", "destination-step", task.Description, 0,
	)

	stored, err := baseRepo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Contains(t, stored.Metadata, models.MetaKeyManualMoveLifecyclePending)
	require.NotContains(t, stored.Metadata, models.MetaKeyManualMoveLifecycleCompleted,
		"failed destination preparation must not be persisted as a completed lifecycle")
	require.Zero(t, repo.disableCount(),
		"source on_exit must wait until destination preparation can succeed")
	require.Zero(t, repo.destinationEntryCount())
	svc.taskLifecycleRetryMu.Lock()
	_, retryScheduled := svc.taskLifecycleRetryTimers[task.ID]
	svc.taskLifecycleRetryMu.Unlock()
	require.True(t, retryScheduled)
}

func TestTaskLifecycleRetryCannotRestartAfterStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	svc := &Service{
		taskLifecycleRetryCtx:    ctx,
		taskLifecycleRetryCancel: cancel,
		taskLifecycleRetryTimers: make(map[string]*time.Timer),
	}
	svc.scheduleTaskLifecycleRetry("task-after-stop")
	svc.stopTaskLifecycleRetries()

	svc.scheduleTaskLifecycleRetry("task-after-stop")
	t.Cleanup(svc.stopTaskLifecycleRetries)

	svc.taskLifecycleRetryMu.Lock()
	defer svc.taskLifecycleRetryMu.Unlock()
	require.Nil(t, svc.taskLifecycleRetryCtx)
	require.Empty(t, svc.taskLifecycleRetryTimers)
}

// Contract coverage: Service supports Stop followed by Start, so its retry
// owner must create a fresh generation rather than retain canceled state.
func TestTaskLifecycleRetriesRestartWithFreshGeneration(t *testing.T) {
	svc := &Service{taskLifecycleRetryTimers: make(map[string]*time.Timer)}
	svc.startTaskLifecycleRetries()
	firstCtx := svc.taskLifecycleRetryCtx
	svc.stopTaskLifecycleRetries()

	svc.startTaskLifecycleRetries()
	t.Cleanup(svc.stopTaskLifecycleRetries)
	secondCtx := svc.taskLifecycleRetryCtx
	svc.scheduleTaskLifecycleRetry("task-after-restart")

	require.NotSame(t, firstCtx, secondCtx)
	svc.taskLifecycleRetryMu.Lock()
	defer svc.taskLifecycleRetryMu.Unlock()
	require.Len(t, svc.taskLifecycleRetryTimers, 1)
}

type blockingTaskLifecycleRetryRepository struct {
	sessionExecutorStore
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingTaskLifecycleRetryRepository) GetTask(
	ctx context.Context, taskID string,
) (*models.Task, error) {
	r.once.Do(func() { close(r.entered) })
	<-r.release
	return r.sessionExecutorStore.GetTask(ctx, taskID)
}

func TestStopTaskLifecycleRetriesWaitsForRunningCallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		baseRepo := setupTestRepo(t)
		ctx, cancel := context.WithCancel(context.Background())
		repo := &blockingTaskLifecycleRetryRepository{
			sessionExecutorStore: baseRepo,
			entered:              make(chan struct{}),
			release:              make(chan struct{}),
		}
		svc := createTestService(baseRepo, newMockStepGetter(), newMockTaskRepo())
		svc.repo = repo
		svc.taskLifecycleRetryCtx = ctx
		svc.taskLifecycleRetryCancel = cancel
		svc.scheduleTaskLifecycleRetry("callback-in-flight")

		time.Sleep(routeEffectLease)
		<-repo.entered
		stopped := make(chan struct{})
		go func() {
			svc.stopTaskLifecycleRetries()
			close(stopped)
		}()

		time.Sleep(time.Second)
		returnedEarly := false
		select {
		case <-stopped:
			returnedEarly = true
		default:
		}
		close(repo.release)
		<-stopped
		require.False(t, returnedEarly, "stop returned while a retry callback was still running")
	})
}

type completedDuringRouteEffectClaimRepository struct {
	sessionExecutorStore
	reads int
}

func (r *completedDuringRouteEffectClaimRepository) GetWorkflowRouteEffectByTransition(
	context.Context, string, int64,
) (routing.Effect, bool, error) {
	return routing.Effect{}, false, nil
}

func (r *completedDuringRouteEffectClaimRepository) GetCurrentWorkflowRouteEffect(
	context.Context, string, string,
) (routing.Effect, bool, error) {
	r.reads++
	status := routing.EffectPending
	if r.reads > 1 {
		status = routing.EffectCompleted
	}
	return routing.Effect{ID: "completed-during-claim", Status: status}, true, nil
}

func (*completedDuringRouteEffectClaimRepository) ClaimWorkflowRouteEffect(
	context.Context, string, string, time.Time, time.Duration,
) (bool, error) {
	return false, nil
}

func (*completedDuringRouteEffectClaimRepository) RenewWorkflowRouteEffect(
	context.Context, string, string, time.Time,
) (bool, error) {
	return false, nil
}

func (*completedDuringRouteEffectClaimRepository) CompleteWorkflowRouteEffect(
	context.Context, string, string, time.Time,
) (bool, error) {
	return false, nil
}

func TestRouteEffectCompletedDuringClaimIsNotReportedAsLeased(t *testing.T) {
	baseRepo := setupTestRepo(t)
	repo := &completedDuringRouteEffectClaimRepository{sessionExecutorStore: baseRepo}
	svc := createTestService(baseRepo, newMockStepGetter(), newMockTaskRepo())
	svc.repo = repo

	finish, claimed, err := svc.claimRouteEffectForStepEnter(
		context.Background(), "task", "destination", 0,
	)

	require.NoError(t, err)
	require.False(t, claimed)
	require.NotNil(t, finish)
}
