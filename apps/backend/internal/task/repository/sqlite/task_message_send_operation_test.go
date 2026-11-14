package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func testSendOperation(id, operationID, senderTaskID, senderSessionID string) *models.TaskMessageSendOperation {
	return &models.TaskMessageSendOperation{
		ID: id, OperationID: operationID,
		SenderTaskID: senderTaskID, SenderSessionID: senderSessionID,
		TargetTaskID: "target-task", TargetSessionID: "target-session",
		RequestedPrompt: "requested prompt", RequestedDeliveryMode: "queued",
		RequestedSessionID: "requested-session", RequestedQuestionID: "question-id",
	}
}

func TestSendOperationClaimSettlementAndSenderScopedReadback(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	first := testSendOperation("row-first", "operation-1", "sender-task-1", "sender-session-1")
	require.NoError(t, repo.ClaimSendOperation(ctx, first))
	require.Equal(t, models.SendOperationStatePending, first.State)

	duplicate := testSendOperation("row-duplicate", "operation-1", "sender-task-1", "sender-session-1")
	if err := repo.ClaimSendOperation(ctx, duplicate); !errors.Is(err, models.ErrSendOperationNotClaimed) {
		t.Fatalf("duplicate claim error = %v, want ErrSendOperationNotClaimed", err)
	}

	otherSender := testSendOperation("row-other-sender", "operation-1", "sender-task-2", "sender-session-1")
	require.NoError(t, repo.ClaimSendOperation(ctx, otherSender), "operation IDs are scoped to the sender task and session")

	if _, err := repo.GetSendOperation(ctx, "sender-task-1", "different-session", "operation-1"); !errors.Is(err, models.ErrSendOperationNotFound) {
		t.Fatalf("read from another session error = %v, want ErrSendOperationNotFound", err)
	}

	loaded, err := repo.GetSendOperation(ctx, "sender-task-1", "sender-session-1", "operation-1")
	require.NoError(t, err)
	require.Equal(t, first.ID, loaded.ID)
	require.Equal(t, first.RequestedPrompt, loaded.RequestedPrompt)
	require.Equal(t, models.SendOperationStatePending, loaded.State)
	require.Nil(t, loaded.SettledAt)

	updated, err := repo.SettleSendOperation(ctx,
		"sender-task-1", "sender-session-1", "operation-1", models.SendOperationStateCommitted,
		"target-session-final", models.SendOperationDeliverySent, "message-1", "", "")
	require.NoError(t, err)
	require.True(t, updated)

	updated, err = repo.SettleSendOperation(ctx,
		"sender-task-1", "sender-session-1", "operation-1", models.SendOperationStateFailed,
		"", "", "", "", models.SendOperationFailureOutcomeUnknown)
	require.NoError(t, err)
	require.False(t, updated, "a terminal receipt must settle only once")

	loaded, err = repo.GetSendOperation(ctx, "sender-task-1", "sender-session-1", "operation-1")
	require.NoError(t, err)
	require.Equal(t, models.SendOperationStateCommitted, loaded.State)
	require.Equal(t, "target-session-final", loaded.TargetSessionID)
	require.Equal(t, models.SendOperationDeliverySent, loaded.DeliveryStatus)
	require.Equal(t, "message-1", loaded.MessageID)
	require.NotNil(t, loaded.SettledAt)
	require.False(t, loaded.RetrySafe())
}

func TestSendOperationPruneMarksOutcomeUnknownAndRemovesExpiredReceipt(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	stale := testSendOperation("row-stale", "operation-stale", "sender-stale", "session-stale")
	require.NoError(t, repo.ClaimSendOperation(ctx, stale))
	expired := testSendOperation("row-expired", "operation-expired", "sender-expired", "session-expired")
	require.NoError(t, repo.ClaimSendOperation(ctx, expired))
	updated, err := repo.SettleSendOperation(ctx,
		"sender-expired", "session-expired", "operation-expired", models.SendOperationStateCommitted,
		"target-session", models.SendOperationDeliverySent, "message-expired", "", "")
	require.NoError(t, err)
	require.True(t, updated)

	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE task_message_send_operations SET created_at = ? WHERE id = ?
	`), time.Now().UTC().Add(-sendOperationStalePending-time.Hour), stale.ID)
	require.NoError(t, err)
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE task_message_send_operations SET created_at = ? WHERE id = ?
	`), time.Now().UTC().Add(-sendOperationRetention-time.Hour), expired.ID)
	require.NoError(t, err)

	settled, removed, err := repo.PruneSendOperations(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, settled)
	require.Equal(t, 1, removed)

	unknown, err := repo.GetSendOperation(ctx, "sender-stale", "session-stale", "operation-stale")
	require.NoError(t, err)
	require.Equal(t, models.SendOperationStateFailed, unknown.State)
	require.Equal(t, models.SendOperationFailureOutcomeUnknown, unknown.FailureCode)
	require.False(t, unknown.RetrySafe(), "an interrupted dispatch must not be retried automatically")
	if _, err := repo.GetSendOperation(ctx, "sender-expired", "session-expired", "operation-expired"); !errors.Is(err, models.ErrSendOperationNotFound) {
		t.Fatalf("expired readback error = %v, want ErrSendOperationNotFound", err)
	}
}

func TestSendOperationSchemaReplaysWithoutLosingReceipts(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	operation := testSendOperation("row-schema", "operation-schema", "sender-schema", "session-schema")
	require.NoError(t, repo.ClaimSendOperation(ctx, operation))

	replayedRepo, err := NewWithDB(repo.db, repo.ro, nil)
	require.NoError(t, err)
	loaded, err := replayedRepo.GetSendOperation(ctx, operation.SenderTaskID, operation.SenderSessionID, operation.OperationID)
	require.NoError(t, err)
	require.Equal(t, operation.ID, loaded.ID)
	require.Equal(t, models.SendOperationStatePending, loaded.State)

	var indexCount int
	require.NoError(t, repo.db.Get(&indexCount, `
		SELECT COUNT(*) FROM sqlite_master
		 WHERE type = 'index' AND name IN (
			'uniq_task_message_send_operations_identity',
			'idx_task_message_send_operations_retention'
		 )
	`))
	require.Equal(t, 2, indexCount)
}
