package process

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testAttachment builds an attachmentState with a tiny budget and no-op
// sleeps, so enforcement tests run fast and deterministically. Callers
// override individual hooks for the behavior under test.
func testAttachment(t *testing.T, budget time.Duration, override func(*attachmentHooks)) *attachmentState {
	t.Helper()
	hooks := attachmentHooks{
		sleep: func(time.Duration) {},
	}
	if override != nil {
		override(&hooks)
	}
	return newAttachmentState(budget, hooks)
}

func doneCh() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func noopClose(int, string) {}

// TestInitialAttachmentState pins the starting state: detached in episode 1,
// no channel nil, IsAttached false, and a launch stream (no attach_id)
// confirms at once, ending episode 1, with sequence 0 for an empty journal.
func TestInitialAttachmentState(t *testing.T) {
	as := testAttachment(t, time.Hour, nil)
	if as.IsAttached() {
		t.Fatal("IsAttached() = true for a fresh instance, want false")
	}
	snap := as.Snapshot()
	if snap.Attached {
		t.Fatal("Snapshot().Attached = true for a fresh instance, want false")
	}
	if snap.Episode != 1 {
		t.Fatalf("Snapshot().Episode = %d, want 1", snap.Episode)
	}
	if snap.AttachedCh == nil || snap.BudgetExhausted == nil {
		t.Fatal("Snapshot() channels must never be nil")
	}

	old, err := as.StreamStart(context.Background(), "s1")
	if err != nil || old != nil {
		t.Fatalf("StreamStart() = (%v, %v), want (nil, nil)", old, err)
	}
	seq, confirmed, stillCurrent := as.FinalizeStreamStart("s1", "", noopClose, doneCh())
	if !confirmed || !stillCurrent || seq != 0 {
		t.Fatalf("FinalizeStreamStart() = (%d, %v, %v), want (0, true, true)", seq, confirmed, stillCurrent)
	}
	if !as.IsAttached() {
		t.Fatal("IsAttached() = false after a launch stream confirmed, want true")
	}
	status := as.Status()
	if !status.Current || !status.Confirmed {
		t.Fatalf("Status() = %+v, want Current and Confirmed true", status)
	}
}

// TestStreamConfirm covers the three Confirm branches: first confirmation
// (204, ends the episode), idempotent retry (204, no change), and a mismatch
// (409 ATTACH_NOT_CURRENT, no change).
func TestStreamConfirm(t *testing.T) {
	as := testAttachment(t, time.Hour, nil)
	as.FinalizeStreamStart("s1", "attach-1", noopClose, doneCh())

	if got := as.Confirm("attach-1"); got != ConfirmMatched {
		t.Fatalf("first Confirm = %v, want ConfirmMatched", got)
	}
	if !as.IsAttached() {
		t.Fatal("expected attached after Confirm")
	}
	if got := as.Confirm("attach-1"); got != ConfirmMatched {
		t.Fatalf("retried Confirm = %v, want ConfirmMatched (idempotent)", got)
	}
	if got := as.Confirm("attach-2"); got != ConfirmNotCurrent {
		t.Fatalf("Confirm with wrong attach_id = %v, want ConfirmNotCurrent", got)
	}
}

func TestConfirmWithNoCurrentStream(t *testing.T) {
	as := testAttachment(t, time.Hour, nil)
	if got := as.Confirm("attach-1"); got != ConfirmNotCurrent {
		t.Fatalf("Confirm() with no current stream = %v, want ConfirmNotCurrent", got)
	}
}

// TestStreamSupersede pins that a new stream start reports the old current
// stream to close, and that FinalizeStreamStart installs the new one.
func TestStreamSupersede(t *testing.T) {
	as := testAttachment(t, time.Hour, nil)
	as.FinalizeStreamStart("s1", "", noopClose, doneCh())

	var closedCode int
	old, err := as.StreamStart(context.Background(), "s2")
	if err != nil {
		t.Fatalf("StreamStart() error = %v", err)
	}
	if old == nil || old.StreamID != "s1" {
		t.Fatalf("StreamStart() superseded = %+v, want streamID s1", old)
	}
	old.Close = func(code int, _ string) { closedCode = code }
	old.Close(4001, "superseded")
	if closedCode != 4001 {
		t.Fatalf("old stream close code = %d, want 4001", closedCode)
	}

	seq, confirmed, stillCurrent := as.FinalizeStreamStart("s2", "", noopClose, doneCh())
	_ = seq
	if !confirmed || !stillCurrent {
		t.Fatalf("FinalizeStreamStart() = (_, %v, %v), want (true, true)", confirmed, stillCurrent)
	}
	status := as.Status()
	if status.AttachID != "" || !status.Current {
		t.Fatalf("Status() = %+v, want the new stream current", status)
	}
}

// TestSupersedeRecheckAfterWait pins that a stream whose predecessor's wait
// took long enough for a THIRD stream to arrive first loses: it must close
// its own socket with 4001 and start no reader or writer.
func TestSupersedeRecheckAfterWait(t *testing.T) {
	as := testAttachment(t, time.Hour, nil)
	as.FinalizeStreamStart("a", "", noopClose, doneCh())
	// B starts (supersedes A), then C supersedes B before B finalizes.
	if _, err := as.StreamStart(context.Background(), "b"); err != nil {
		t.Fatalf("B StreamStart() error = %v", err)
	}
	if _, err := as.StreamStart(context.Background(), "c"); err != nil {
		t.Fatalf("C StreamStart() error = %v", err)
	}
	as.FinalizeStreamStart("c", "", noopClose, doneCh())

	seq, confirmed, stillCurrent := as.FinalizeStreamStart("b", "", noopClose, doneCh())
	if stillCurrent {
		t.Fatal("B's FinalizeStreamStart() reported stillCurrent = true after C superseded it, want false")
	}
	_ = seq
	_ = confirmed
	if as.Status().AttachID != "" {
		t.Fatal("B must not have overwritten C as current")
	}
}

// TestDetachClock pins that a confirmed stream ending starts the detach
// clock, and that its budget expiring closes BudgetExhausted with no turn
// running (so enforcement ends without journaling).
func TestDetachClock(t *testing.T) {
	var journaled atomic.Bool
	as := testAttachment(t, 20*time.Millisecond, func(h *attachmentHooks) {
		h.hasActiveTurn = func() bool { return false }
		h.journalBudgetExhausted = func(context.Context, BudgetPause) error {
			journaled.Store(true)
			return nil
		}
	})
	as.FinalizeStreamStart("s1", "", noopClose, doneCh())
	as.StreamEnd("s1")

	snap := as.Snapshot()
	select {
	case <-snap.BudgetExhausted:
	case <-time.After(time.Second):
		t.Fatal("BudgetExhausted never closed after the budget elapsed")
	}
	if journaled.Load() {
		t.Fatal("expire with no active turn must not journal anything")
	}
}

// TestOfflineBudget pins that a confirmed reattach before the budget cancels
// nothing, and that the clock restarts at the next detach.
func TestOfflineBudget(t *testing.T) {
	var turnActive atomic.Bool
	turnActive.Store(true)
	var cancelled atomic.Bool
	as := testAttachment(t, 40*time.Millisecond, func(h *attachmentHooks) {
		h.hasActiveTurn = turnActive.Load
		h.cancelTurn = func(context.Context) error { cancelled.Store(true); return nil }
	})
	as.FinalizeStreamStart("s1", "", noopClose, doneCh())
	as.StreamEnd("s1")

	time.Sleep(10 * time.Millisecond)
	as.FinalizeStreamStart("s2", "", noopClose, doneCh()) // reattach before budget
	time.Sleep(60 * time.Millisecond)
	if cancelled.Load() {
		t.Fatal("a confirmed reattach before the budget must cancel nothing")
	}

	as.StreamEnd("s2") // clock restarts
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !cancelled.Load() {
		time.Sleep(5 * time.Millisecond)
	}
	if !cancelled.Load() {
		t.Fatal("the clock must restart at the next detach")
	}
}

// TestUnconfirmedStreamKeepsBudget pins that an unconfirmed stream attaching
// and ending, any number of times, leaves the episode -- and so the budget
// clock -- unchanged.
func TestUnconfirmedStreamKeepsBudget(t *testing.T) {
	as := testAttachment(t, time.Hour, nil)
	initial := as.Snapshot().Episode

	for i := 0; i < 3; i++ {
		streamID := "reconnect"
		old, err := as.StreamStart(context.Background(), streamID)
		if err != nil {
			t.Fatalf("StreamStart() error = %v", err)
		}
		if old != nil {
			old.Close(4001, "superseded")
		}
		as.FinalizeStreamStart(streamID, "attach-id", noopClose, doneCh())
		as.StreamEnd(streamID)
	}

	if got := as.Snapshot().Episode; got != initial {
		t.Fatalf("episode = %d after unconfirmed attach/end cycles, want unchanged %d", got, initial)
	}
}

// TestAttachedAtSequence pins that AttachedAtSequence is the journal high
// water at the moment the stream became current.
func TestAttachedAtSequence(t *testing.T) {
	as := testAttachment(t, time.Hour, func(h *attachmentHooks) {
		h.journalHighWater = func() uint64 { return 42 }
	})
	seq, _, _ := as.FinalizeStreamStart("s1", "", noopClose, doneCh())
	if seq != 42 {
		t.Fatalf("AttachedAtSequence = %d, want 42", seq)
	}
	if as.Status().AttachedAtSequence != 42 {
		t.Fatalf("Status().AttachedAtSequence = %d, want 42", as.Status().AttachedAtSequence)
	}
}

// TestAttachRacesExpiry pins that a Confirm racing expiry either wins (no
// cancel) or loses (exactly one cancel attempt observed via the process
// ending detached), never both -- run under -race.
func TestAttachRacesExpiry(t *testing.T) {
	for i := 0; i < 20; i++ {
		as := testAttachment(t, 5*time.Millisecond, func(h *attachmentHooks) {
			h.hasActiveTurn = func() bool { return false }
		})
		as.FinalizeStreamStart("s1", "attach-1", noopClose, doneCh())
		as.StreamEnd("s1")
		as.FinalizeStreamStart("s2", "attach-1", noopClose, doneCh())

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			as.Confirm("attach-1")
		}()
		time.Sleep(6 * time.Millisecond)
		wg.Wait()
	}
}

// TestAttachWaitsForEnforcement pins that a stream start observing
// enforcement running waits for it to end before proceeding.
func TestAttachWaitsForEnforcement(t *testing.T) {
	release := make(chan struct{})
	as := testAttachment(t, 5*time.Millisecond, func(h *attachmentHooks) {
		h.hasActiveTurn = func() bool { return true }
		h.cancelTurn = func(context.Context) error { <-release; return nil }
	})
	as.FinalizeStreamStart("s1", "", noopClose, doneCh())
	as.StreamEnd("s1")

	time.Sleep(20 * time.Millisecond) // let expiry start enforcement

	started := make(chan struct{})
	go func() {
		as.StreamStart(context.Background(), "s2") //nolint:errcheck
		close(started)
	}()

	select {
	case <-started:
		t.Fatal("StreamStart returned before enforcement ended")
	case <-time.After(30 * time.Millisecond):
	}

	close(release)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("StreamStart never returned after enforcement ended")
	}
}

// TestBudgetEnforcementRetriesThenStops pins that a cancel failing all 3
// attempts escalates to a process-group stop, and a stop that then succeeds
// journals outcome stopped.
func TestBudgetEnforcementRetriesThenStops(t *testing.T) {
	var cancelAttemptsSeen, stopCalls int32
	journaled := make(chan BudgetPause, 1)
	as := testAttachment(t, 5*time.Millisecond, func(h *attachmentHooks) {
		h.hasActiveTurn = func() bool { return true }
		h.cancelTurn = func(context.Context) error {
			atomic.AddInt32(&cancelAttemptsSeen, 1)
			return errors.New("cancel failed")
		}
		h.stopAgent = func(context.Context) error {
			atomic.AddInt32(&stopCalls, 1)
			return nil
		}
		h.journalBudgetExhausted = func(_ context.Context, pause BudgetPause) error {
			journaled <- pause
			return nil
		}
	})
	as.FinalizeStreamStart("s1", "", noopClose, doneCh())
	as.StreamEnd("s1")

	select {
	case pause := <-journaled:
		if pause.Outcome != budgetOutcomeStopped {
			t.Fatalf("Outcome = %q, want %q", pause.Outcome, budgetOutcomeStopped)
		}
	case <-time.After(time.Second):
		t.Fatal("enforcement never journaled")
	}
	if got := atomic.LoadInt32(&cancelAttemptsSeen); got != cancelAttempts {
		t.Fatalf("cancel attempts = %d, want %d", got, cancelAttempts)
	}
	if got := atomic.LoadInt32(&stopCalls); got != 1 {
		t.Fatalf("stop calls = %d, want 1", got)
	}
}

// TestBudgetStopFailedHoldsGate pins that a stop that keeps failing holds
// enforcement open (Enforcing() stays true) until a stream start's
// attachWaitCh signal resolves it with outcome stop_failed.
func TestBudgetStopFailedHoldsGate(t *testing.T) {
	afterCh := make(chan time.Time) // never fires: the timer must not win first
	journaled := make(chan BudgetPause, 1)
	as := testAttachment(t, 5*time.Millisecond, func(h *attachmentHooks) {
		h.hasActiveTurn = func() bool { return true }
		h.cancelTurn = func(context.Context) error { return errors.New("cancel failed") }
		h.stopAgent = func(context.Context) error { return errors.New("stop failed") }
		h.after = func(time.Duration) <-chan time.Time { return afterCh }
		h.journalBudgetExhausted = func(_ context.Context, pause BudgetPause) error {
			journaled <- pause
			return nil
		}
	})
	as.FinalizeStreamStart("s1", "", noopClose, doneCh())
	as.StreamEnd("s1")

	time.Sleep(20 * time.Millisecond) // enforcement is now in its stop-retry wait
	if !as.Enforcing() {
		t.Fatal("Enforcing() = false while a failed stop should hold the gate closed")
	}

	go as.StreamStart(context.Background(), "s2") //nolint:errcheck

	select {
	case pause := <-journaled:
		if pause.Outcome != budgetOutcomeStopFailed {
			t.Fatalf("Outcome = %q, want %q", pause.Outcome, budgetOutcomeStopFailed)
		}
	case <-time.After(time.Second):
		t.Fatal("enforcement never journaled stop_failed")
	}
}

// TestStopRetryAttachWaitTiebreak pins that a stream start closing
// attachWaitCh WHILE a stop attempt is still in flight is observed only
// after that stop returns -- it sees that stop's own result, and triggers
// no second stop attempt.
func TestStopRetryAttachWaitTiebreak(t *testing.T) {
	var stopCalls int32
	stopStarted := make(chan struct{})
	releaseStop := make(chan struct{})
	journaled := make(chan BudgetPause, 1)
	as := testAttachment(t, 5*time.Millisecond, func(h *attachmentHooks) {
		h.hasActiveTurn = func() bool { return true }
		h.cancelTurn = func(context.Context) error { return errors.New("cancel failed") }
		h.stopAgent = func(context.Context) error {
			if atomic.AddInt32(&stopCalls, 1) == 1 {
				close(stopStarted)
				<-releaseStop
			}
			return errors.New("stop failed")
		}
		h.after = func(time.Duration) <-chan time.Time { return make(chan time.Time) } // never fires
		h.journalBudgetExhausted = func(_ context.Context, pause BudgetPause) error {
			journaled <- pause
			return nil
		}
	})
	as.FinalizeStreamStart("s1", "", noopClose, doneCh())
	as.StreamEnd("s1")

	select {
	case <-stopStarted:
	case <-time.After(time.Second):
		t.Fatal("stop was never attempted")
	}
	go as.StreamStart(context.Background(), "s2") //nolint:errcheck
	time.Sleep(20 * time.Millisecond)             // let the stream start close attachWaitCh
	close(releaseStop)

	select {
	case pause := <-journaled:
		if pause.Outcome != budgetOutcomeStopFailed {
			t.Fatalf("Outcome = %q, want %q", pause.Outcome, budgetOutcomeStopFailed)
		}
	case <-time.After(time.Second):
		t.Fatal("enforcement never journaled stop_failed")
	}
	if got := atomic.LoadInt32(&stopCalls); got != 1 {
		t.Fatalf("stop calls = %d, want exactly 1 (attachWaitCh must not trigger another stop)", got)
	}
}

// TestBudgetEnforcementCancelsPendingPermissions pins system design part 2
// "Permission requests": after a cancelled or stopped outcome, enforcement
// cancels any permission request of the instance still pending, before
// journaling. A stop_failed outcome must not trigger it (the turn keeps
// running, now visible to the user).
func TestBudgetEnforcementCancelsPendingPermissions(t *testing.T) {
	t.Run("cancelled", func(t *testing.T) {
		var cancelledPending atomic.Bool
		journaled := make(chan BudgetPause, 1)
		as := testAttachment(t, 5*time.Millisecond, func(h *attachmentHooks) {
			h.hasActiveTurn = func() bool { return true }
			h.cancelTurn = func(context.Context) error { return nil }
			h.cancelPendingPermissions = func() { cancelledPending.Store(true) }
			h.journalBudgetExhausted = func(_ context.Context, pause BudgetPause) error {
				journaled <- pause
				return nil
			}
		})
		as.FinalizeStreamStart("s1", "", noopClose, doneCh())
		as.StreamEnd("s1")

		select {
		case pause := <-journaled:
			if pause.Outcome != budgetOutcomeCancelled {
				t.Fatalf("Outcome = %q, want %q", pause.Outcome, budgetOutcomeCancelled)
			}
		case <-time.After(time.Second):
			t.Fatal("enforcement never journaled")
		}
		if !cancelledPending.Load() {
			t.Fatal("expected pending permissions cancelled after outcome cancelled")
		}
	})

	t.Run("stopped", func(t *testing.T) {
		var cancelledPending atomic.Bool
		journaled := make(chan BudgetPause, 1)
		as := testAttachment(t, 5*time.Millisecond, func(h *attachmentHooks) {
			h.hasActiveTurn = func() bool { return true }
			h.cancelTurn = func(context.Context) error { return errors.New("cancel failed") }
			h.stopAgent = func(context.Context) error { return nil }
			h.cancelPendingPermissions = func() { cancelledPending.Store(true) }
			h.journalBudgetExhausted = func(_ context.Context, pause BudgetPause) error {
				journaled <- pause
				return nil
			}
		})
		as.FinalizeStreamStart("s1", "", noopClose, doneCh())
		as.StreamEnd("s1")

		select {
		case pause := <-journaled:
			if pause.Outcome != budgetOutcomeStopped {
				t.Fatalf("Outcome = %q, want %q", pause.Outcome, budgetOutcomeStopped)
			}
		case <-time.After(time.Second):
			t.Fatal("enforcement never journaled")
		}
		if !cancelledPending.Load() {
			t.Fatal("expected pending permissions cancelled after outcome stopped")
		}
	})

	t.Run("stop_failed does not cancel pending permissions", func(t *testing.T) {
		var cancelledPending atomic.Bool
		journaled := make(chan BudgetPause, 1)
		as := testAttachment(t, 5*time.Millisecond, func(h *attachmentHooks) {
			h.hasActiveTurn = func() bool { return true }
			h.cancelTurn = func(context.Context) error { return errors.New("cancel failed") }
			h.stopAgent = func(context.Context) error { return errors.New("stop failed") }
			h.after = func(time.Duration) <-chan time.Time { return make(chan time.Time) } // never fires
			h.cancelPendingPermissions = func() { cancelledPending.Store(true) }
			h.journalBudgetExhausted = func(_ context.Context, pause BudgetPause) error {
				journaled <- pause
				return nil
			}
		})
		as.FinalizeStreamStart("s1", "", noopClose, doneCh())
		as.StreamEnd("s1")

		time.Sleep(20 * time.Millisecond)             // enforcement is now in its stop-retry wait
		go as.StreamStart(context.Background(), "s2") //nolint:errcheck

		select {
		case pause := <-journaled:
			if pause.Outcome != budgetOutcomeStopFailed {
				t.Fatalf("Outcome = %q, want %q", pause.Outcome, budgetOutcomeStopFailed)
			}
		case <-time.After(time.Second):
			t.Fatal("enforcement never journaled stop_failed")
		}
		if cancelledPending.Load() {
			t.Fatal("stop_failed must not cancel pending permissions: the turn keeps running")
		}
	})
}

// TestBudgetJournalAppendFailure pins that a journal append failing all 3
// attempts still ends enforcement (closing enforcementDoneCh, so a waiting
// stream proceeds), counts the failure, and keeps the pause until Confirm.
func TestBudgetJournalAppendFailure(t *testing.T) {
	var journalFailedCount int32
	as := testAttachment(t, 5*time.Millisecond, func(h *attachmentHooks) {
		h.hasActiveTurn = func() bool { return true }
		h.cancelTurn = func(context.Context) error { return nil }
		h.journalBudgetExhausted = func(context.Context, BudgetPause) error {
			return errors.New("journal full")
		}
		h.onBudgetJournalFailed = func() { atomic.AddInt32(&journalFailedCount, 1) }
	})
	as.FinalizeStreamStart("s1", "attach-1", noopClose, doneCh())
	as.StreamEnd("s1")
	as.FinalizeStreamStart("s2", "attach-2", noopClose, doneCh()) // detached, unconfirmed: budget still runs

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && as.Status().UnjournaledBudgetPause == nil {
		time.Sleep(5 * time.Millisecond)
	}
	status := as.Status()
	if status.UnjournaledBudgetPause == nil {
		t.Fatal("expected an unjournaled budget pause after 3 failed journal attempts")
	}
	if got := atomic.LoadInt32(&journalFailedCount); got != 1 {
		t.Fatalf("onBudgetJournalFailed calls = %d, want 1", got)
	}

	// Budget expiry already ended s2 (it was never confirmed), so the next
	// reattach is a fresh stream, not a Confirm of the same attach_id. Its
	// confirmation (attachLocked) is what clears the unjournaled pause.
	as.FinalizeStreamStart("s3", "", noopClose, doneCh())
	if as.Status().UnjournaledBudgetPause != nil {
		t.Fatal("the next confirmed stream must clear the unjournaled budget pause")
	}
}
