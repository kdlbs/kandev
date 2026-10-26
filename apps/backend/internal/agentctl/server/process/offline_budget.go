package process

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"go.uber.org/zap"
)

// DefaultOfflineBudget is the longest an instance may go without a stable
// backend attachment before its running turn is cancelled.
// stopReasonCancelled is the stop reason of a cancelled turn.
const stopReasonCancelled = "cancelled"

const DefaultOfflineBudget = 15 * time.Minute

// stableAttachmentWindow is how long an attachment must hold before it
// restarts the offline budget clock.
var stableAttachmentWindow = 30 * time.Second

// budgetCancelTimeout bounds the agent cancel issued when the budget expires.
var budgetCancelTimeout = 5 * time.Second

type budgetCancelState string

const (
	budgetCancelPending budgetCancelState = "pending"
	budgetCancelIssued  budgetCancelState = "issued"
	budgetCancelFailed  budgetCancelState = "failed"
	budgetCancelSkipped budgetCancelState = "skipped"
)

// budgetCancelRecord is the expiry decision for the turn that was running when
// the budget ran out. It is deleted only by that turn's terminal stamp.
type budgetCancelRecord struct {
	submissionID string
	state        budgetCancelState
	decidedAt    time.Time
}

// budgetClock is the offline budget state of one instance, guarded by the
// attachTracker mutex. The zero value has no budget.
type budgetClock struct {
	enabled        bool
	budget         time.Duration
	detachedSince  time.Time
	deadline       time.Time
	exhausted      bool
	deadlinePassed bool
	stable         bool
	gen            uint64
	budgetTimer    *time.Timer
	stableTimer    *time.Timer
	inFlight       string
	record         *budgetCancelRecord
}

// StartOfflineBudget starts the budget clock for a new instance, which has
// never had a stream.
func (m *Manager) StartOfflineBudget(budget time.Duration) {
	t := &m.attach
	t.mu.Lock()
	defer t.mu.Unlock()
	b := &t.budget
	b.enabled = true
	b.budget = budget
	m.restartClockLocked(time.Now())
}

func (m *Manager) restartClockLocked(now time.Time) {
	b := &m.attach.budget
	b.detachedSince = now
	b.deadline = now.Add(b.budget)
	if b.budgetTimer != nil {
		b.budgetTimer.Stop()
	}
	b.budgetTimer = time.AfterFunc(b.budget, m.onBudgetTimer)
}

func (m *Manager) armStabilityLocked() {
	b := &m.attach.budget
	b.gen++
	if !b.enabled {
		return
	}
	gen := b.gen
	if b.stableTimer != nil {
		b.stableTimer.Stop()
	}
	b.stableTimer = time.AfterFunc(stableAttachmentWindow, func() { m.onStable(gen) })
}

func (m *Manager) onStable(gen uint64) {
	t := &m.attach
	t.mu.Lock()
	defer t.mu.Unlock()
	b := &t.budget
	if b.gen != gen || len(t.streams) == 0 {
		return
	}
	b.stable = true
	b.deadlinePassed = false
	m.restartClockLocked(time.Now())
	if b.exhausted {
		b.exhausted = false
		t.changeLocked()
	}
}

// lastStreamEndedLocked runs on the 1 to 0 transition. It returns the expiry
// decision to run after the tracker mutex is released, or nil.
func (m *Manager) lastStreamEndedLocked() func() {
	b := &m.attach.budget
	b.gen++
	if !b.enabled {
		return nil
	}
	if b.stableTimer != nil {
		b.stableTimer.Stop()
	}
	if b.stable {
		b.stable = false
		b.deadlinePassed = false
		m.restartClockLocked(time.Now())
		return nil
	}
	if b.deadlinePassed {
		gen := b.gen
		return func() { m.decideBudget(gen) }
	}
	return nil
}

func (m *Manager) onBudgetTimer() {
	t := &m.attach
	t.mu.Lock()
	b := &t.budget
	if !b.enabled || b.exhausted || time.Now().Before(b.deadline) {
		t.mu.Unlock()
		return
	}
	if len(t.streams) > 0 {
		b.deadlinePassed = true
		t.mu.Unlock()
		return
	}
	gen := b.gen
	t.mu.Unlock()
	m.decideBudget(gen)
}

// decideBudget takes the expiry decision. It re-checks every precondition
// under a fresh hold of the tracker mutex, so a stream that attached first
// wins, and holds no lock while it cancels.
func (m *Manager) decideBudget(gen uint64) {
	t := &m.attach
	t.mu.Lock()
	b := &t.budget
	now := time.Now()
	if !b.enabled || len(t.streams) > 0 || b.exhausted || now.Before(b.deadline) || b.gen != gen {
		t.mu.Unlock()
		return
	}
	b.exhausted = true
	var submission string
	if b.inFlight != "" {
		submission = b.inFlight
		b.record = &budgetCancelRecord{submissionID: submission, state: budgetCancelPending, decidedAt: now}
	}
	t.changeLocked()
	t.mu.Unlock()
	m.logBudget("budget_exhausted", zap.Bool("turn_running", submission != ""))
	if submission != "" {
		m.cancelForBudget(submission)
	}
}

// cancelForBudget cancels the recorded turn through the agent cancel a user
// cancel uses. It never runs under the tracker mutex or a journal lock.
func (m *Manager) cancelForBudget(submission string) {
	t := &m.attach
	t.mu.Lock()
	b := &t.budget
	if b.inFlight != submission {
		m.setBudgetStateLocked(submission, budgetCancelSkipped)
		t.mu.Unlock()
		return
	}
	t.mu.Unlock()

	agent := m.GetAdapter()
	if agent == nil {
		m.finishBudgetCancel(submission, context.Canceled)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), budgetCancelTimeout)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- agent.Cancel(ctx) }()
	select {
	case err := <-result:
		m.finishBudgetCancel(submission, err)
	case <-ctx.Done():
		m.finishBudgetCancel(submission, ctx.Err())
	}
}

func (m *Manager) finishBudgetCancel(submission string, err error) {
	t := &m.attach
	state := budgetCancelIssued
	if err != nil {
		state = budgetCancelFailed
	}
	t.mu.Lock()
	applied := m.setBudgetStateLocked(submission, state)
	t.mu.Unlock()
	switch {
	case err != nil:
		m.logBudget("budget_cancel_failed", zap.Error(err), zap.Bool("record_present", applied))
	case applied:
		m.CancelPendingPermissions()
	}
}

// setBudgetStateLocked moves the decision record of submission to state. It
// never creates a record: a missing record means the turn's terminal already
// consumed it.
func (m *Manager) setBudgetStateLocked(submission string, state budgetCancelState) bool {
	rec := m.attach.budget.record
	if rec == nil || rec.submissionID != submission {
		return false
	}
	rec.state = state
	return true
}

func (m *Manager) logBudget(reason string, fields ...zap.Field) {
	if m.logger == nil {
		return
	}
	m.logger.Warn("offline budget", append(fields, zap.String("reason", reason))...)
}

// setBudgetInFlight marks submission as the turn whose prompt is being
// dispatched. Called from DispatchDeliverySubmission, which holds
// deliverySubmissionMu, so the lock order is deliverySubmissionMu then T.
func (m *Manager) setBudgetInFlight(submission string) {
	t := &m.attach
	t.mu.Lock()
	t.budget.inFlight = submission
	t.mu.Unlock()
}

func (m *Manager) clearBudgetInFlight(submission string) {
	t := &m.attach
	t.mu.Lock()
	if t.budget.inFlight == submission {
		t.budget.inFlight = ""
	}
	t.mu.Unlock()
}

// terminalStamp is what the journal commit of a terminal event records about
// the instance at that moment.
type terminalStamp struct {
	detached      bool
	committedAt   time.Time
	budgetCancel  bool
	cancelDecided time.Time
}

const cancelCauseOfflineBudget = "offline_budget"

// applyTerminalStamp stamps a terminal event with the detached fields and, for
// a completion whose turn the offline budget cancelled, with stop reason
// cancelled and the budget cause. An error terminal keeps its error.
func (m *Manager) applyTerminalStamp(update adapter.AgentEvent) adapter.AgentEvent {
	stamp := m.stampTerminal(update.DeliverySubmissionID)
	update.DetachedAtCommit = stamp.detached
	committed := stamp.committedAt
	update.CommittedAt = &committed
	if stamp.budgetCancel && update.Type == adapter.EventTypeComplete {
		data := make(map[string]any, len(update.Data)+1)
		for k, v := range update.Data {
			data[k] = v
		}
		data["stop_reason"] = stopReasonCancelled
		update.Data = data
		update.CancelCause = cancelCauseOfflineBudget
		decided := stamp.cancelDecided
		update.CancelDecidedAt = &decided
	}
	return update
}

// stampTerminal runs under the tracker mutex once per terminal event, before
// the event is encoded. It consumes the decision record of submission and
// clears the in-flight marker; it never waits on the cancel.
func (m *Manager) stampTerminal(submission string) terminalStamp {
	t := &m.attach
	t.mu.Lock()
	defer t.mu.Unlock()
	b := &t.budget
	stamp := terminalStamp{detached: len(t.streams) == 0, committedAt: time.Now()}
	if submission == "" {
		return stamp
	}
	if b.inFlight == submission {
		b.inFlight = ""
	}
	if rec := b.record; rec != nil && rec.submissionID == submission {
		b.record = nil
		if rec.state == budgetCancelPending || rec.state == budgetCancelIssued {
			stamp.budgetCancel = true
			stamp.cancelDecided = rec.decidedAt
		}
	}
	return stamp
}
