package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func TestInterruptionContinuationSettlementManagedInputIsUncertain(t *testing.T) {
	ctx := context.Background()
	svc, _, data := continuationFailureFixture(t)
	installContinuationRestoreFixture(t, svc)
	turn, err := svc.turnService.StartTurn(ctx, "s1")
	require.NoError(t, err)
	queue := newAuthoritativeMemoryQueue(svc.repo.(*sqliterepo.Repository), testLogger())
	svc.messageQueue = queue
	storage := queue.ManagedInputStorage()
	svc.SetManagedInputStorage(storage)
	identity, err := queue.ResolveSessionIdentity(ctx, "t1", "s1")
	require.NoError(t, err)
	_, _, err = storage.AdmitManagedInput(ctx, identity, messagequeue.ManagedInputRequest{
		ID: "interrupted-input", OccurrenceKey: "interrupted-input", PayloadDigest: "sha256:fixed",
		Payload: "test original request", Origin: messagequeue.ManagedInputOriginHuman, ConversationRevision: 1,
	}, 10)
	require.NoError(t, err)
	_, _, err = storage.MarkManagedInputRunning(ctx, identity, "interrupted-input", turn.ID, "execution-1")
	require.NoError(t, err)
	require.True(t, svc.handleTransientFailure(ctx, data))
	receipt, err := storage.GetManagedInput(ctx, identity, "interrupted-input")
	require.NoError(t, err)
	require.Equal(t, messagequeue.ManagedInputStateUncertain, receipt.State, "interrupted input must not remain running or be replayed")
}
