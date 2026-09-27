package process

import (
	"context"
	"encoding/json"
	"expvar"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
)

// agentLinkBudgetJournalFailedTotal counts every time journalAndEnd exhausts
// its retries and keeps the budget pause as an in-memory unjournaled pause
// instead (system design part 2 "Budget enforcement" step 5).
var agentLinkBudgetJournalFailedTotal = expvar.NewInt("agent_link_budget_journal_failed_total")

// offlineBudgetEventType is the journal event type recorded by
// journalOfflineBudgetExhausted, per system design part 2 "Budget
// enforcement" step 5.
const offlineBudgetEventType = "agent_link.offline_budget_exhausted"

// offlineBudgetDuration resolves the configured offline budget, falling back
// to defaultOfflineBudget when unset (system design part 2 "Budget
// configuration": absent or zero means 15 minutes).
func (m *Manager) offlineBudgetDuration() time.Duration {
	if m.cfg == nil || m.cfg.OfflineBudgetMinutes <= 0 {
		return defaultOfflineBudget
	}
	return time.Duration(m.cfg.OfflineBudgetMinutes) * time.Minute
}

// ensureAttach lazily builds m.attach with real, wired hooks the first time
// it is needed, so a Manager built directly by a test or by
// blocking_send_sites_test.go still starts detached rather than panicking on
// a nil pointer.
func (m *Manager) ensureAttach() *attachmentState {
	m.attachMu.Lock()
	defer m.attachMu.Unlock()
	if m.attach == nil {
		m.attach = newAttachmentState(m.offlineBudgetDuration(), m.newAttachmentHooks())
	}
	return m.attach
}

// newAttachmentHooks wires attachmentHooks to this Manager's journal,
// adapter, and process group, mirroring the injectable-hook pattern already
// used for groupAliveFn/terminateGroupFn/killGroupFn.
func (m *Manager) newAttachmentHooks() attachmentHooks {
	return attachmentHooks{
		journalHighWater: m.deliveryJournalHighWater,
		hasActiveTurn:    func() bool { return m.activeTurnCount.Load() > 0 },
		cancelTurn: func(ctx context.Context) error {
			adapter := m.GetAdapter()
			if adapter == nil {
				return nil
			}
			return adapter.Cancel(ctx)
		},
		stopAgent:                m.Stop,
		journalBudgetExhausted:   m.journalOfflineBudgetExhausted,
		onBudgetJournalFailed:    func() { agentLinkBudgetJournalFailedTotal.Add(1) },
		cancelPendingPermissions: m.CancelPendingPermissions,
	}
}

// deliveryJournalHighWater returns the current durable stream's high water,
// or zero for a non-durable instance or a stream with no journaled events
// yet. It must never block on attachMu (attachmentHooks.journalHighWater's
// contract).
func (m *Manager) deliveryJournalHighWater() uint64 {
	deliveryJournal, err := m.DeliveryJournal()
	if err != nil {
		return 0
	}
	stream, err := deliveryJournal.GetStream(context.Background(), m.DeliveryStreamID())
	if err != nil {
		return 0
	}
	return stream.HighWater
}

// journalOfflineBudgetExhausted journals agent_link.offline_budget_exhausted
// directly via the delivery journal's AppendBatch, bypassing
// updatesCh/persistDeliveryEvent: nothing is attached to drain updatesCh
// while detached, and a future reconnecting stream picks this up through the
// existing cursor-based journal replay regardless of live-channel delivery.
// A non-durable instance (mirroring persistDeliveryEvent's own check) returns
// nil without writing anything.
func (m *Manager) journalOfflineBudgetExhausted(ctx context.Context, pause BudgetPause) error {
	if m.cfg == nil || m.cfg.DurableJournalPath == "" {
		return nil
	}
	deliveryJournal, err := m.DeliveryJournal()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(struct {
		DetachedSince string `json:"detached_since"`
		ExhaustedAt   string `json:"exhausted_at"`
		Outcome       string `json:"outcome"`
		CancelError   string `json:"cancel_error,omitempty"`
	}{
		DetachedSince: pause.DetachedSince.Format(time.RFC3339Nano),
		ExhaustedAt:   pause.ExhaustedAt.Format(time.RFC3339Nano),
		Outcome:       pause.Outcome,
		CancelError:   errString(pause.CancelError),
	})
	if err != nil {
		return err
	}
	streamID := m.DeliveryStreamID()
	sessionID := ""
	if m.cfg != nil {
		sessionID = m.cfg.SessionID
	}
	if sessionID == "" {
		sessionID = streamID
	}
	_, err = deliveryJournal.AppendBatch(ctx, []journal.Event{{
		SessionID:         sessionID,
		IncarnationID:     m.DeliveryIncarnationID(),
		HarnessGeneration: m.DeliveryHarnessGeneration(),
		StreamID:          streamID,
		Type:              offlineBudgetEventType,
		Payload:           payload,
	}})
	return err
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// IsAttached reports whether a stream is current, confirmed or not. It is
// the one production call site sendPermissionNotification relies on.
func (m *Manager) IsAttached() bool {
	return m.ensureAttach().IsAttached()
}

// CloseAgentStream closes the current backend stream, if any, with the given
// code and reason (system design part 2 "Capability and close reason").
func (m *Manager) CloseAgentStream(code int, reason string) {
	m.ensureAttach().CloseCurrent(code, reason)
}

// AttachmentSnapshot returns the current attachment snapshot for task 02's
// Kandev call waiters.
func (m *Manager) AttachmentSnapshot() AttachmentSnapshot {
	return m.ensureAttach().Snapshot()
}

// AttachmentStatus returns the attachment state for the delivery-status
// endpoint.
func (m *Manager) AttachmentStatus() Attachment {
	return m.ensureAttach().Status()
}

// AttachmentEnforcing reports whether budget enforcement is currently
// running, for the unowned reaper gate.
func (m *Manager) AttachmentEnforcing() bool {
	return m.ensureAttach().Enforcing()
}

// AttachmentReaperGate reports whether this instance currently holds the
// unowned reaper's shutdown decision open, and the time its most recent
// budget enforcement ended, for the reaper gate (system design part 2
// "Unowned reaper").
func (m *Manager) AttachmentReaperGate() (hold bool, enforcementEndedAt time.Time) {
	return m.ensureAttach().ReaperGate()
}

// StreamStart begins a new agent stream, superseding any prior current
// stream (see attachmentState.StreamStart).
func (m *Manager) StreamStart(ctx context.Context, streamID string) (*supersededStream, error) {
	return m.ensureAttach().StreamStart(ctx, streamID)
}

// FinalizeStreamStart installs streamID as current, if it is still the
// stream meant to become current (see attachmentState.FinalizeStreamStart).
func (m *Manager) FinalizeStreamStart(
	streamID, attachID string,
	closeFn func(code int, reason string),
	done <-chan struct{},
) (attachedAtSequence uint64, confirmed bool, stillCurrent bool) {
	return m.ensureAttach().FinalizeStreamStart(streamID, attachID, closeFn, done)
}

// StreamEnd applies the Stream end transition for streamID.
func (m *Manager) StreamEnd(streamID string) {
	m.ensureAttach().StreamEnd(streamID)
}

// ConfirmStream implements the Confirm transition for the given attach_id.
func (m *Manager) ConfirmStream(attachID string) ConfirmResult {
	return m.ensureAttach().Confirm(attachID)
}

// BeginTurn and EndTurn track whether the adapter has a turn in flight, for
// budget enforcement's "no active turn" check. Callers increment BeginTurn
// before dispatching a prompt, durable or not, and decrement via EndTurn once
// it settles.
func (m *Manager) BeginTurn() {
	m.activeTurnCount.Add(1)
}

// EndTurn is the counterpart to BeginTurn.
func (m *Manager) EndTurn() {
	m.activeTurnCount.Add(-1)
}
