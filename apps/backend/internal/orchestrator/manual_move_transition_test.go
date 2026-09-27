package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/routing"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// transitionOnlyRouteEffectRepository models a task.moved delivery after the
// task has been reloaded. The task row no longer carries its transient entry
// id, so the lifecycle must use the immutable event id rather than a mutable
// current-effect lookup.
type transitionOnlyRouteEffectRepository struct {
	*sqliterepo.Repository
	currentReads int
}

func (r *transitionOnlyRouteEffectRepository) GetCurrentWorkflowRouteEffect(
	context.Context, string, string,
) (routing.Effect, bool, error) {
	r.currentReads++
	return routing.Effect{}, false, nil
}

func TestManualMoveLifecycleUsesEventTransitionForRouteEffect(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)
	seedSession(t, baseRepo, "event-transition-task", "event-transition-session", "source-step")
	repo := &transitionOnlyRouteEffectRepository{Repository: baseRepo}

	task, err := baseRepo.GetTask(ctx, "event-transition-task")
	require.NoError(t, err)
	task.WorkflowStepID = "destination-step"
	task.State = v1.TaskStateInProgress
	task.Metadata = map[string]interface{}{
		models.MetaKeyManualMoveLifecyclePending: map[string]interface{}{"from_step_id": "source-step"},
	}
	require.NoError(t, baseRepo.UpdateTask(ctx, task))
	require.Greater(t, task.WorkflowStepTransitionID, int64(0))
	require.NoError(t, baseRepo.RecordWorkflowRouteOperation(ctx, routing.Operation{
		ID: "event-transition-operation", TaskID: task.ID, Producer: routing.ProducerManualMove,
		ExpectedStepID: "source-step", TargetStepID: "destination-step",
		Outcome: routing.OutcomeCommitted, TransitionID: task.WorkflowStepTransitionID,
		EffectID: "event-transition-effect",
	}))

	steps := newMockStepGetter()
	steps.steps["source-step"] = &wfmodels.WorkflowStep{ID: "source-step", WorkflowID: "wf1", Name: "Source"}
	steps.steps["destination-step"] = &wfmodels.WorkflowStep{ID: "destination-step", WorkflowID: "wf1", Name: "Destination"}
	svc := createTestService(baseRepo, steps, newMockTaskRepo())
	svc.repo = repo
	session, err := baseRepo.GetTaskSession(ctx, "event-transition-session")
	require.NoError(t, err)

	svc.processManualMoveLifecycleWithFeederBarrier(
		ctx, task.ID, session, steps.steps["source-step"], steps.steps["destination-step"],
		"source-step", "destination-step", task.Description, task.WorkflowStepTransitionID,
	)

	effect, found, err := baseRepo.GetWorkflowRouteEffect(ctx, "event-transition-effect")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, routing.EffectCompleted, effect.Status,
		"manual lifecycle must claim the effect belonging to its task.moved event")
	require.Zero(t, repo.currentReads,
		"a task.moved delivery must not replace its immutable entry id with a current-effect lookup")
}
