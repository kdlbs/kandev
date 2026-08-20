package messagequeue

import (
	"context"
	"expvar"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeliveryLedgerCreateOrGetIsIdempotentPerSourceTurn(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ledger, ok := repo.(DeliveryLedger)
	require.True(t, ok, "queue repository must expose durable delivery storage")
	duplicates := expvar.Get("administrative_turn_message_delivery_duplicates_total").(*expvar.Int)

	first, created, err := ledger.CreateOrGetDelivery(context.Background(), Delivery{
		SenderTaskID:    "source-task",
		SenderSessionID: "source-session",
		SourceTurnID:    "source-turn",
		IdempotencyKey:  "report-v1",
		TargetTaskID:    "target-task",
		TargetSessionID: "target-session",
		Content:         "review is ready",
		State:           DeliveryPendingCapacity,
	})
	require.NoError(t, err)
	require.True(t, created)
	require.NotEmpty(t, first.ID)

	beforeDuplicates := duplicates.Value()
	second, created, err := ledger.CreateOrGetDelivery(context.Background(), Delivery{
		SenderTaskID:    "source-task",
		SenderSessionID: "source-session",
		SourceTurnID:    "source-turn",
		IdempotencyKey:  "report-v1",
		TargetTaskID:    "target-task",
		TargetSessionID: "target-session",
		Content:         "must not create another delivery",
		State:           DeliveryPendingCapacity,
	})
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.ID, second.ID)
	assert.Equal(t, "review is ready", second.Content)
	assert.Equal(t, beforeDuplicates+1, duplicates.Value(), "duplicate suppression must be observable")
}

func TestDeliveryLedgerClaimDueRowsUsesExpiringLease(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ledger, ok := repo.(DeliveryLedger)
	require.True(t, ok, "queue repository must expose durable delivery storage")
	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)

	delivery, _, err := ledger.CreateOrGetDelivery(context.Background(), Delivery{
		SenderTaskID:    "source-task",
		SenderSessionID: "source-session",
		SourceTurnID:    "source-turn",
		IdempotencyKey:  "report-v1",
		TargetTaskID:    "target-task",
		TargetSessionID: "target-session",
		Content:         "review is ready",
		State:           DeliveryPendingCapacity,
		NextAttemptAt:   now,
	})
	require.NoError(t, err)

	claimed, err := ledger.ClaimDueDeliveries(context.Background(), now, "worker-a", time.Minute, 10)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	assert.Equal(t, delivery.ID, claimed[0].ID)
	assert.Equal(t, DeliveryReserved, claimed[0].State)
	assert.Equal(t, 1, claimed[0].Attempts)

	claimed, err = ledger.ClaimDueDeliveries(context.Background(), now.Add(30*time.Second), "worker-b", time.Minute, 10)
	require.NoError(t, err)
	assert.Empty(t, claimed)

	claimed, err = ledger.ClaimDueDeliveries(context.Background(), now.Add(time.Minute), "worker-b", time.Minute, 10)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	assert.Equal(t, "worker-b", claimed[0].LeaseOwner)
	assert.Equal(t, 2, claimed[0].Attempts)
}

func TestDeliveryLedgerRetryAndAcknowledgementRequireTheCurrentLease(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ledger, ok := repo.(DeliveryLedger)
	require.True(t, ok, "queue repository must expose durable delivery storage")
	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)

	delivery, _, err := ledger.CreateOrGetDelivery(context.Background(), Delivery{
		SenderTaskID:    "source-task",
		SenderSessionID: "source-session",
		SourceTurnID:    "source-turn",
		IdempotencyKey:  "report-v1",
		TargetTaskID:    "target-task",
		TargetSessionID: "target-session",
		Content:         "review is ready",
		State:           DeliveryPendingCapacity,
		NextAttemptAt:   now,
	})
	require.NoError(t, err)
	claimed, err := ledger.ClaimDueDeliveries(context.Background(), now, "worker-a", time.Minute, 1)
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	_, err = ledger.RescheduleDelivery(context.Background(), delivery.ID, "worker-b", now.Add(time.Minute), "target is full")
	require.ErrorIs(t, err, ErrDeliveryLeaseLost)
	retried, err := ledger.RescheduleDelivery(context.Background(), delivery.ID, "worker-a", now.Add(time.Minute), "target is full")
	require.NoError(t, err)
	assert.Equal(t, DeliveryRetryWait, retried.State)
	assert.Equal(t, "target is full", retried.LastError)

	claimed, err = ledger.ClaimDueDeliveries(context.Background(), now.Add(time.Minute), "worker-b", time.Minute, 1)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	queued, err := ledger.MarkDeliveryQueued(context.Background(), delivery.ID, "worker-b", "queue-entry")
	require.NoError(t, err)
	assert.Equal(t, DeliveryQueued, queued.State)
	assert.Equal(t, "queue-entry", queued.QueueEntryID)

	delivered, err := ledger.AcknowledgeDelivery(context.Background(), delivery.ID, "queue-entry", now.Add(2*time.Minute))
	require.NoError(t, err)
	assert.Equal(t, DeliveryDelivered, delivered.State)
	assert.False(t, delivered.DeliveredAt.IsZero())
}

func TestDeliveryLedgerDirectReservationRecoversAfterRestartLeaseExpiry(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ledger := repo.(DeliveryLedger)
	ctx := context.Background()
	delivery, _, err := ledger.CreateOrGetDelivery(ctx, Delivery{
		SenderTaskID: "source-task", SenderSessionID: "source-session", SourceTurnID: "source-turn",
		IdempotencyKey: "direct-v1", TargetTaskID: "target-task", TargetSessionID: "target-session",
		Content: "durable direct prompt", State: DeliveryPendingCapacity,
	})
	require.NoError(t, err)
	reserved, claimed, err := ledger.ReserveDeliveryForDirectDispatch(ctx, delivery.ID, "mcp-direct-a", time.Millisecond)
	require.NoError(t, err)
	require.True(t, claimed)
	assert.Equal(t, DeliveryReserved, reserved.State)
	duplicate, claimed, err := ledger.ReserveDeliveryForDirectDispatch(ctx, delivery.ID, "mcp-direct-b", time.Minute)
	require.NoError(t, err)
	assert.False(t, claimed, "a replay must not create another direct dispatch")
	assert.Equal(t, "mcp-direct-a", duplicate.LeaseOwner)

	claimedRows, err := ledger.ClaimDueDeliveries(ctx, reserved.LeaseExpiresAt.Add(time.Millisecond), "restarted-worker", time.Minute, 1)
	require.NoError(t, err)
	require.Len(t, claimedRows, 1, "a crash before direct acknowledgement is recoverable")
	queued, err := ledger.MarkDeliveryQueued(ctx, delivery.ID, "restarted-worker", "recovered-entry")
	require.NoError(t, err)
	assert.Equal(t, DeliveryQueued, queued.State)
}

func TestPurgeTaskCancelsUndeliveredDeliveryReceipts(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ledger := repo.(DeliveryLedger)
	delivery, _, err := ledger.CreateOrGetDelivery(context.Background(), Delivery{
		SenderTaskID:    "source-task",
		SenderSessionID: "source-session",
		SourceTurnID:    "source-turn",
		IdempotencyKey:  "report-v1",
		TargetTaskID:    "target-task",
		TargetSessionID: "target-session",
		Content:         "review is ready",
		State:           DeliveryPendingCapacity,
	})
	require.NoError(t, err)

	_, err = repo.PurgeTask(context.Background(), "target-task")
	require.NoError(t, err)
	stored, err := ledger.GetDelivery(context.Background(), delivery.ID)
	require.NoError(t, err)
	assert.Equal(t, DeliveryCancelled, stored.State)
}

func TestDeliveryLedgerRetryReopensOnlyRecoverableReceipt(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ledger := repo.(DeliveryLedger)
	ctx := context.Background()
	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	delivery, _, err := ledger.CreateOrGetDelivery(ctx, Delivery{
		SenderTaskID: "source-task", SenderSessionID: "source-session", SourceTurnID: "source-turn",
		IdempotencyKey: "report-v1", TargetTaskID: "target-task", TargetSessionID: "target-session",
		Content: "review is ready", State: DeliveryPendingCapacity, NextAttemptAt: now,
	})
	require.NoError(t, err)
	claimed, err := ledger.ClaimDueDeliveries(ctx, now, "worker-a", time.Minute, 1)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	_, err = ledger.MarkDeliveryRecoverable(ctx, delivery.ID, "worker-a", "target_queue_full")
	require.NoError(t, err)

	retried, err := ledger.RetryDelivery(ctx, delivery.ID, now.Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, DeliveryPendingCapacity, retried.State)
	assert.Empty(t, retried.LastError)

	_, err = ledger.RetryDelivery(ctx, delivery.ID, now.Add(2*time.Minute))
	assert.ErrorIs(t, err, ErrEntryNotFound, "a pending receipt cannot be retried twice as a recovery action")
}

// TestDeliveryLedgerPostgreSQLParity is test-only contract coverage for the
// already-shared repository implementation. It exercises the dialect-sensitive
// conflict, lease, acknowledgement, and cancellation paths on PostgreSQL.
func TestDeliveryLedgerPostgreSQLParity(t *testing.T) {
	repo := newTestPostgresRepo(t)
	ledger := repo.(DeliveryLedger)
	ctx := context.Background()
	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)

	first, created, err := ledger.CreateOrGetDelivery(ctx, Delivery{
		SenderTaskID: "source-task", SenderSessionID: "source-session", SourceTurnID: "source-turn",
		IdempotencyKey: "report-v1", TargetTaskID: "target-task", TargetSessionID: "target-session",
		Content: "review is ready", State: DeliveryPendingCapacity, NextAttemptAt: now,
	})
	require.NoError(t, err)
	require.True(t, created)
	duplicate, created, err := ledger.CreateOrGetDelivery(ctx, Delivery{
		SenderTaskID: "source-task", SenderSessionID: "source-session", SourceTurnID: "source-turn",
		IdempotencyKey: "report-v1", TargetTaskID: "target-task", TargetSessionID: "target-session",
		Content: "must be suppressed", State: DeliveryPendingCapacity, NextAttemptAt: now,
	})
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.ID, duplicate.ID)

	claimed, err := ledger.ClaimDueDeliveries(ctx, now, "worker-a", time.Minute, 1)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	assert.Equal(t, 1, claimed[0].Attempts)
	retried, err := ledger.RescheduleDelivery(ctx, first.ID, "worker-a", now.Add(time.Minute), "target_queue_full")
	require.NoError(t, err)
	assert.Equal(t, DeliveryRetryWait, retried.State)
	claimed, err = ledger.ClaimDueDeliveries(ctx, retried.NextAttemptAt, "worker-b", time.Minute, 1)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	queued, err := ledger.MarkDeliveryQueued(ctx, first.ID, "worker-b", "queue-entry")
	require.NoError(t, err)
	assert.Equal(t, DeliveryQueued, queued.State)
	delivered, err := ledger.AcknowledgeDelivery(ctx, first.ID, "queue-entry", now.Add(2*time.Minute))
	require.NoError(t, err)
	assert.Equal(t, DeliveryDelivered, delivered.State)
	direct, _, err := ledger.CreateOrGetDelivery(ctx, Delivery{
		SenderTaskID: "source-task", SenderSessionID: "source-session", SourceTurnID: "source-turn-direct",
		IdempotencyKey: "direct-v1", TargetTaskID: "target-task", TargetSessionID: "target-session",
		Content: "direct prompt", State: DeliveryPendingCapacity,
	})
	require.NoError(t, err)
	_, claimedDirect, err := ledger.ReserveDeliveryForDirectDispatch(ctx, direct.ID, "mcp-direct", time.Minute)
	require.NoError(t, err)
	require.True(t, claimedDirect)
	directDelivered, err := ledger.AcknowledgeDirectDelivery(ctx, direct.ID, "mcp-direct", now.Add(3*time.Minute))
	require.NoError(t, err)
	assert.Equal(t, DeliveryDelivered, directDelivered.State)

	pending, _, err := ledger.CreateOrGetDelivery(ctx, Delivery{
		SenderTaskID: "source-task", SenderSessionID: "source-session", SourceTurnID: "source-turn-2",
		IdempotencyKey: "report-v2", TargetTaskID: "target-task", TargetSessionID: "target-session",
		Content: "cancel me", State: DeliveryPendingCapacity, NextAttemptAt: now,
	})
	require.NoError(t, err)
	_, err = repo.PurgeTask(ctx, "target-task")
	require.NoError(t, err)
	cancelled, err := ledger.GetDelivery(ctx, pending.ID)
	require.NoError(t, err)
	assert.Equal(t, DeliveryCancelled, cancelled.State)
}
