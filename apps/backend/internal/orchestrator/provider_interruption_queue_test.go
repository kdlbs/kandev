package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type continuationUnreadableQueue struct{ messagequeue.Repository }

func (continuationUnreadableQueue) ListBySession(context.Context, string) ([]messagequeue.QueuedMessage, error) {
	return nil, errors.New("queue read unavailable")
}

func TestInterruptionContinuationOwnershipQueuedHumanWorkTakesPriority(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	mgr := installContinuationRestoreFixture(t, svc)
	svc.messageQueue = newAuthoritativeMemoryQueue(svc.repo.(*sqliterepo.Repository), testLogger())
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	queued, err := svc.messageQueue.QueueMessage(context.Background(), "s1", "t1", "new human request", "", "", false, nil)
	require.NoError(t, err)
	value, _ := svc.transientRetries.Load("s1")
	entry := value.(*transientRetryEntry)
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
	require.Empty(t, mgr.capturedPrompts, "automatic continuation cannot jump ahead of human work")
	require.Equal(t, 1, svc.messageQueue.GetStatus(context.Background(), "s1").Count)
	require.Equal(t, queued.ID, svc.messageQueue.GetStatus(context.Background(), "s1").Entries[0].ID)
	_, owned := svc.transientRetries.Load("s1")
	require.False(t, owned)
}

func TestInterruptionContinuationOwnershipUnknownQueueRefusesDispatch(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, _ := svc.transientRetries.Load("s1")
	svc.messageQueue = messagequeue.NewService(continuationUnreadableQueue{Repository: messagequeue.NewMemoryRepository()}, messagequeue.DefaultMaxPerSession, testLogger())
	require.Error(t, svc.validateContinuationOwner(context.Background(), "t1", "s1", value.(*transientRetryEntry)), "a failed queue read cannot attest that no human work is waiting")
}
