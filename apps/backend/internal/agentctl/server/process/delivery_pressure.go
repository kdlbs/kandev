package process

import (
	"context"
	"errors"
	"syscall"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	agenttypes "github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

var errDeliveryPressure = errors.New("durable delivery requires recovery before prompt admission")

// DeliveryHealth is independent of the native process status. A live process
// with blocked output must remain reachable for status, replay, ACK, and Stop.
type DeliveryHealth struct {
	State               string                              `json:"state"`
	ProducerPaused      bool                                `json:"producer_paused"`
	Capacity            journal.Capacity                    `json:"capacity"`
	CancellationPending bool                                `json:"cancellation_pending"`
	Failure             *agenttypes.DeliveryFailureSnapshot `json:"failure,omitempty"`
}

const deliveryStorageError = "storage_error"

func (m *Manager) DeliveryHealth() DeliveryHealth {
	if j, err := m.DeliveryJournal(); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = m.refreshDeliveryPressure(ctx, j, "")
		cancel()
	}
	m.deliveryActiveMu.RLock()
	defer m.deliveryActiveMu.RUnlock()
	return m.deliveryHealth
}

func (m *Manager) refreshDeliveryPressure(ctx context.Context, j *journal.Journal, submissionID string) error {
	capacity, err := j.Capacity(ctx, m.DeliveryStreamID())
	if err != nil {
		return err
	}
	m.deliveryActiveMu.Lock()
	previous := m.deliveryHealth.State
	m.applyDeliveryCapacityLocked(capacity)
	// A late event cannot cancel a successor that already owns the harness.
	cancelTurn := capacity.Guarded && m.deliveryCanCancelLocked(submissionID)
	if cancelTurn {
		m.beginDeliveryCancellationLocked(submissionID)
	}
	state := m.deliveryHealth.State
	m.deliveryActiveMu.Unlock()
	if state != previous {
		m.logger.Warn("durable delivery pressure changed", zap.String("state", state), zap.Int64("retained_bytes", capacity.StreamBytes), zap.Int64("limit_bytes", capacity.StreamLimit))
	}
	if cancelTurn {
		m.cancelDeliveryPressureTurn(submissionID)
	}
	return nil
}

func (m *Manager) cancelDeliveryPressureTurn(submissionID string) {
	ctx, release, err := m.BeginOwnedOperation(context.Background())
	if err != nil {
		m.finishDeliveryPressureCancellation(submissionID, err)
		return
	}
	generation := m.ProcessGeneration()
	go func() {
		defer release()
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		err := errDeliveryPressure
		agentAdapter, current := m.GetAdapterForGeneration(generation)
		if !current {
			m.finishDeliveryPressureCancellation(submissionID, errDeliveryPressure)
			return
		}
		if ctx.Err() != nil {
			m.finishDeliveryPressureCancellation(submissionID, ctx.Err())
			return
		}
		if agentAdapter != nil {
			err = agentAdapter.Cancel(ctx)
		}
		m.finishDeliveryPressureCancellation(submissionID, err)
	}()
}

func (m *Manager) finishDeliveryPressureCancellation(submissionID string, err error) {
	m.deliveryActiveMu.Lock()
	defer m.deliveryActiveMu.Unlock()
	if m.deliveryPressureCancelledID != submissionID {
		return
	}
	m.deliveryHealth.CancellationPending = false
	if m.deliveryPressureTerminalID == submissionID {
		err = nil
	}
	m.deliveryCancellationFailed = err != nil
	m.deliveryHealth.State = "interrupted"
	if err != nil {
		m.deliveryHealth.State = "cancellation_failed"
	}
	// Cancellation does not establish the prompt outcome. Only the retained
	// terminal and the normal reconciliation path may settle the submission.
}

func (m *Manager) recordDeliveryStorageFailure(err error) {
	m.deliveryActiveMu.Lock()
	m.deliveryBlocked = true
	m.deliveryHealth.State = deliveryStorageError
	m.deliveryActiveMu.Unlock()
	m.logger.Error("durable delivery storage failed; process liveness unchanged", zap.Error(err))
}

func (m *Manager) appendDeliveryWithRecovery(ctx context.Context, j *journal.Journal, events []journal.Event) ([]journal.Event, error) {
	paused := false
	defer func() {
		if paused {
			m.setDeliveryProducerPaused(false)
		}
	}()
	for {
		// AppendBatch assigns sequence numbers before a transaction can roll back.
		// Retries must use the original records, not the mutated failed attempt.
		attempt := append([]journal.Event(nil), events...)
		committed, err := j.AppendBatch(ctx, attempt)
		if err == nil {
			m.deliveryEventsCommitted(committed)
			if pressureErr := m.refreshDeliveryPressure(ctx, j, deliveryBatchSubmission(events)); pressureErr != nil {
				m.recordDeliveryStorageFailure(pressureErr)
			}
			return committed, nil
		}
		if errors.Is(err, journal.ErrEventTooLarge) || errors.Is(err, journal.ErrReplayPageTooLarge) {
			// A permanent size refusal is not storage pressure. It must never
			// enter the capacity wait or be reported as an exhausted journal.
			return nil, err
		}
		if !paused && (errors.Is(err, journal.ErrStreamFull) || errors.Is(err, journal.ErrJournalFull) || errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EDQUOT)) {
			paused = true
			m.setDeliveryProducerPaused(true)
		}
		if err := m.waitForDeliveryCapacity(ctx, j, events, err); err != nil {
			return nil, err
		}
	}
}

func (m *Manager) deliveryEventsCommitted(events []journal.Event) {
	m.deliveryActiveMu.Lock()
	defer m.deliveryActiveMu.Unlock()
	// A real commit re-establishes a usable writer, but ordinary output from the
	// failed submission cannot clear the sticky failure. Only its terminal or a
	// commit from a different submission may release admission.
	for _, event := range events {
		clear := event.Terminal
		if m.deliveryFailedSubmissionID != "" && event.SubmissionID != "" &&
			event.SubmissionID != m.deliveryFailedSubmissionID {
			clear = true
		}
		if clear {
			m.clearDeliveryFailureLocked()
		}
	}
	for _, event := range events {
		if !event.Terminal || event.SubmissionID == "" || event.SubmissionID != m.deliveryPressureCancelledID {
			continue
		}
		m.deliveryPressureTerminalID = event.SubmissionID
		m.deliveryCancellationFailed = false
		if !m.deliveryHealth.CancellationPending {
			m.deliveryHealth.State = "interrupted"
		}
	}
	if m.deliveryHealth.State == deliveryStorageError {
		m.deliveryHealth.State = "guarded"
	}
}

func (m *Manager) waitForDeliveryCapacity(ctx context.Context, j *journal.Journal, events []journal.Event, writeErr error) error {
	submissionID := deliveryBatchSubmission(events)
	switch {
	case errors.Is(writeErr, journal.ErrEventTooLarge), errors.Is(writeErr, journal.ErrReplayPageTooLarge):
		return writeErr
	case errors.Is(writeErr, syscall.ENOSPC), errors.Is(writeErr, syscall.EDQUOT):
		m.recordDeliveryStorageFailure(writeErr)
		m.cancelDeliveryAfterStorageFailure(submissionID)
	case errors.Is(writeErr, journal.ErrStreamFull), errors.Is(writeErr, journal.ErrJournalFull):
	default:
		return writeErr
	}
	for _, event := range events {
		if int64(len(event.Payload)) > j.MaxEventBytes() {
			return writeErr
		}
	}
	if err := m.refreshDeliveryPressure(ctx, j, submissionID); err != nil {
		return err
	}
	m.deliveryActiveMu.RLock()
	empty := m.deliveryHealth.Capacity.StreamBytes == 0
	m.deliveryActiveMu.RUnlock()
	if errors.Is(writeErr, journal.ErrStreamFull) && empty {
		return writeErr
	}
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func deliveryBatchSubmission(events []journal.Event) string {
	if len(events) == 0 {
		return ""
	}
	return events[0].SubmissionID
}

func (m *Manager) cancelDeliveryAfterStorageFailure(submissionID string) {
	m.deliveryActiveMu.Lock()
	cancelTurn := m.deliveryCanCancelLocked(submissionID)
	if cancelTurn {
		m.beginDeliveryCancellationLocked(submissionID)
	}
	m.deliveryActiveMu.Unlock()
	if cancelTurn {
		m.cancelDeliveryPressureTurn(submissionID)
	}
}

func (m *Manager) deliveryCanCancelLocked(submissionID string) bool {
	return submissionID != "" && m.deliveryActiveID == submissionID && !m.deliveryHealth.CancellationPending && m.deliveryPressureCancelledID != submissionID
}

func (m *Manager) beginDeliveryCancellationLocked(submissionID string) {
	m.deliveryBlocked = true
	m.deliveryPressureCancelledID = submissionID
	m.deliveryHealth.CancellationPending = true
	m.deliveryHealth.State = "cancelling"
}

func (m *Manager) applyDeliveryCapacityLocked(capacity journal.Capacity) {
	m.deliveryHealth.Capacity = capacity
	if m.deliveryHealth.Failure != nil {
		// Only a successful commit can prove the writer usable again. Capacity
		// recovery must not erase the captured failure.
		m.deliveryBlocked = true
		return
	}
	if m.deliveryCancellationFailed {
		return
	}
	switch m.deliveryHealth.State {
	case deliveryStorageError, "cancellation_failed":
		return
	}
	if capacity.Recovered && !m.deliveryHealth.CancellationPending {
		m.deliveryBlocked = false
		m.deliveryHealth.State = deliveryHealthyState
		return
	}
	if capacity.Guarded {
		m.deliveryBlocked = true
		switch m.deliveryHealth.State {
		case "", deliveryHealthyState, "pressure":
			m.deliveryHealth.State = "guarded"
		}
		return
	}
	if capacity.Pressure && !m.deliveryBlocked {
		m.deliveryHealth.State = "pressure"
	}
}

func (m *Manager) setDeliveryProducerPaused(paused bool) {
	m.deliveryActiveMu.Lock()
	defer m.deliveryActiveMu.Unlock()
	if paused {
		m.deliveryPausedWriters++
	} else {
		m.deliveryPausedWriters--
	}
	m.deliveryHealth.ProducerPaused = m.deliveryPausedWriters > 0
}
