package messagequeue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingDeliveryQueueAcknowledgementRepository struct {
	Repository
	DeliveryLedger
	failQueueAcknowledgement bool
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

	repo.failQueueAcknowledgement = false
	processed, err = service.ProcessDueDeliveries(ctx, stored.LeaseExpiresAt, "worker-b")
	require.NoError(t, err)
	require.Equal(t, 1, processed)
	stored, err = ledger.GetDelivery(ctx, delivery.ID)
	require.NoError(t, err)
	require.Equal(t, DeliveryQueued, stored.State)

	entries, err := baseRepo.ListBySession(ctx, "target-session")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, stored.QueueEntryID, entries[0].ID)
	assert.Equal(t, delivery.ID, entries[0].Metadata[MetadataDeliveryID])
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
			return
		}
		now = stored.NextAttemptAt
	}
}
