package process

import (
	"context"
	"encoding/json"
	"errors"
	"syscall"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	agenttypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

const (
	defaultDeliveryFailureDrainTimeout = 30 * time.Second
	deliveryFailedState                = "delivery_failed"
	deliveryHealthyState               = "healthy"
)

// recordDeliveryWriteFailure captures bounded failure evidence at the exact
// failed write boundary. It runs before cancellation or teardown so status and
// recovery can report a reason without output content.
func (m *Manager) recordDeliveryWriteFailure(err error, update adapter.AgentEvent, limit int64) {
	if m == nil || err == nil || !deliveryFailureEligible(update.Type) {
		return
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		// Shutdown or an expired request is transport uncertainty, not a
		// storage or size failure.
		return
	}
	eventBytes := int64(0)
	if encoded, marshalErr := json.Marshal(update); marshalErr == nil {
		eventBytes = int64(len(encoded))
	}
	sessionID, incarnationID, generation := m.DeliverySubmissionIdentity()
	revision := m.deliveryFailureRevision.Add(1)
	snapshot := &agenttypes.DeliveryFailureSnapshot{
		Reason:            classifyDeliveryFailure(err),
		ProcessGeneration: m.ProcessGeneration(),
		SessionID:         sessionID,
		IncarnationID:     incarnationID,
		HarnessGeneration: generation,
		StreamID:          m.DeliveryStreamID(),
		SubmissionID:      update.DeliverySubmissionID,
		PromptGeneration:  update.PromptGeneration,
		Revision:          revision,
		EventType:         update.Type,
		EventBytes:        eventBytes,
		EventLimit:        limit,
		ObservedAt:        time.Now().UTC(),
	}
	m.deliveryActiveMu.Lock()
	m.deliveryHealth.Failure = snapshot
	m.deliveryHealth.State = deliveryFailedState
	m.deliveryBlocked = true
	m.deliveryActiveMu.Unlock()
	m.logger.Error("durable delivery write failed; process liveness unchanged",
		zap.String("reason", snapshot.Reason),
		zap.String("event_type", snapshot.EventType),
		zap.Int64("event_bytes", snapshot.EventBytes),
		zap.Int64("event_limit", snapshot.EventLimit),
		zap.String("submission_id", snapshot.SubmissionID),
		zap.Error(err))
}

// deliveryFailureEligible keeps best-effort diagnostic carriers out of the
// transcript delivery-health contract. Their own publish path already reports
// a bounded debug or error line.
func deliveryFailureEligible(eventType string) bool {
	switch eventType {
	case "", streams.EventTypeMCPAttachment:
		return false
	default:
		return true
	}
}

func classifyDeliveryFailure(err error) string {
	switch {
	case err == nil:
		return agenttypes.DeliveryFailureWriterFailed
	case errors.Is(err, journal.ErrEventTooLarge):
		return agenttypes.DeliveryFailureEventTooLarge
	case errors.Is(err, journal.ErrJournalFull), errors.Is(err, syscall.ENOSPC), errors.Is(err, syscall.EDQUOT):
		return agenttypes.DeliveryFailureStorageFull
	case errors.Is(err, journal.ErrJournalCorrupt), errors.Is(err, journal.ErrJournalNewerVersion):
		return agenttypes.DeliveryFailureStorageCorrupt
	case errors.Is(err, journal.ErrJournalClosed):
		return agenttypes.DeliveryFailureStorageUnavailable
	default:
		return agenttypes.DeliveryFailureWriterFailed
	}
}

// clearDeliveryFailure marks the writer usable again only after a real commit.
func (m *Manager) clearDeliveryFailure() {
	m.deliveryActiveMu.Lock()
	if m.deliveryHealth.Failure != nil {
		m.deliveryHealth.Failure = nil
		m.deliveryBlocked = false
		if m.deliveryHealth.State == deliveryFailedState {
			m.deliveryHealth.State = deliveryHealthyState
		}
	}
	m.deliveryActiveMu.Unlock()
}

// drainAfterDeliveryFailure keeps the sole adapter consumer alive through a
// bounded cancellation drain. It drops uncommitted ordinary output and tries
// to commit only an actual terminal, which is the only event that may settle
// the retained submission. It returns true when the normal forwarding loop may
// resume with the same open writer; a closed adapter channel or shutdown ends
// the consumer instead.
func (m *Manager) drainAfterDeliveryFailure(updates <-chan adapter.AgentEvent, stopCh <-chan struct{}) bool {
	timeout := m.deliveryFailureDrain
	if timeout <= 0 {
		timeout = defaultDeliveryFailureDrainTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	lifetime := m.lifetimeCtx
	var lifetimeDone <-chan struct{}
	if lifetime != nil {
		lifetimeDone = lifetime.Done()
	}
	for {
		select {
		case update, ok := <-updates:
			if !ok {
				return false
			}
			update.DeliverySubmissionID = m.deliverySubmissionIDForEvent(update)
			if !isTerminalDeliveryEvent(update) {
				// Ordinary output after a failed write must never be published as
				// if delivery continued from the last accepted sequence.
				continue
			}
			m.commitDrainedTerminal(update)
			// The writer remains open. Resume forwarding so a later recovered
			// prompt on this retained runtime still reaches the journal.
			return true
		case <-stopCh:
			return false
		case <-lifetimeDone:
			return false
		case <-timer.C:
			// No terminal was observed yet. Keep the consumer and the sticky
			// failure; admission stays fenced until a real commit or recovery.
			return true
		}
	}
}

func (m *Manager) commitDrainedTerminal(update adapter.AgentEvent) {
	persisted, err := m.persistDeliveryEvent(update)
	if err != nil {
		m.logger.Error("failed to commit the actual terminal after delivery failure",
			zap.String("submission_id", update.DeliverySubmissionID), zap.Error(err))
		return
	}
	m.recordTerminalOutcome(&persisted)
	m.signalDeliveryWakeup()
	m.clearDeliveryFailure()
	m.logger.Info("committed the actual terminal after delivery failure",
		zap.String("submission_id", update.DeliverySubmissionID),
		zap.Uint64("sequence", persisted.DeliverySequence))
}
