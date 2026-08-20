package messagequeue

import (
	"context"
	"expvar"
	"sync"
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

func TestDeliveryLedgerAcceptedDirectReservationIsNeverReclaimed(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ledger := repo.(DeliveryLedger)
	ctx := context.Background()
	delivery, _, err := ledger.CreateOrGetDelivery(ctx, Delivery{
		SenderTaskID: "source-task", SenderSessionID: "source-session", SourceTurnID: "accepted-source-turn",
		IdempotencyKey: "accepted-v1", TargetTaskID: "target-task", TargetSessionID: "target-session",
		Content: "accepted direct prompt", State: DeliveryPendingCapacity,
	})
	require.NoError(t, err)
	_, claimed, err := ledger.ReserveDeliveryForDirectDispatch(ctx, delivery.ID, "mcp-direct", time.Millisecond)
	require.NoError(t, err)
	require.True(t, claimed)
	uncertain, err := ledger.MarkDirectDeliveryAcceptanceUncertain(ctx, delivery.ID, "mcp-direct")
	require.NoError(t, err)
	assert.Equal(t, DeliveryAmbiguous, uncertain.State)

	claimedRows, err := ledger.ClaimDueDeliveries(ctx, time.Now().UTC().Add(time.Hour), "restart-worker", time.Minute, 1)
	require.NoError(t, err)
	assert.Empty(t, claimedRows, "agentctl acceptance must survive a worker restart without replay")

	delivered, err := ledger.AcknowledgeDirectDelivery(ctx, delivery.ID, "mcp-direct", time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, DeliveryDelivered, delivered.State)
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
	uncertainDirect, err := ledger.MarkDirectDeliveryAcceptanceUncertain(ctx, direct.ID, "mcp-direct")
	require.NoError(t, err)
	assert.Equal(t, DeliveryAmbiguous, uncertainDirect.State)
	directDelivered, err := ledger.AcknowledgeDirectDelivery(ctx, direct.ID, "mcp-direct", now.Add(3*time.Minute))
	require.NoError(t, err)
	assert.Equal(t, DeliveryDelivered, directDelivered.State)

	// PostgreSQL must use the same transaction for FIFO removal and receipt
	// terminalization. This also covers the direct-interrupt metadata fallback:
	// the entry can reach the executor before queue_entry_id is attached.
	interrupt, _, err := ledger.CreateOrGetDelivery(ctx, Delivery{
		SenderTaskID: "source-task", SenderSessionID: "source-session", SourceTurnID: "source-turn-interrupt",
		IdempotencyKey: "interrupt-v1", TargetTaskID: "target-task", TargetSessionID: "target-session",
		Content: "take this now", State: DeliveryPendingCapacity,
	})
	require.NoError(t, err)
	_, claimedInterrupt, err := ledger.ReserveDeliveryForDirectDispatch(ctx, interrupt.ID, "mcp-interrupt", time.Minute)
	require.NoError(t, err)
	require.True(t, claimedInterrupt)
	entry := &QueuedMessage{
		SessionID: "target-session", TaskID: "target-task", Content: interrupt.Content, QueuedBy: QueuedByAgent,
		Metadata: map[string]interface{}{MetadataLifecycleDurable: true, MetadataDeliveryID: interrupt.ID},
	}
	require.NoError(t, repo.Insert(ctx, entry, 0))
	reserved, err := repo.ReserveHead(ctx, entry.SessionID)
	require.NoError(t, err)
	require.NotNil(t, reserved)
	acknowledger, ok := repo.(interface {
		AcknowledgeQueueEntryAndDelivery(context.Context, string, string, time.Time) error
	})
	require.True(t, ok)
	require.NoError(t, acknowledger.AcknowledgeQueueEntryAndDelivery(ctx, entry.SessionID, entry.ID, now.Add(4*time.Minute)))
	interruptDelivered, err := ledger.GetDelivery(ctx, interrupt.ID)
	require.NoError(t, err)
	assert.Equal(t, DeliveryDelivered, interruptDelivered.State)
	assert.Equal(t, entry.ID, interruptDelivered.QueueEntryID)
	entries, err := repo.ListBySession(ctx, entry.SessionID)
	require.NoError(t, err)
	assert.Empty(t, entries)

	// A queue-entry acknowledgement whose delete predicate fails must roll the
	// receipt update back in the same PostgreSQL transaction. Otherwise a crash
	// between the two writes could hide an undelivered FIFO entry behind a
	// delivered receipt.
	rollback, _, err := ledger.CreateOrGetDelivery(ctx, Delivery{
		SenderTaskID: "source-task", SenderSessionID: "source-session", SourceTurnID: "source-turn-rollback",
		IdempotencyKey: "rollback-v1", TargetTaskID: "target-task", TargetSessionID: "rollback-session",
		Content: "must remain queued", State: DeliveryPendingCapacity,
	})
	require.NoError(t, err)
	_, claimedRollback, err := ledger.ReserveDeliveryForDirectDispatch(ctx, rollback.ID, "rollback-owner", time.Minute)
	require.NoError(t, err)
	require.True(t, claimedRollback)
	rollbackEntry := &QueuedMessage{SessionID: "rollback-session", TaskID: "target-task", Content: rollback.Content, QueuedBy: QueuedByAgent, Metadata: map[string]interface{}{MetadataLifecycleDurable: true, MetadataDeliveryID: rollback.ID}}
	require.NoError(t, repo.Insert(ctx, rollbackEntry, 0))
	require.ErrorIs(t, acknowledger.AcknowledgeQueueEntryAndDelivery(ctx, "wrong-session", rollbackEntry.ID, now.Add(5*time.Minute)), ErrEntryNotFound)
	rollbackStored, err := ledger.GetDelivery(ctx, rollback.ID)
	require.NoError(t, err)
	assert.Equal(t, DeliveryReserved, rollbackStored.State)
	rollbackEntries, err := repo.ListBySession(ctx, rollbackEntry.SessionID)
	require.NoError(t, err)
	require.Len(t, rollbackEntries, 1)

	// A concurrent expired-lease claim and FIFO acknowledgement may race on a
	// restarted worker. PostgreSQL must leave exactly one durable outcome: a
	// delivered/deleted entry, or a reclaimed reserved entry that still exists.
	// The latter is safe to retry; neither outcome duplicates the prompt.
	repoA, repoB, _ := newTestPostgresRepoPair(t)
	ledgerA := repoA.(DeliveryLedger)
	race, _, err := ledgerA.CreateOrGetDelivery(ctx, Delivery{SenderTaskID: "source-task", SenderSessionID: "source-session", SourceTurnID: "source-turn-race", IdempotencyKey: "race-v1", TargetTaskID: "race-task", TargetSessionID: "race-session", Content: "race payload", State: DeliveryPendingCapacity})
	require.NoError(t, err)
	reservedRace, claimedRace, err := ledgerA.ReserveDeliveryForDirectDispatch(ctx, race.ID, "expired-owner", time.Nanosecond)
	require.NoError(t, err)
	require.True(t, claimedRace)
	raceEntry := &QueuedMessage{SessionID: "race-session", TaskID: "race-task", Content: race.Content, QueuedBy: QueuedByAgent, Metadata: map[string]interface{}{MetadataLifecycleDurable: true, MetadataDeliveryID: race.ID}}
	require.NoError(t, repoA.Insert(ctx, raceEntry, 0))
	var raceWG sync.WaitGroup
	raceWG.Add(2)
	go func() {
		defer raceWG.Done()
		_, _ = repoA.(DeliveryLedger).ClaimDueDeliveries(ctx, reservedRace.LeaseExpiresAt.Add(time.Second), "worker-race", time.Minute, 1)
	}()
	go func() {
		defer raceWG.Done()
		_ = repoB.(interface {
			AcknowledgeQueueEntryAndDelivery(context.Context, string, string, time.Time) error
		}).AcknowledgeQueueEntryAndDelivery(ctx, raceEntry.SessionID, raceEntry.ID, now.Add(6*time.Minute))
	}()
	raceWG.Wait()
	raceStored, err := ledgerA.GetDelivery(ctx, race.ID)
	require.NoError(t, err)
	raceEntries, err := repoA.ListBySession(ctx, raceEntry.SessionID)
	require.NoError(t, err)
	if raceStored.State == DeliveryDelivered {
		assert.Empty(t, raceEntries)
	} else {
		assert.Equal(t, DeliveryReserved, raceStored.State)
		require.Len(t, raceEntries, 1)
	}

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
