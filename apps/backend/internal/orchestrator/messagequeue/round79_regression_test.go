package messagequeue

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReserveQueuedWithAutoRunCapturesReservedHeadGeneration(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	_, err := svc.PurgeTask(ctx, "task-reserved-head")
	require.NoError(t, err)
	_, _, accepted, err := svc.QueueLifecycleMessageWithCoalesceKey(
		ctx, "session-reserved-head", "task-reserved-head", "head", "",
		QueuedByWorkflow, false, nil, nil, "head-key", true,
	)
	require.NoError(t, err)
	require.True(t, accepted)
	reserved, ok := svc.ReserveQueued(ctx, "session-reserved-head")
	require.True(t, ok)
	require.NotNil(t, reserved)

	_, _, accepted, err = svc.QueueLifecycleMessageWithCoalesceKey(
		ctx, "session-reserved-head", "task-following", "following", "",
		QueuedByWorkflow, false, nil, nil, "following-key", true,
	)
	require.NoError(t, err)
	require.True(t, accepted)

	recovered, ok, autoRun := svc.ReserveQueuedWithAutoRun(ctx, "session-reserved-head")
	require.True(t, autoRun)
	require.True(t, ok)
	require.NotNil(t, recovered)
	require.Equal(t, "task-reserved-head", recovered.TaskID)
	_, generation, captured := recovered.ReservationGenerations()
	require.True(t, captured)
	require.Equal(t, int64(1), generation)
}
