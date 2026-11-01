package changes

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/stretchr/testify/require"
)

func TestCoordinatorCleanupRespectsAdmissionDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := newCoordinatorStore()
		policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{Enabled: true}}
		client := &coordinatorCheckpointClient{deleteErr: errors.New("executor unavailable")}
		coordinator := NewCoordinator(store, coordinatorTurnReader{}, policy, NewContentService(store, nil), nil)
		admission := coordinatorAdmission()
		require.NoError(t, coordinator.Admit(context.Background(), admission, client))
		require.NoError(t, coordinator.Finish(context.Background(), Terminal{Admission: admission, At: time.Now()}, client))
		require.True(t, store.repositoryRows[turnChangeSetID(admission.TurnID)][0].CleanupPending)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		started := time.Now()
		_ = coordinator.Admit(ctx, admission, &blockedCleanupClient{client})
		require.LessOrEqual(t, time.Since(started), 20*time.Millisecond, "old cleanup intents must not outlive prompt admission")
	})
}

type blockedCleanupClient struct{ *coordinatorCheckpointClient }

func (c *blockedCleanupClient) DeleteTurnCheckpoint(ctx context.Context, _ turnchanges.CheckpointDeleteRequest) error {
	<-ctx.Done()
	return ctx.Err()
}
