package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func cancelTargetService(t *testing.T, executionID string, turn *models.Turn) *Service {
	t.Helper()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-cancel", "session-cancel", models.TaskSessionStateRunning)
	manager := &mockAgentManager{getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
		return executionID, nil
	}}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.turnService = &failingActiveTurnLookup{turn: turn}
	return svc
}

func TestObserveCancelTargetReadsTheLiveExecutionAndTurn(t *testing.T) {
	svc := cancelTargetService(t, "exec-1", &models.Turn{ID: "turn-1"})

	target, err := svc.ObserveCancelTarget(context.Background(), "session-cancel")

	require.NoError(t, err)
	require.Equal(t, CancelTarget{ExecutionID: "exec-1", TurnID: "turn-1"}, target)
}

// A turn that finished and was replaced after the observation shares the
// execution ID, so only the turn comparison keeps the new work alive.
func TestCancelAgentIfTargetRefusesAReplacedTurn(t *testing.T) {
	svc := cancelTargetService(t, "exec-1", &models.Turn{ID: "turn-2"})
	ctx := context.WithValue(context.Background(), cancelTargetContextKey{}, CancelTarget{ExecutionID: "exec-1", TurnID: "turn-1"})

	require.ErrorIs(t, svc.verifyCancelTarget(ctx, "session-cancel"), ErrCancelTargetChanged)
}

func TestCancelAgentIfTargetRefusesAReplacementExecution(t *testing.T) {
	svc := cancelTargetService(t, "exec-2", &models.Turn{ID: "turn-1"})
	ctx := context.WithValue(context.Background(), cancelTargetContextKey{}, CancelTarget{ExecutionID: "exec-1", TurnID: "turn-1"})

	require.ErrorIs(t, svc.verifyCancelTarget(ctx, "session-cancel"), ErrCancelTargetChanged)
}

func TestVerifyCancelTargetAllowsTheObservedTargetAndUnboundCancels(t *testing.T) {
	svc := cancelTargetService(t, "exec-1", &models.Turn{ID: "turn-1"})
	bound := context.WithValue(context.Background(), cancelTargetContextKey{}, CancelTarget{ExecutionID: "exec-1", TurnID: "turn-1"})

	require.NoError(t, svc.verifyCancelTarget(bound, "session-cancel"))
	require.NoError(t, svc.verifyCancelTarget(context.Background(), "session-cancel"))
}
