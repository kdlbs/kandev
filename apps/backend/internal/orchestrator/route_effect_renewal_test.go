package orchestrator

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/routing"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type renewingRouteEffectRepository struct {
	mu       sync.Mutex
	renewals int
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
		repo := setupTestRepo(t)
		seedSession(t, repo, "leased-effect-task", "leased-effect-session", "source-step")
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
		steps.steps["source-step"] = &wfmodels.WorkflowStep{ID: "source-step", WorkflowID: "wf1", Name: "Source"}
		steps.steps["destination-step"] = &wfmodels.WorkflowStep{
			ID: "destination-step", WorkflowID: "wf1", Name: "Destination",
			Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{
				Type: wfmodels.OnEnterSetSessionMode, Config: map[string]interface{}{"mode": "destination"},
			}}},
		}
		svc := createTestService(repo, steps, newMockTaskRepo())
		t.Cleanup(svc.stopTaskLifecycleRetries)
		session, err := repo.GetTaskSession(ctx, "leased-effect-session")
		require.NoError(t, err)

		svc.processManualMoveLifecycleWithFeederBarrier(
			ctx, task.ID, session, steps.steps["source-step"], steps.steps["destination-step"],
			"source-step", "destination-step", task.Description, 0,
		)
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
		session, err = repo.GetTaskSession(ctx, "leased-effect-session")
		require.NoError(t, err)
		require.Equal(t, "destination", session.Metadata[models.SessionMetaKeySessionMode])
		var status string
		require.NoError(t, repo.DB().QueryRowContext(ctx,
			`SELECT status FROM workflow_route_effects WHERE id = ?`, "leased-effect").Scan(&status))
		require.Equal(t, routing.EffectCompleted, status)
	})
}
