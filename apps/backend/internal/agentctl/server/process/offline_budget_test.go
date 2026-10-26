package process

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func withShortStableWindow(t *testing.T, d time.Duration) {
	t.Helper()
	prev := stableAttachmentWindow
	stableAttachmentWindow = d
	t.Cleanup(func() { stableAttachmentWindow = prev })
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func exhausted(m *Manager) bool { return m.AttachmentSnapshot().Exhausted }

func TestOfflineBudgetNeverAttachedInstanceExhausts(t *testing.T) {
	m := &Manager{}
	m.StartOfflineBudget(40 * time.Millisecond)
	eventually(t, "exhaustion", func() bool { return exhausted(m) })
}

func TestOfflineBudgetUnstableAttachmentDoesNotMoveDeadline(t *testing.T) {
	withShortStableWindow(t, time.Hour)
	m := &Manager{}
	m.StartOfflineBudget(150 * time.Millisecond)
	m.AttachStream("s1")
	time.Sleep(30 * time.Millisecond)
	m.DetachStream("s1")
	start := time.Now()
	eventually(t, "exhaustion at the original deadline", func() bool { return exhausted(m) })
	if time.Since(start) > 140*time.Millisecond {
		t.Fatalf("deadline moved: exhausted after %v", time.Since(start))
	}
}

func TestOfflineBudgetStableAttachmentRestartsClock(t *testing.T) {
	withShortStableWindow(t, 20*time.Millisecond)
	m := &Manager{}
	m.StartOfflineBudget(100 * time.Millisecond)
	m.AttachStream("s1")
	time.Sleep(60 * time.Millisecond) // stable, deadline restarted at ~20ms
	m.DetachStream("s1")              // clock restarts: deadline = now+100ms
	time.Sleep(70 * time.Millisecond)
	if exhausted(m) {
		t.Fatal("exhausted before a full budget after the stable attachment")
	}
	eventually(t, "exhaustion", func() bool { return exhausted(m) })
}

func TestOfflineBudgetStaysAttachedPastDeadlineNoExpiryWhenStable(t *testing.T) {
	withShortStableWindow(t, 10*time.Millisecond)
	m := &Manager{}
	m.StartOfflineBudget(60 * time.Millisecond)
	m.AttachStream("s1")
	time.Sleep(150 * time.Millisecond)
	if exhausted(m) {
		t.Fatal("exhausted while attached and stable")
	}
}

func TestOfflineBudgetDeadlinePassedWhileUnstableDecidesAtNextDetach(t *testing.T) {
	withShortStableWindow(t, time.Hour)
	m := &Manager{}
	m.StartOfflineBudget(40 * time.Millisecond)
	m.AttachStream("s1")
	time.Sleep(100 * time.Millisecond)
	if exhausted(m) {
		t.Fatal("exhausted while attached")
	}
	m.DetachStream("s1")
	if !exhausted(m) {
		t.Fatal("expiry decision not taken at the moment the instance became detached")
	}
}

func TestOfflineBudgetStreamAttachingFirstPreventsDecision(t *testing.T) {
	m := &Manager{}
	m.StartOfflineBudget(time.Hour)
	m.attach.mu.Lock()
	gen := m.attach.budget.gen
	m.attach.budget.deadline = time.Now().Add(-time.Second)
	m.attach.mu.Unlock()
	m.AttachStream("s1")
	m.decideBudget(gen)
	if exhausted(m) {
		t.Fatal("decision taken although a stream attached first")
	}
}

func TestOfflineBudgetStableAttachmentClearsExhaustion(t *testing.T) {
	withShortStableWindow(t, 20*time.Millisecond)
	m := &Manager{}
	m.StartOfflineBudget(20 * time.Millisecond)
	eventually(t, "exhaustion", func() bool { return exhausted(m) })
	m.AttachStream("s1")
	eventually(t, "exhaustion cleared", func() bool { return !exhausted(m) })
}

type cancelRecordingAdapter struct {
	*stubAdapter
	calls atomic.Int32
	err   error
	block chan struct{}
}

func (a *cancelRecordingAdapter) Cancel(ctx context.Context) error {
	a.calls.Add(1)
	if a.block != nil {
		select {
		case <-a.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return a.err
}

func budgetManagerWithTurn(t *testing.T, a *cancelRecordingAdapter, submission string) *Manager {
	t.Helper()
	m := &Manager{adapter: a, logger: newTestLogger(t)}
	if submission != "" {
		m.setBudgetInFlight(submission)
	}
	m.StartOfflineBudget(30 * time.Millisecond)
	return m
}

func recordState(m *Manager) budgetCancelState {
	m.attach.mu.Lock()
	defer m.attach.mu.Unlock()
	if m.attach.budget.record == nil {
		return ""
	}
	return m.attach.budget.record.state
}

func TestOfflineBudgetExpiryCancelsRunningTurn(t *testing.T) {
	a := &cancelRecordingAdapter{stubAdapter: newStubAdapter()}
	m := budgetManagerWithTurn(t, a, "sub-1")
	eventually(t, "issued cancel", func() bool { return recordState(m) == budgetCancelIssued })
	if a.calls.Load() != 1 {
		t.Fatalf("cancel calls = %d, want 1", a.calls.Load())
	}
}

func TestOfflineBudgetExpiryWithoutRunningTurnCancelsNothing(t *testing.T) {
	a := &cancelRecordingAdapter{stubAdapter: newStubAdapter()}
	m := budgetManagerWithTurn(t, a, "")
	eventually(t, "exhaustion", func() bool { return exhausted(m) })
	time.Sleep(50 * time.Millisecond)
	if a.calls.Load() != 0 || recordState(m) != "" {
		t.Fatalf("cancel calls = %d, record = %q; want none", a.calls.Load(), recordState(m))
	}
}

func TestOfflineBudgetCancelFailureIsRecordedAsFailedAndNotRetried(t *testing.T) {
	a := &cancelRecordingAdapter{stubAdapter: newStubAdapter(), err: errors.New("boom")}
	m := budgetManagerWithTurn(t, a, "sub-1")
	eventually(t, "failed cancel", func() bool { return recordState(m) == budgetCancelFailed })
	time.Sleep(50 * time.Millisecond)
	if a.calls.Load() != 1 {
		t.Fatalf("cancel calls = %d, want 1 (no retry)", a.calls.Load())
	}
	if stamp := m.stampTerminal("sub-1"); stamp.budgetCancel {
		t.Fatal("terminal stamped with the budget cause after a failed cancel")
	}
}

func TestOfflineBudgetCancelNotAcknowledgedWithinDeadlineIsFailed(t *testing.T) {
	prev := budgetCancelTimeout
	budgetCancelTimeout = 40 * time.Millisecond
	t.Cleanup(func() { budgetCancelTimeout = prev })
	a := &cancelRecordingAdapter{stubAdapter: newStubAdapter(), block: make(chan struct{})}
	t.Cleanup(func() { close(a.block) })
	m := budgetManagerWithTurn(t, a, "sub-1")
	eventually(t, "failed cancel", func() bool { return recordState(m) == budgetCancelFailed })
}

func TestOfflineBudgetCancelSkippedWhenTurnEndedBeforeCancel(t *testing.T) {
	a := &cancelRecordingAdapter{stubAdapter: newStubAdapter()}
	m := &Manager{adapter: a, logger: newTestLogger(t)}
	m.attach.budget.record = &budgetCancelRecord{submissionID: "sub-1", state: budgetCancelPending}
	m.setBudgetInFlight("sub-2") // successor turn
	m.cancelForBudget("sub-1")
	if a.calls.Load() != 0 || recordState(m) != budgetCancelSkipped {
		t.Fatalf("calls = %d, state = %q; want 0 and skipped", a.calls.Load(), recordState(m))
	}
}

func TestOfflineBudgetStateTransitionNeverRecreatesConsumedRecord(t *testing.T) {
	m := &Manager{logger: newTestLogger(t)}
	m.attach.mu.Lock()
	applied := m.setBudgetStateLocked("sub-1", budgetCancelIssued)
	m.attach.mu.Unlock()
	if applied || m.attach.budget.record != nil {
		t.Fatal("state transition created a record")
	}
}

func TestStampTerminalConsumesRecordAndForcesBudgetCauseWhenPendingOrIssued(t *testing.T) {
	for _, state := range []budgetCancelState{budgetCancelPending, budgetCancelIssued} {
		m := &Manager{}
		m.setBudgetInFlight("sub-1")
		m.attach.budget.record = &budgetCancelRecord{submissionID: "sub-1", state: state, decidedAt: time.Unix(5, 0)}
		stamp := m.stampTerminal("sub-1")
		if !stamp.budgetCancel || !stamp.detached || !stamp.cancelDecided.Equal(time.Unix(5, 0)) {
			t.Fatalf("state %s: stamp = %+v", state, stamp)
		}
		if m.attach.budget.record != nil || m.attach.budget.inFlight != "" {
			t.Fatalf("state %s: record or marker survived the stamp", state)
		}
	}
}

func TestStampTerminalIgnoresRecordOfAnotherSubmission(t *testing.T) {
	m := &Manager{}
	m.attach.budget.record = &budgetCancelRecord{submissionID: "sub-1", state: budgetCancelPending}
	if stamp := m.stampTerminal("sub-2"); stamp.budgetCancel {
		t.Fatal("stamped a later turn with an earlier decision")
	}
	if m.attach.budget.record == nil {
		t.Fatal("stamp of another submission deleted the record")
	}
}

func TestStampTerminalEmptySubmissionOnlyDetachedFields(t *testing.T) {
	m := &Manager{}
	m.attach.budget.record = &budgetCancelRecord{submissionID: "sub-1", state: budgetCancelPending}
	stamp := m.stampTerminal("")
	if stamp.budgetCancel || !stamp.detached || stamp.committedAt.IsZero() {
		t.Fatalf("stamp = %+v", stamp)
	}
}

func TestStampTerminalAttachedIsNotDetached(t *testing.T) {
	m := &Manager{}
	m.AttachStream("s1")
	if m.stampTerminal("sub-1").detached {
		t.Fatal("stamp reports detached while attached")
	}
}
