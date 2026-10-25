package messagequeue

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type failAfterReservationRepository struct {
	Repository
	reserved bool
}

func (r *failAfterReservationRepository) ReserveHeadIfAutoRun(ctx context.Context, sessionID string) (*QueuedMessage, bool, error) {
	msg, enabled, err := r.Repository.ReserveHeadIfAutoRun(ctx, sessionID)
	if err == nil && enabled && msg != nil {
		r.reserved = true
	}
	return msg, enabled, err
}

func (r *failAfterReservationRepository) LifecycleGeneration(ctx context.Context, taskID string) (int64, error) {
	if r.reserved {
		return 0, errors.New("lifecycle generation unavailable after reservation")
	}
	return r.Repository.LifecycleGeneration(ctx, taskID)
}

func TestReserveQueuedWithAutoRunCapturesLifecycleGenerationBeforeReservation(t *testing.T) {
	base := NewMemoryRepository()
	repo := &failAfterReservationRepository{Repository: base}
	svc := setupService(t)
	svc.repo = repo
	ctx := context.Background()

	_, _, accepted, err := svc.QueueLifecycleMessageWithCoalesceKey(
		ctx, "session-reserve-generation", "task-reserve-generation", "lifecycle", "",
		QueuedByWorkflow, false, nil, nil, "lifecycle-key", true,
	)
	require.NoError(t, err)
	require.True(t, accepted)

	reserved, ok, autoRun := svc.ReserveQueuedWithAutoRun(ctx, "session-reserve-generation")
	require.True(t, autoRun)
	require.True(t, ok, "a successful reservation must be returned even when later reads fail")
	require.NotNil(t, reserved)
	require.Equal(t, "task-reserve-generation", reserved.TaskID)
}

func TestEditLeaseDoesNotBlockAppendByAnotherQueueOwner(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	entry, err := svc.QueueMessage(ctx, "session-owner-fencing", "task-owner-fencing", "user entry", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	_, err = svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
	require.NoError(t, err)

	inserted, appended, err := svc.AppendContent(ctx, entry.SessionID, entry.TaskID, "agent entry", "", QueuedByAgent, false, nil)
	require.NoError(t, err)
	require.False(t, appended)
	require.NotEqual(t, entry.ID, inserted.ID)
	require.Equal(t, QueuedByAgent, inserted.QueuedBy)
}
