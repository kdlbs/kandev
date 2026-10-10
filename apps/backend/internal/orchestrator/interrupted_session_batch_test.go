package orchestrator

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestInterruptedBatchReportsIndependentFailures(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	ctx := context.Background()
	items := []InterruptedSessionBatchItem{{TaskID: "missing-one", SessionID: "one"}, {TaskID: "missing-two", SessionID: "two"}}
	result, err := svc.ResumeInterruptedSessions(ctx, items)
	require.NoError(t, err)
	require.Equal(t, 2, result.Completed)
	require.Len(t, result.Results, 2)
	require.Equal(t, "one", result.Results[0].SessionID)
	require.Equal(t, "two", result.Results[1].SessionID)
	require.Equal(t, SessionDeliveryRecoveryBlocked, result.Results[0].Outcome)
	require.Equal(t, SessionDeliveryRecoveryBlocked, result.Results[1].Outcome)
	items[1].SessionID = items[0].SessionID
	result, err = svc.ResumeInterruptedSessions(ctx, items)
	require.Error(t, err)
	require.Nil(t, result, "duplicate selection must be refused before any continuation")
	_, err = svc.ResumeInterruptedSessions(ctx, make([]InterruptedSessionBatchItem, MaxInterruptedRecoveryBatch+1))
	require.Error(t, err)
}
