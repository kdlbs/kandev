package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/workflow/routing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowRoutingSchemaReplayAndCommittedReadback(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	require.NoError(t, repo.initWorkflowRoutingSchema())
	require.NoError(t, repo.initWorkflowRoutingSchema())
	ctx := context.Background()
	task := createStepTransitionsTestTask(t, repo, "task-route-readback", "workflow-route", "step-pr")

	operation := routing.Operation{
		ID: "route-operation-1", TaskID: task.ID, WorkspaceID: task.WorkspaceID,
		Producer: routing.ProducerManualMove, ExpectedStepID: "step-pr",
		ObservedStepID: "step-pr", TargetStepID: "step-done",
		SessionID: "session-route", ActorKind: "agent", ActorID: "session-route",
	}
	task.WorkflowStepID = "step-done"
	require.NoError(t, repo.UpdateTask(routing.WithOperation(ctx, operation), task))

	var outcome, effectID string
	var transitionID int64
	require.NoError(t, repo.db.QueryRowContext(ctx, `
		SELECT outcome, transition_id, effect_id
		FROM workflow_route_operations WHERE id = ?
	`, operation.ID).Scan(&outcome, &transitionID, &effectID))
	assert.Equal(t, string(routing.OutcomeCommitted), outcome)
	assert.NotZero(t, transitionID)
	assert.Equal(t, operation.ID+":destination-entry", effectID)

	// A retry cannot rewrite the terminal stored outcome with a stale result.
	operation.Outcome = routing.OutcomeStaleSource
	require.NoError(t, repo.RecordWorkflowRouteOperation(ctx, operation))
	require.NoError(t, repo.db.QueryRowContext(ctx, `SELECT outcome FROM workflow_route_operations WHERE id = ?`, operation.ID).Scan(&outcome))
	assert.Equal(t, string(routing.OutcomeCommitted), outcome)

	var effects int
	require.NoError(t, repo.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_route_effects WHERE operation_id = ?`, operation.ID).Scan(&effects))
	assert.Equal(t, 1, effects)
}
