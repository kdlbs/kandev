package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFreshStartSessionCreatedCommitsReplacementForOwningExecution(t *testing.T) {
	ctx := context.Background()
	svc, repo, data := seedCommittedFirstGeneration(t)
	seedExecutorRunning(t, repo, freshStartSessionID, freshStartTaskID, "fresh-start-execution")

	svc.handleACPSessionCreated(ctx, data)

	current, err := repo.GetCurrentHarnessSessionGeneration(ctx, freshStartSessionID, data.DeliveryIncarnationID)
	require.NoError(t, err)
	require.EqualValues(t, 2, current.Generation)
	require.Equal(t, "replacement-native", current.NativeSessionID)
}

func TestFreshStartSessionCreatedFromRotatedExecutionKeepsGeneration(t *testing.T) {
	ctx := context.Background()
	svc, repo, data := seedCommittedFirstGeneration(t)
	seedExecutorRunning(t, repo, freshStartSessionID, freshStartTaskID, "current-execution")

	svc.handleACPSessionCreated(ctx, data)

	current, err := repo.GetCurrentHarnessSessionGeneration(ctx, freshStartSessionID, data.DeliveryIncarnationID)
	require.NoError(t, err)
	require.EqualValues(t, 1, current.Generation, "a rejected resume token must not commit the replacement")
	require.Equal(t, "previous-native", current.NativeSessionID)
}
