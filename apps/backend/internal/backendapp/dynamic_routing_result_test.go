package backendapp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/task/models"
)

func TestRouteActionResultUsesAuthoritativeRouteState(t *testing.T) {
	ctx := context.Background()
	repo := newDynamicRoutingCancelRaceRepo(t)
	seedDynamicRoutingCancelRaceSession(t, repo, "session-1", models.TaskSessionStateWaitingForInput)

	require.NoError(t, repo.SaveRouteState(ctx, dynamicruntime.RouteState{
		SessionID:          "session-1",
		LogicalProfileID:   "logical-authoritative",
		ExecutionProfileID: "execution-authoritative",
		Generation:         7,
		ProfileVersion:     4,
		Status:             "retrying",
		PolicyStateJSON:    `{"retry_ordinal":2}`,
	}))

	result, err := repo.GetTaskSession(ctx, "session-1")
	require.NoError(t, err)
	got := routeActionResult(ctx, repo, result)

	require.Equal(t, "logical-authoritative", got.LogicalProfileID)
	require.Equal(t, "execution-authoritative", got.ExecutionProfileID)
	require.Equal(t, int64(7), got.RouteGeneration)
	require.Equal(t, int64(4), got.ProfileVersion)
	require.Equal(t, "retrying", got.State)
}
