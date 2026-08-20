package messagequeue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	defaultDeliveryClaimBatch   = 32
	defaultDeliveryLease        = time.Minute
	defaultDeliveryAttemptLimit = 5
)

// StartDeliveryRecovery owns the bounded capacity retry loop. It runs an
// immediate startup scan so deliveries accepted before a restart do not wait
// for the first tick, then derives all later work from a cancellable owner.
func (s *Service) StartDeliveryRecovery(ctx context.Context) {
	if _, ok := s.repo.(DeliveryLedger); !ok {
		return
	}
	s.deliveryWorkerMu.Lock()
	if s.deliveryWorkerStarted {
		s.deliveryWorkerMu.Unlock()
		return
	}
	if s.deliveryWorkerStopped {
		s.deliveryWorkerStopped = false
	}
	s.deliveryWorkerCtx, s.deliveryWorkerCancel = context.WithCancel(ctx)
	workerCtx := s.deliveryWorkerCtx
	interval := s.deliveryWorkerInterval
	if interval <= 0 {
		interval = time.Second
	}
	s.deliveryWorkerStarted = true
	s.deliveryWorkerWG.Add(1)
	s.deliveryWorkerMu.Unlock()

	s.processDueDeliveriesAtStartup(workerCtx)
	go s.runDeliveryRecovery(workerCtx, interval)
}

// StopDeliveryRecovery cancels and joins the only background delivery owner.
func (s *Service) StopDeliveryRecovery() {
	s.deliveryWorkerMu.Lock()
	s.deliveryWorkerStopped = true
	cancel := s.deliveryWorkerCancel
	s.deliveryWorkerMu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.deliveryWorkerWG.Wait()
	s.deliveryWorkerMu.Lock()
	s.deliveryWorkerStarted = false
	s.deliveryWorkerCancel = nil
	s.deliveryWorkerCtx = nil
	s.deliveryWorkerMu.Unlock()
}

func (s *Service) processDueDeliveriesAtStartup(ctx context.Context) {
	if _, err := s.ProcessDueDeliveries(ctx, time.Now().UTC(), "startup-"+uuid.NewString()); err != nil {
		s.logger.Warn("failed to process due delivery receipts at startup", zap.Error(err))
	}
}

func (s *Service) runDeliveryRecovery(ctx context.Context, interval time.Duration) {
	defer s.deliveryWorkerWG.Done()
	workerID := "delivery-" + uuid.NewString()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if _, err := s.ProcessDueDeliveries(ctx, now.UTC(), workerID); err != nil {
				s.logger.Warn("failed to process due delivery receipts", zap.Error(err))
			}
		}
	}
}

// ProcessDueDeliveries performs one bounded sender-independent capacity scan.
// It intentionally queues only into the target FIFO: prompt dispatch and
// acknowledgement retain the existing executor ownership boundary.
func (s *Service) ProcessDueDeliveries(ctx context.Context, now time.Time, workerID string) (int, error) {
	ledger, ok := s.repo.(DeliveryLedger)
	if !ok {
		return 0, nil
	}
	if workerID == "" {
		workerID = uuid.NewString()
	}
	deliveries, err := ledger.ClaimDueDeliveries(ctx, now, workerID, defaultDeliveryLease, defaultDeliveryClaimBatch)
	if err != nil {
		return 0, err
	}
	for _, delivery := range deliveries {
		s.processClaimedDelivery(ctx, ledger, delivery, workerID, now)
	}
	return len(deliveries), nil
}

func (s *Service) processClaimedDelivery(ctx context.Context, ledger DeliveryLedger, delivery Delivery, workerID string, now time.Time) {
	metadata := copyMessageMetadata(delivery.Metadata, 2)
	metadata[MetadataDeliveryID] = delivery.ID
	metadata[MetadataLifecycleDurable] = true
	queued, err := s.queueMessageWithMetadataSeparate(
		ctx, delivery.TargetSessionID, delivery.TargetTaskID, delivery.Content, "", QueuedByAgent,
		false, nil, metadata, s.MaxPerSession(),
	)
	if err != nil {
		s.deferClaimedDelivery(ctx, ledger, delivery, workerID, now, err)
		return
	}
	if _, err := ledger.MarkDeliveryQueued(ctx, delivery.ID, workerID, queued.ID); err != nil {
		s.logger.Error("delivery queue admission acknowledgement failed",
			zap.String("delivery_id", delivery.ID), zap.String("queue_entry_id", queued.ID), zap.Error(err))
	}
}

func (s *Service) deferClaimedDelivery(ctx context.Context, ledger DeliveryLedger, delivery Delivery, workerID string, now time.Time, cause error) {
	message := deliveryFailureCode(cause)
	if delivery.Attempts >= defaultDeliveryAttemptLimit {
		if _, err := ledger.MarkDeliveryRecoverable(ctx, delivery.ID, workerID, message); err != nil {
			s.logger.Error("failed to retain exhausted delivery", zap.String("delivery_id", delivery.ID), zap.Error(err))
		}
		return
	}
	nextAttemptAt := now.Add(deliveryRetryDelay(delivery.Attempts))
	if _, err := ledger.RescheduleDelivery(ctx, delivery.ID, workerID, nextAttemptAt, message); err != nil {
		s.logger.Error("failed to reschedule delivery", zap.String("delivery_id", delivery.ID), zap.Error(err))
	}
}

func deliveryFailureCode(err error) string {
	if errors.Is(err, ErrQueueFull) {
		return "target_queue_full"
	}
	return fmt.Sprintf("queue_admission_failed:%T", err)
}

func deliveryRetryDelay(attempt int) time.Duration {
	if attempt <= 1 {
		return time.Second
	}
	delay := time.Second << min(attempt-1, 5)
	return min(delay, time.Minute)
}
