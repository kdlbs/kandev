package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
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
	readback, found, err := repo.GetWorkflowRouteOperation(ctx, operation.ID)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, operation.ID, readback.ID)
	assert.Equal(t, task.ID, readback.TaskID)
	assert.Equal(t, routing.OutcomeCommitted, readback.Outcome)
	assert.Equal(t, transitionID, readback.TransitionID)
	assert.Equal(t, effectID, readback.EffectID)
	effect, found, err := repo.GetWorkflowRouteEffect(ctx, effectID)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, routing.Effect{
		ID: effectID, OperationID: operation.ID, TaskID: task.ID,
		TransitionID: transitionID, TargetStepID: "step-done", Status: "pending",
	}, effect)

	// A retry cannot rewrite the terminal stored outcome with a stale result.
	operation.Outcome = routing.OutcomeStaleSource
	require.NoError(t, repo.RecordWorkflowRouteOperation(ctx, operation))
	require.NoError(t, repo.db.QueryRowContext(ctx, `SELECT outcome FROM workflow_route_operations WHERE id = ?`, operation.ID).Scan(&outcome))
	assert.Equal(t, string(routing.OutcomeCommitted), outcome)

	var effects int
	require.NoError(t, repo.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_route_effects WHERE operation_id = ?`, operation.ID).Scan(&effects))
	assert.Equal(t, 1, effects)
}

// TestWorkflowRouteOperationPendingToCommittedFillsTurnAndTarget reproduces
// the real two-phase step_complete flow: handleStepComplete first records a
// pending operation with a placeholder target_step_id ("") and the real
// turn_id, then the turn-end engine commit records the *same* OperationID
// with the real target_step_id but no turn_id (the committed-side context
// never carries turn_id — see executeStepTransition/finishTurn in
// event_handlers_workflow.go). Both writes must land on the same operation
// row rather than failing the ON CONFLICT identity gate, or the primary
// "agent signals done -> auto-advance" flow never commits.
func TestWorkflowRouteOperationPendingToCommittedFillsTurnAndTarget(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	task := createStepTransitionsTestTask(t, repo, "task-route-two-phase", "workflow-route", "step-work")

	const opID = "shared-step-complete-op"
	const turnID = "turn-123"

	// Phase 1: handleStepComplete's pending pre-record (handlers.go). Real
	// target is unknown yet, so TargetStepID stays "". TurnID is populated.
	require.NoError(t, repo.RecordWorkflowRouteOperation(ctx, routing.Operation{
		ID: opID, TaskID: task.ID, WorkspaceID: task.WorkspaceID,
		Producer: routing.ProducerStepComplete, ExpectedStepID: "step-work",
		ObservedStepID: "step-work", SessionID: "session-route", TurnID: turnID,
		ActorKind: "agent", ActorID: "session-route", Outcome: routing.OutcomePending,
	}))

	// Phase 2: turn-end engine commit (executeStepTransition) reuses the same
	// OperationID with the real TargetStepID, but its context never carries
	// TurnID.
	task.WorkflowStepID = "step-done"
	require.NoError(t, repo.UpdateTask(routing.WithOperation(ctx, routing.Operation{
		ID: opID, TaskID: task.ID, Producer: routing.ProducerStepComplete,
		ExpectedStepID: "step-work", TargetStepID: "step-done",
		SessionID: "session-route", ActorKind: "agent", ActorID: "session-route",
	}), task))

	var outcome, targetStepID string
	var storedTurnID sql.NullString
	var transitionID int64
	require.NoError(t, repo.db.QueryRowContext(ctx, `
		SELECT outcome, target_step_id, turn_id, transition_id
		FROM workflow_route_operations WHERE id = ?
	`, opID).Scan(&outcome, &targetStepID, &storedTurnID, &transitionID))
	assert.Equal(t, string(routing.OutcomeCommitted), outcome)
	assert.Equal(t, "step-done", targetStepID)
	assert.Equal(t, turnID, storedTurnID.String, "the pending turn_id must survive the commit fill")
	assert.NotZero(t, transitionID)

	var moved int
	require.NoError(t, repo.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM tasks WHERE id = ? AND workflow_step_id = 'step-done'
	`, task.ID).Scan(&moved))
	assert.Equal(t, 1, moved, "the task must actually advance, not roll back")
}

func TestWorkflowRouteOperationRejectsConflictingProgressiveIdentity(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*routing.Operation)
	}{
		{
			name: "workspace",
			mutate: func(operation *routing.Operation) {
				operation.WorkspaceID = "different-workspace"
			},
		},
		{
			name: "target step",
			mutate: func(operation *routing.Operation) {
				operation.TargetStepID = "step-review"
			},
		},
		{
			name: "turn",
			mutate: func(operation *routing.Operation) {
				operation.TurnID = "turn-456"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newStepTransitionsTestRepo(t)
			ctx := context.Background()
			task := createStepTransitionsTestTask(
				t, repo, "task-route-conflict-"+strings.ReplaceAll(tt.name, " ", "-"),
				"workflow-route", "step-work",
			)
			operation := routing.Operation{
				ID:     "route-conflict-" + strings.ReplaceAll(tt.name, " ", "-"),
				TaskID: task.ID, WorkspaceID: task.WorkspaceID,
				Producer: routing.ProducerStepComplete, ExpectedStepID: "step-work",
				ObservedStepID: "step-work", TargetStepID: "step-done",
				SessionID: "session-route", TurnID: "turn-123",
				ActorKind: "agent", ActorID: "session-route", Outcome: routing.OutcomePending,
			}
			require.NoError(t, repo.RecordWorkflowRouteOperation(ctx, operation))

			conflicting := operation
			tt.mutate(&conflicting)
			require.ErrorIs(t, repo.RecordWorkflowRouteOperation(ctx, conflicting), routing.ErrOperationIdentityConflict)

			stored, found, err := repo.GetWorkflowRouteOperation(ctx, operation.ID)
			require.NoError(t, err)
			require.True(t, found)
			assert.Equal(t, operation.WorkspaceID, stored.WorkspaceID)
			assert.Equal(t, operation.TargetStepID, stored.TargetStepID)
			assert.Equal(t, operation.TurnID, stored.TurnID)
		})
	}
}

func TestWorkflowRouteEffectClaimAndCrashRecovery(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	task := createStepTransitionsTestTask(t, repo, "task-route-effect", "workflow-route", "step-pr")
	op := routing.Operation{ID: "route-effect-op", TaskID: task.ID, WorkspaceID: task.WorkspaceID,
		Producer: routing.ProducerManualMove, ExpectedStepID: "step-pr", TargetStepID: "step-done"}
	task.WorkflowStepID = "step-done"
	require.NoError(t, repo.UpdateTask(routing.WithOperation(ctx, op), task))
	effectID := op.ID + ":destination-entry"
	now := time.Date(2026, 8, 30, 23, 0, 0, 0, time.UTC)

	claimed, err := repo.ClaimWorkflowRouteEffect(ctx, effectID, "worker-a", now, time.Minute)
	require.NoError(t, err)
	require.True(t, claimed)
	claimed, err = repo.ClaimWorkflowRouteEffect(ctx, effectID, "worker-b", now.Add(30*time.Second), time.Minute)
	require.NoError(t, err)
	assert.False(t, claimed, "a live owner is never duplicated")
	renewed, err := repo.RenewWorkflowRouteEffect(ctx, effectID, "worker-a", now.Add(45*time.Second))
	require.NoError(t, err)
	require.True(t, renewed)
	claimed, err = repo.ClaimWorkflowRouteEffect(ctx, effectID, "worker-b", now.Add(90*time.Second), time.Minute)
	require.NoError(t, err)
	assert.False(t, claimed, "a renewed long-running owner keeps its claim")

	claimed, err = repo.ClaimWorkflowRouteEffect(ctx, effectID, "recovery", now.Add(2*time.Minute), time.Minute)
	require.NoError(t, err)
	require.True(t, claimed, "a crashed claimant is recoverable after its lease")
	begun, err := repo.BeginWorkflowRouteEffect(ctx, effectID, "recovery", now.Add(2*time.Minute))
	require.NoError(t, err)
	require.True(t, begun)
	claimed, err = repo.ClaimWorkflowRouteEffect(ctx, effectID, "late", now.Add(10*time.Minute), time.Minute)
	require.NoError(t, err)
	assert.False(t, claimed, "an executing effect is never reclaimed after external work may have started")
	completed, err := repo.CompleteWorkflowRouteEffect(ctx, effectID, "worker-a", now.Add(2*time.Minute))
	require.NoError(t, err)
	assert.False(t, completed, "a stale claimant cannot complete a reclaimed effect")
	completed, err = repo.CompleteWorkflowRouteEffect(ctx, effectID, "recovery", now.Add(2*time.Minute))
	require.NoError(t, err)
	require.True(t, completed)
	completed, err = repo.CompleteWorkflowRouteEffect(ctx, effectID, "recovery", now.Add(3*time.Minute))
	require.NoError(t, err)
	require.True(t, completed, "same-token completion retry must read back as successful")

	claimed, err = repo.ClaimWorkflowRouteEffect(ctx, effectID, "completed-late", now.Add(10*time.Minute), time.Minute)
	require.NoError(t, err)
	assert.False(t, claimed, "completed route effects are absorbing")
}

func TestWorkflowRouteOperationIdentitySerializesDifferentTasks(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	first := createStepTransitionsTestTask(t, repo, "task-route-first", "workflow-route", "step-pr")
	second := createStepTransitionsTestTask(t, repo, "task-route-second", "workflow-route", "step-pr")

	start := make(chan struct{})
	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for _, task := range []*models.Task{first, second} {
		task := task
		go func() {
			operation := routing.Operation{
				ID: "shared-route-operation", TaskID: task.ID, WorkspaceID: task.WorkspaceID,
				Producer: routing.ProducerManualMove, ExpectedStepID: "step-pr",
				TargetStepID: "step-done", Outcome: routing.OutcomePending,
			}
			task.WorkflowStepID = "step-done"
			ready.Done()
			<-start
			results <- repo.UpdateTask(routing.WithOperation(ctx, operation), task)
		}()
	}
	ready.Wait()
	close(start)

	successes := 0
	conflicts := 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			successes++
		case strings.Contains(err.Error(), "route operation identity conflict"):
			conflicts++
		default:
			t.Fatalf("route result = %v, want success or identity conflict", err)
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, conflicts)

	var moved, transitions, operations int
	require.NoError(t, repo.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM tasks WHERE id IN (?, ?) AND workflow_step_id = 'step-done'
	`, first.ID, second.ID).Scan(&moved))
	require.NoError(t, repo.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM task_step_transitions
		WHERE task_id IN (?, ?) AND to_workflow_step_id = 'step-done'
	`, first.ID, second.ID).Scan(&transitions))
	require.NoError(t, repo.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM workflow_route_operations WHERE id = 'shared-route-operation'
	`).Scan(&operations))
	assert.Equal(t, 1, moved)
	assert.Equal(t, 1, transitions)
	assert.Equal(t, 1, operations)
}
