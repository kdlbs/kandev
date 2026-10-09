package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestManagedInputAcceptedExecutionPersistsExactTurnAndGeneration(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-managed-lifecycle", "session-managed-lifecycle", models.TaskSessionStateRunning)
	queue := newAuthoritativeMemoryQueue(repo, testLogger())
	storage := queue.ManagedInputStorage()
	require.NotNil(t, storage)
	identity, err := queue.ResolveSessionIdentity(ctx, "task-managed-lifecycle", "session-managed-lifecycle")
	require.NoError(t, err)
	_, _, err = storage.AdmitManagedInput(ctx, identity, messagequeue.ManagedInputRequest{
		ID: "managed-input-lifecycle", OccurrenceKey: "occurrence-one", PayloadDigest: "sha256:payload",
		Payload: "do the task", Origin: messagequeue.ManagedInputOriginHuman, ConversationRevision: 1,
	}, 10)
	require.NoError(t, err)

	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-current", nil
		},
	})
	service.messageQueue = queue
	service.SetManagedInputStorage(storage)
	queued := &messagequeue.QueuedMessage{
		ID: "managed-input-lifecycle", TaskID: identity.TaskID, SessionID: identity.SessionID,
		Metadata: map[string]interface{}{
			messagequeue.MetadataManagedInput:   true,
			messagequeue.MetadataManagedInputID: "managed-input-lifecycle",
		},
	}

	tracked, err := service.recordManagedInputAcceptance(ctx, identity, queued, "turn-current")

	require.NoError(t, err)
	require.True(t, tracked)
	receipt, err := storage.GetManagedInput(ctx, identity, queued.ID)
	require.NoError(t, err)
	require.Equal(t, messagequeue.ManagedInputStateRunning, receipt.State)
	require.Equal(t, "turn-current", receipt.TurnID)
	require.Equal(t, "execution-current", receipt.ExecutionID)
}

func TestManagedInputReadySettlesOnlyMatchingTurnAndExecution(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-managed-ready", "session-managed-ready", models.TaskSessionStateRunning)
	queue := newAuthoritativeMemoryQueue(repo, testLogger())
	storage := queue.ManagedInputStorage()
	identity, err := queue.ResolveSessionIdentity(ctx, "task-managed-ready", "session-managed-ready")
	require.NoError(t, err)
	_, _, err = storage.AdmitManagedInput(ctx, identity, messagequeue.ManagedInputRequest{
		ID: "managed-input-ready", OccurrenceKey: "occurrence-ready", PayloadDigest: "sha256:payload",
		Payload: "do the task", Origin: messagequeue.ManagedInputOriginHuman, ConversationRevision: 1,
	}, 10)
	require.NoError(t, err)
	_, _, err = storage.MarkManagedInputRunning(ctx, identity, "managed-input-ready", "turn-ready", "execution-ready")
	require.NoError(t, err)

	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	service.messageQueue = queue
	service.SetManagedInputStorage(storage)
	err = service.settleManagedInputTurn(ctx, identity.TaskID, identity.SessionID, "turn-old", "execution-ready", messagequeue.ManagedInputStateCompleted, "")
	require.NoError(t, err)
	receipt, err := storage.GetManagedInput(ctx, identity, "managed-input-ready")
	require.NoError(t, err)
	require.Equal(t, messagequeue.ManagedInputStateRunning, receipt.State, "stale turn completion must not settle this receipt")

	err = service.settleManagedInputTurn(ctx, identity.TaskID, identity.SessionID, "turn-ready", "execution-ready", messagequeue.ManagedInputStateCompleted, "")
	require.NoError(t, err)
	receipt, err = storage.GetManagedInput(ctx, identity, "managed-input-ready")
	require.NoError(t, err)
	require.Equal(t, messagequeue.ManagedInputStateCompleted, receipt.State)
}

func TestManagedInputFailedExecutionRemainsUncertainWithoutEffectEvidence(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-managed-failed", "session-managed-failed", models.TaskSessionStateRunning)
	queue := newAuthoritativeMemoryQueue(repo, testLogger())
	storage := queue.ManagedInputStorage()
	identity, err := queue.ResolveSessionIdentity(ctx, "task-managed-failed", "session-managed-failed")
	require.NoError(t, err)
	_, _, err = storage.AdmitManagedInput(ctx, identity, messagequeue.ManagedInputRequest{
		ID: "managed-input-failed", OccurrenceKey: "occurrence-failed", PayloadDigest: "sha256:payload",
		Payload: "do the task", Origin: messagequeue.ManagedInputOriginHuman, ConversationRevision: 1,
	}, 10)
	require.NoError(t, err)
	_, _, err = storage.MarkManagedInputRunning(ctx, identity, "managed-input-failed", "turn-failed", "execution-failed")
	require.NoError(t, err)

	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	service.messageQueue = queue
	service.SetManagedInputStorage(storage)
	err = service.settleManagedInputTurn(ctx, identity.TaskID, identity.SessionID, "turn-failed", "execution-failed", messagequeue.ManagedInputStateUncertain, "execution_failed_effects_unknown")
	require.NoError(t, err)
	receipt, err := storage.GetManagedInput(ctx, identity, "managed-input-failed")
	require.NoError(t, err)
	require.Equal(t, messagequeue.ManagedInputStateUncertain, receipt.State)
}

func TestManagedInputAttemptedQueueEntryBecomesUncertainInsteadOfReplaying(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-managed-recovery", "session-managed-recovery", models.TaskSessionStateWaitingForInput)
	queue := newAuthoritativeMemoryQueue(repo, testLogger())
	storage := queue.ManagedInputStorage()
	identity, err := queue.ResolveSessionIdentity(ctx, "task-managed-recovery", "session-managed-recovery")
	require.NoError(t, err)
	_, _, err = storage.AdmitManagedInput(ctx, identity, messagequeue.ManagedInputRequest{
		ID: "managed-input-recovery", OccurrenceKey: "occurrence-recovery", PayloadDigest: "sha256:payload",
		Payload: "do the task", Origin: messagequeue.ManagedInputOriginHuman, ConversationRevision: 1,
	}, 10)
	require.NoError(t, err)
	require.NoError(t, queue.SetAutoRunForSession(ctx, identity, true))

	queued, reserved, _, err := queue.ReserveQueuedWithAutoRunForSession(ctx, identity)
	require.NoError(t, err)
	require.True(t, reserved)
	require.NotNil(t, queued)
	entries, _, err := queue.SnapshotSessionForIdentity(ctx, identity)
	require.NoError(t, err)
	require.Len(t, entries, 1, "managed input remains durable until receipt linkage or acknowledgement")
	require.NoError(t, queue.MarkDeliveryAttemptedForSession(ctx, identity, []messagequeue.QueuedMessage{*queued}))
	queued.Metadata[messagequeue.MetadataDeliveryAttempted] = true

	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "", nil
		},
	})
	service.messageQueue = queue
	service.SetManagedInputStorage(storage)

	reconciled, err := service.reconcileAttemptedManagedInput(ctx, identity, queued)

	require.NoError(t, err)
	require.True(t, reconciled)
	receipt, err := storage.GetManagedInput(ctx, identity, queued.ID)
	require.NoError(t, err)
	require.Equal(t, messagequeue.ManagedInputStateUncertain, receipt.State)
	entries, _, err = queue.SnapshotSessionForIdentity(ctx, identity)
	require.NoError(t, err)
	require.Empty(t, entries)
}
