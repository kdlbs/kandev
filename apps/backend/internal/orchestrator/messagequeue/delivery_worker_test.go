package messagequeue

import (
	"context"
	"errors"
	"expvar"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingDeliveryQueueAcknowledgementRepository struct {
	Repository
	DeliveryLedger
	failQueueAcknowledgement    bool
	failDispatchAcknowledgement bool
}

func (r *failingDeliveryQueueAcknowledgementRepository) MarkDeliveryQueued(
	ctx context.Context,
	deliveryID, leaseOwner, queueEntryID string,
) (*Delivery, error) {
	if r.failQueueAcknowledgement {
		return nil, errors.New("transient delivery queue acknowledgement failure")
	}
	return r.DeliveryLedger.MarkDeliveryQueued(ctx, deliveryID, leaseOwner, queueEntryID)
}

func (r *failingDeliveryQueueAcknowledgementRepository) AcknowledgeQueueEntryAndDelivery(
	ctx context.Context, sessionID, queueEntryID string, deliveredAt time.Time,
) error {
	if r.failDispatchAcknowledgement {
		return errors.New("injected atomic dispatch acknowledgement failure")
	}
	return r.DeliveryLedger.(interface {
		AcknowledgeQueueEntryAndDelivery(context.Context, string, string, time.Time) error
	}).AcknowledgeQueueEntryAndDelivery(ctx, sessionID, queueEntryID, deliveredAt)
}

func (r *failingDeliveryQueueAcknowledgementRepository) AcknowledgeDeliveryByQueueEntry(
	ctx context.Context, queueEntryID string, deliveredAt time.Time,
) (*Delivery, error) {
	if r.failDispatchAcknowledgement {
		return nil, errors.New("injected dispatch acknowledgement failure")
	}
	return r.DeliveryLedger.AcknowledgeDeliveryByQueueEntry(ctx, queueEntryID, deliveredAt)
}

func TestProcessDueDeliveriesRetainsFullQueueReceiptAndPromotesItOnce(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	service := NewService(repo, 1, logger.Default())
	ledger := repo.(DeliveryLedger)
	ctx := context.Background()
	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)

	_, err := service.QueueMessage(ctx, "target-session", "target-task", "existing", "", QueuedByAgent, false, nil)
	require.NoError(t, err)
	delivery, _, err := ledger.CreateOrGetDelivery(ctx, Delivery{
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

	processed, err := service.ProcessDueDeliveries(ctx, now, "worker-a")
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	stored, err := ledger.GetDelivery(ctx, delivery.ID)
	require.NoError(t, err)
	assert.Equal(t, DeliveryRetryWait, stored.State)
	assert.NotEmpty(t, stored.LastError)

	_, ok := service.TakeQueued(ctx, "target-session")
	require.True(t, ok)
	processed, err = service.ProcessDueDeliveries(ctx, stored.NextAttemptAt, "worker-a")
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	stored, err = ledger.GetDelivery(ctx, delivery.ID)
	require.NoError(t, err)
	assert.Equal(t, DeliveryQueued, stored.State)

	entries, err := repo.ListBySession(ctx, "target-session")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, stored.QueueEntryID, entries[0].ID)
	assert.Equal(t, delivery.ID, entries[0].Metadata[MetadataDeliveryID])
	assert.True(t, entries[0].IsDurableLifecycle())

	reserved, ok := service.ReserveQueued(ctx, "target-session")
	require.True(t, ok)
	require.Equal(t, stored.QueueEntryID, reserved.ID)
	require.NoError(t, service.AcknowledgeQueued(ctx, "target-session", reserved.ID))
	stored, err = ledger.GetDelivery(ctx, delivery.ID)
	require.NoError(t, err)
	assert.Equal(t, DeliveryDelivered, stored.State)
}

// TestProcessDueDeliveriesNotifiesOnQueuePromotion covers the gap where a
// cross-task message queued while the target session is busy landed in the
// database via queueMessageWithMetadataSeparate but never reached the
// frontend's queue store, because this worker path bypasses the WS/MCP
// handler layers that normally publish message.queue.status_changed. The
// orchestrator bridges this with SetQueuePromotionNotifier; this test proves
// the worker actually invokes it, and only after a real promotion.
func TestProcessDueDeliveriesNotifiesOnQueuePromotion(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	service := NewService(repo, 10, logger.Default())
	ledger := repo.(DeliveryLedger)
	ctx := context.Background()
	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)

	var notified []string
	service.SetQueuePromotionNotifier(func(_ context.Context, sessionID string) {
		notified = append(notified, sessionID)
	})

	delivery, _, err := ledger.CreateOrGetDelivery(ctx, Delivery{
		SenderTaskID:    "source-task",
		SenderSessionID: "source-session",
		SourceTurnID:    "source-turn",
		IdempotencyKey:  "notify-v1",
		TargetTaskID:    "target-task",
		TargetSessionID: "target-session",
		Content:         "agent queued 1",
		State:           DeliveryPendingCapacity,
		NextAttemptAt:   now,
	})
	require.NoError(t, err)

	processed, err := service.ProcessDueDeliveries(ctx, now, "worker-a")
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.Equal(t, []string{"target-session"}, notified)

	stored, err := ledger.GetDelivery(ctx, delivery.ID)
	require.NoError(t, err)
	assert.Equal(t, DeliveryQueued, stored.State)

	// A stale worker cannot reacquire this receipt with worker-b. The failed
	// acknowledgement must remove its FIFO row and suppress notification rather
	// than dispatching a cancelled or lease-lost delivery.
	service.processClaimedDelivery(ctx, ledger, *stored, "worker-b", now)
	assert.Equal(t, []string{"target-session"}, notified)
	entries, err := repo.ListBySession(ctx, "target-session")
	require.NoError(t, err)
	assert.Empty(t, entries, "a lease-lost receipt must not remain dispatchable")
}

func TestAcceptedQueuedDeliveryDoesNotReplayAfterRestart(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	service := NewService(repo, 1, logger.Default())
	ledger := repo.(DeliveryLedger)
	ctx := context.Background()
	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	delivery, _, err := ledger.CreateOrGetDelivery(ctx, Delivery{
		SenderTaskID: "source-task", SenderSessionID: "source-session", SourceTurnID: "source-turn",
		IdempotencyKey: "accepted-restart-v1", TargetTaskID: "target-task", TargetSessionID: "target-session",
		Content: "accepted exactly once", State: DeliveryPendingCapacity, NextAttemptAt: now,
	})
	require.NoError(t, err)
	processed, err := service.ProcessDueDeliveries(ctx, now, "worker-a")
	require.NoError(t, err)
	require.Equal(t, 1, processed)
	reserved, ok := service.ReserveQueued(ctx, "target-session")
	require.True(t, ok)
	// This is the synchronous accepted-prompt callback's transaction. A crash
	// after it commits but before the caller returns must not resurrect either
	// the FIFO row or its receipt on a restarted worker.
	require.NoError(t, service.AcknowledgeQueued(ctx, "target-session", reserved.ID))
	restarted := NewService(repo, 1, logger.Default())
	processed, err = restarted.ProcessDueDeliveries(ctx, now.Add(time.Hour), "worker-restarted")
	require.NoError(t, err)
	require.Equal(t, 0, processed)
	stored, err := ledger.GetDelivery(ctx, delivery.ID)
	require.NoError(t, err)
	require.Equal(t, DeliveryDelivered, stored.State)
	assert.Equal(t, 0, restarted.GetStatus(ctx, "target-session").Count)
}

func TestProcessDueDeliveriesReusesExistingQueueEntryAfterAcknowledgementFailure(t *testing.T) {
	baseRepo := newTestSQLiteRepo(t)
	ledger := baseRepo.(DeliveryLedger)
	repo := &failingDeliveryQueueAcknowledgementRepository{
		Repository:               baseRepo,
		DeliveryLedger:           ledger,
		failQueueAcknowledgement: true,
	}
	service := NewService(repo, 2, logger.Default())
	ctx := context.Background()
	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	delivery, _, err := ledger.CreateOrGetDelivery(ctx, Delivery{
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

	processed, err := service.ProcessDueDeliveries(ctx, now, "worker-a")
	require.NoError(t, err)
	require.Equal(t, 1, processed)
	stored, err := ledger.GetDelivery(ctx, delivery.ID)
	require.NoError(t, err)
	require.Equal(t, DeliveryReserved, stored.State)
	require.False(t, stored.LeaseExpiresAt.IsZero())

	entries, err := baseRepo.ListBySession(ctx, "target-session")
	require.NoError(t, err)
	assert.Empty(t, entries, "a lost delivery lease must not leave a FIFO entry")

	repo.failQueueAcknowledgement = false
	processed, err = service.ProcessDueDeliveries(ctx, stored.LeaseExpiresAt, "worker-b")
	require.NoError(t, err)
	require.Equal(t, 1, processed)
	stored, err = ledger.GetDelivery(ctx, delivery.ID)
	require.NoError(t, err)
	require.Equal(t, DeliveryQueued, stored.State)

	entries, err = baseRepo.ListBySession(ctx, "target-session")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, stored.QueueEntryID, entries[0].ID)
	assert.Equal(t, delivery.ID, entries[0].Metadata[MetadataDeliveryID])
}

func TestAcknowledgeQueuedDeliveryFailureRetainsAmbiguousReceiptWithoutReplay(t *testing.T) {
	baseRepo := newTestSQLiteRepo(t)
	ledger := baseRepo.(DeliveryLedger)
	repo := &failingDeliveryQueueAcknowledgementRepository{
		Repository: baseRepo, DeliveryLedger: ledger, failDispatchAcknowledgement: true,
	}
	service := NewService(repo, 2, logger.Default())
	ctx := context.Background()
	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	delivery, _, err := ledger.CreateOrGetDelivery(ctx, Delivery{
		SenderTaskID: "source-task", SenderSessionID: "source-session", SourceTurnID: "source-turn",
		IdempotencyKey: "report-v1", TargetTaskID: "target-task", TargetSessionID: "target-session",
		Content: "review is ready", State: DeliveryPendingCapacity, NextAttemptAt: now,
	})
	require.NoError(t, err)

	_, err = service.ProcessDueDeliveries(ctx, now, "worker-a")
	require.NoError(t, err)
	reserved, ok := service.ReserveQueued(ctx, "target-session")
	require.True(t, ok)
	require.Error(t, service.AcknowledgeQueued(ctx, "target-session", reserved.ID))

	// The delivery is marked ambiguous in the ledger so an operator can inspect
	// it, but the queue entry is removed to prevent a permanent FIFO blockage.
	stored, err := ledger.GetDelivery(ctx, delivery.ID)
	require.NoError(t, err)
	assert.Equal(t, DeliveryAmbiguous, stored.State)
	entries, err := baseRepo.ListBySession(ctx, "target-session")
	require.NoError(t, err)
	assert.Empty(t, entries, "ambiguous queue entry must be removed from the active FIFO")

	// An operator can inspect this receipt, but the ordinary recovery action
	// must not blindly send a prompt that the executor may have already taken.
	_, err = service.RetryDeliveryReceipt(ctx, delivery.ID)
	require.ErrorIs(t, err, ErrEntryNotFound)
	restarted := NewService(baseRepo, 2, logger.Default())
	_, replayed := restarted.ReserveQueued(ctx, "target-session")
	assert.False(t, replayed, "accepted-but-unfinalized delivery must not auto-replay")
}

func TestAcknowledgeQueuedDeliveryFinalizesDirectInterruptReceiptFromEntryMetadata(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	service := NewService(repo, 2, logger.Default())
	ledger := repo.(DeliveryLedger)
	ctx := context.Background()
	delivery, _, err := ledger.CreateOrGetDelivery(ctx, Delivery{
		SenderTaskID: "source-task", SenderSessionID: "source-session", SourceTurnID: "source-turn",
		IdempotencyKey: "interrupt-v1", TargetTaskID: "target-task", TargetSessionID: "target-session",
		Content: "stop and take this", State: DeliveryPendingCapacity,
	})
	require.NoError(t, err)
	_, claimed, err := ledger.ReserveDeliveryForDirectDispatch(ctx, delivery.ID, "mcp-interrupt", time.Minute)
	require.NoError(t, err)
	require.True(t, claimed)
	queued, err := service.QueueMessageWithMetadata(ctx, "target-session", "target-task", delivery.Content, "", QueuedByAgent, false, nil,
		map[string]interface{}{MetadataLifecycleDurable: true, MetadataDeliveryID: delivery.ID})
	require.NoError(t, err)
	reserved, ok := service.ReserveQueued(ctx, "target-session")
	require.True(t, ok)
	require.Equal(t, queued.ID, reserved.ID)

	// QueueAndInterrupt can accept this entry before its caller returns to
	// attach queue_entry_id. The callback must still terminalize this exact
	// reserved direct receipt from the trusted FIFO metadata.
	require.NoError(t, service.AcknowledgeQueued(ctx, "target-session", reserved.ID))
	stored, err := ledger.GetDelivery(ctx, delivery.ID)
	require.NoError(t, err)
	assert.Equal(t, DeliveryDelivered, stored.State)
	assert.Equal(t, queued.ID, stored.QueueEntryID)
	assert.Empty(t, service.GetStatus(ctx, "target-session").Entries)
}

func TestDeliveryRecoveryLifecycleProcessesStartupScanAndStops(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	service := NewService(repo, 1, logger.Default())
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
		NextAttemptAt:   time.Now().UTC(),
	})
	require.NoError(t, err)

	service.StartDeliveryRecovery(context.Background())
	t.Cleanup(service.StopDeliveryRecovery)
	stored, err := ledger.GetDelivery(context.Background(), delivery.ID)
	require.NoError(t, err)
	assert.Equal(t, DeliveryQueued, stored.State)

	service.StopDeliveryRecovery()
	service.StopDeliveryRecovery()
	reserved, ok := service.ReserveQueued(context.Background(), "target-session")
	require.True(t, ok)
	require.NoError(t, service.AcknowledgeQueued(context.Background(), "target-session", reserved.ID))
	second, _, err := ledger.CreateOrGetDelivery(context.Background(), Delivery{
		SenderTaskID:    "source-task",
		SenderSessionID: "source-session",
		SourceTurnID:    "next-source-turn",
		IdempotencyKey:  "report-v1",
		TargetTaskID:    "target-task",
		TargetSessionID: "target-session",
		Content:         "second report",
		State:           DeliveryPendingCapacity,
		NextAttemptAt:   time.Now().UTC(),
	})
	require.NoError(t, err)
	service.StartDeliveryRecovery(context.Background())
	t.Cleanup(service.StopDeliveryRecovery)
	stored, err = ledger.GetDelivery(context.Background(), second.ID)
	require.NoError(t, err)
	assert.Equal(t, DeliveryQueued, stored.State)
}

func TestProcessDueDeliveriesRetainsExhaustedCapacityFailureForRecovery(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	service := NewService(repo, 1, logger.Default())
	ledger := repo.(DeliveryLedger)
	retries := expvar.Get("administrative_turn_message_delivery_retries_total").(*expvar.Int)
	outcomes := expvar.Get("administrative_turn_message_delivery_outcomes_total").(*expvar.Map)
	beforeRetries := retries.Value()
	beforeRecoverable := expvarMapInt(t, outcomes, "outcome=recoverable")
	ctx := context.Background()
	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	_, err := service.QueueMessage(ctx, "target-session", "target-task", "existing", "", QueuedByAgent, false, nil)
	require.NoError(t, err)
	delivery, _, err := ledger.CreateOrGetDelivery(ctx, Delivery{
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

	for attempt := 0; attempt < defaultDeliveryAttemptLimit; attempt++ {
		processed, err := service.ProcessDueDeliveries(ctx, now, "worker-a")
		require.NoError(t, err)
		require.Equal(t, 1, processed)
		stored, err := ledger.GetDelivery(ctx, delivery.ID)
		require.NoError(t, err)
		if attempt == defaultDeliveryAttemptLimit-1 {
			assert.Equal(t, DeliveryRecoverable, stored.State)
			assert.Equal(t, "review is ready", stored.Content)
			assert.Equal(t, "target_queue_full", stored.LastError)
			assert.Equal(t, beforeRetries+defaultDeliveryAttemptLimit-1, retries.Value())
			assert.Equal(t, beforeRecoverable+1, expvarMapInt(t, outcomes, "outcome=recoverable"))
			return
		}
		now = stored.NextAttemptAt
	}
}

func expvarMapInt(t *testing.T, m *expvar.Map, key string) int64 {
	t.Helper()
	v := m.Get(key)
	if v == nil {
		return 0
	}
	counter, ok := v.(*expvar.Int)
	require.True(t, ok, "metric %q must be an integer counter", key)
	return counter.Value()
}
