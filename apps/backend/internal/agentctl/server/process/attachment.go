package process

import (
	"context"
	"sync"
	"time"
)

// defaultOfflineBudget is used when an instance's configured budget is zero,
// matching the "absent or empty means 15 [minutes]" rule in system design
// part 2 "Budget configuration".
const defaultOfflineBudget = 15 * time.Minute

// WebSocket close codes for the agent stream (system design part 2 "Stream
// start" and "Offline budget"). Shared with the api package so both the
// caller-side close (superseded) and this package's own close (offline
// budget expiry) use one source of truth.
const (
	CloseCodeSuperseded    = 4001
	CloseCodeOfflineBudget = 4002
)

// BudgetPause records one offline-budget-exhausted event. It is journaled by
// budget enforcement; if the journal append fails after retries, a copy is
// kept in memory as the instance's unjournaled budget pause until the next
// Confirm.
type BudgetPause struct {
	DetachedSince time.Time
	ExhaustedAt   time.Time
	Outcome       string
	CancelError   error
}

// Attachment is the attachment-state snapshot reported to callers such as the
// delivery-status endpoint. AttachedAtSequence is meaningful only when
// Current is true; zero is a real value (the high water of an empty
// journal), never a sentinel for "detached".
type Attachment struct {
	Current            bool
	AttachID           string
	Confirmed          bool
	AttachedAtSequence uint64

	UnjournaledBudgetPause *BudgetPause
}

// AttachmentSnapshot is what a Kandev-call waiter reads from AttachmentWaiter
// to decide whether to send, wait for a reattach, or give up because the
// offline budget expired. All fields describe one instant.
type AttachmentSnapshot struct {
	// Attached reports whether the current stream is confirmed.
	Attached bool
	// Episode identifies the detached period AttachedCh/BudgetExhausted
	// belong to.
	Episode uint64
	// AttachedCh is closed when this episode ends by a confirmation.
	AttachedCh <-chan struct{}
	// BudgetExhausted is closed when this episode's offline budget expires.
	BudgetExhausted <-chan struct{}
}

// AttachmentWaiter is implemented by *attachmentState for task 02's Kandev
// call waiters, so they depend on a narrow read-only view rather than the
// whole state machine.
type AttachmentWaiter interface {
	Snapshot() AttachmentSnapshot
}

// currentStream describes the one stream agentctl considers current. close
// ends its socket with the given WebSocket close code and reason; done is
// closed by the caller once that stream's reader and writer goroutines have
// both exited.
type currentStream struct {
	streamID  string
	attachID  string
	confirmed bool
	close     func(code int, reason string)
	done      <-chan struct{}
}

// supersededStream is the old current stream a Stream start must close and
// wait for before it can become current itself.
type supersededStream struct {
	StreamID string
	Close    func(code int, reason string)
	Done     <-chan struct{}
}

// attachmentHooks are the effectful operations budget enforcement and the
// journal high-water read need. They are injected so attachmentState can be
// unit tested without a real journal, adapter, or process group, following
// the same pattern as Manager's groupAliveFn/terminateGroupFn/killGroupFn.
type attachmentHooks struct {
	now       func() time.Time
	afterFunc func(d time.Duration, f func()) *time.Timer
	after     func(d time.Duration) <-chan time.Time
	sleep     func(d time.Duration)

	// journalHighWater returns the journal's current high water. It backs
	// AttachedAtSequence and must never block on attachMu.
	journalHighWater func() uint64

	// hasActiveTurn reports whether the adapter has a turn in flight.
	hasActiveTurn func() bool
	// cancelTurn is the adapter's Cancel, already bounded by the caller's ctx.
	cancelTurn func(ctx context.Context) error
	// stopAgent stops the agent process group, as Manager.Stop does.
	stopAgent func(ctx context.Context) error
	// journalBudgetExhausted journals the agent_link.offline_budget_exhausted
	// event. A non-durable instance returns nil without writing anything.
	journalBudgetExhausted func(ctx context.Context, pause BudgetPause) error
	// onBudgetJournalFailed counts agent_link_budget_journal_failed_total.
	onBudgetJournalFailed func()
	// cancelPendingPermissions cancels any permission request of the instance
	// still pending after a cancelled or stopped outcome (system design part 2
	// "Permission requests"), the same way Manager.CancelPendingPermissions
	// does before a new prompt.
	cancelPendingPermissions func()
}

func (h *attachmentHooks) setDefaults() {
	if h.now == nil {
		h.now = time.Now
	}
	if h.afterFunc == nil {
		h.afterFunc = time.AfterFunc
	}
	if h.after == nil {
		h.after = time.After
	}
	if h.sleep == nil {
		h.sleep = time.Sleep
	}
	if h.journalHighWater == nil {
		h.journalHighWater = func() uint64 { return 0 }
	}
	if h.hasActiveTurn == nil {
		h.hasActiveTurn = func() bool { return false }
	}
	if h.cancelTurn == nil {
		h.cancelTurn = func(context.Context) error { return nil }
	}
	if h.stopAgent == nil {
		h.stopAgent = func(context.Context) error { return nil }
	}
	if h.journalBudgetExhausted == nil {
		h.journalBudgetExhausted = func(context.Context, BudgetPause) error { return nil }
	}
	if h.onBudgetJournalFailed == nil {
		h.onBudgetJournalFailed = func() {}
	}
	if h.cancelPendingPermissions == nil {
		h.cancelPendingPermissions = func() {}
	}
}

// attachmentState is the attachMu-guarded state described in system design
// part 2 "Attachment state". An instance starts detached in episode 1, with
// a fresh channel pair and the budget timer armed; no channel is ever nil.
type attachmentState struct {
	mu sync.Mutex

	current *currentStream

	episode uint64

	attachedCh  chan struct{}
	exhaustedCh chan struct{}

	detachedSince time.Time
	timer         *time.Timer

	attachedAtSequence uint64

	enforcing         bool
	attachWaitCh      chan struct{}
	enforcementDoneCh chan struct{}

	unjournaledPause *BudgetPause

	budget time.Duration
	hooks  attachmentHooks

	// startingID is the streamID of the most recent StreamStart call. It is
	// never cleared back to "" by a successful FinalizeStreamStart, so it
	// stays the authoritative "who is allowed to become current" answer even
	// when a THIRD stream (C) supersedes a second (B) while B is still
	// waiting to finalize: FinalizeStreamStart("b", ...) then finds
	// startingID == "c" and reports stillCurrent false without touching
	// current, which C's own finalize already installed. Empty means no
	// StreamStart has ever been called, so a caller that finalizes directly
	// (every attachment_test.go case that skips StreamStart for the first
	// stream) is never treated as stale.
	startingID string
}

// newAttachmentState builds the initial state: detached in episode 1, with
// the budget timer armed. budget <= 0 falls back to defaultOfflineBudget.
func newAttachmentState(budget time.Duration, hooks attachmentHooks) *attachmentState {
	hooks.setDefaults()
	if budget <= 0 {
		budget = defaultOfflineBudget
	}
	as := &attachmentState{
		episode:       1,
		attachedCh:    make(chan struct{}),
		exhaustedCh:   make(chan struct{}),
		detachedSince: hooks.now(),
		budget:        budget,
		hooks:         hooks,
	}
	as.timer = hooks.afterFunc(budget, func() { as.expire(1) })
	return as
}

// IsAttached reports whether a stream is current, confirmed or not -- the
// same question the atomic counter it replaces answered for its one existing
// caller, sendPermissionNotification.
func (as *attachmentState) IsAttached() bool {
	as.mu.Lock()
	defer as.mu.Unlock()
	return as.current != nil
}

// Snapshot implements AttachmentWaiter.
func (as *attachmentState) Snapshot() AttachmentSnapshot {
	as.mu.Lock()
	defer as.mu.Unlock()
	return AttachmentSnapshot{
		Attached:        as.current != nil && as.current.confirmed,
		Episode:         as.episode,
		AttachedCh:      as.attachedCh,
		BudgetExhausted: as.exhaustedCh,
	}
}

// Enforcing reports whether budget enforcement is currently running, for the
// unowned reaper gate (system design part 2 "Unowned reaper").
func (as *attachmentState) Enforcing() bool {
	as.mu.Lock()
	defer as.mu.Unlock()
	return as.enforcing
}

// Status reports the attachment state for the delivery-status endpoint.
func (as *attachmentState) Status() Attachment {
	as.mu.Lock()
	defer as.mu.Unlock()
	a := Attachment{AttachedAtSequence: as.attachedAtSequence}
	if as.current != nil {
		a.Current = true
		a.AttachID = as.current.attachID
		a.Confirmed = as.current.confirmed
	}
	if as.unjournaledPause != nil {
		pause := *as.unjournaledPause
		a.UnjournaledBudgetPause = &pause
	}
	return a
}

// closeAttachWaitLocked closes attachWaitCh at most once. Called with mu
// held.
func closeAttachWaitLocked(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}

// StreamStart begins a new stream. If enforcement is running, it first waits
// (outside further lock holds) for enforcement to end or for ctx to be done;
// a stream start that observes enforcement running closes attachWaitCh so a
// stop retry waiting on the 60s timer resolves at once with outcome
// stop_failed instead of repeating the stop. It then reports the stream
// current before the call, if any, so the caller can close it and wait for
// its goroutines to exit before calling FinalizeStreamStart.
func (as *attachmentState) StreamStart(ctx context.Context, streamID string) (*supersededStream, error) {
	as.mu.Lock()
	if as.enforcing {
		closeAttachWaitLocked(as.attachWaitCh)
		done := as.enforcementDoneCh
		as.mu.Unlock()
		select {
		case <-done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		as.mu.Lock()
	}
	defer as.mu.Unlock()
	as.startingID = streamID
	if as.current == nil {
		return nil, nil
	}
	old := as.current
	return &supersededStream{StreamID: old.streamID, Close: old.close, Done: old.done}, nil
}

// FinalizeStreamStart re-checks under the lock that streamID is still the
// stream meant to become current -- a later start may have superseded it
// while the caller waited for the old stream's goroutines to exit -- and, if
// so, installs it. attachID empty means the stream was opened without the
// query parameter and is confirmed at once. stillCurrent false means the
// caller must close its own socket with code 4001 reason "superseded" and
// start no reader or writer.
func (as *attachmentState) FinalizeStreamStart(
	streamID, attachID string,
	closeFn func(code int, reason string),
	done <-chan struct{},
) (attachedAtSequence uint64, confirmed bool, stillCurrent bool) {
	as.mu.Lock()
	defer as.mu.Unlock()
	if as.startingID != "" && as.startingID != streamID {
		return 0, false, false
	}
	oldConfirmed := as.current != nil && as.current.confirmed
	confirmed = attachID == ""
	as.current = &currentStream{streamID: streamID, attachID: attachID, confirmed: confirmed, close: closeFn, done: done}
	as.attachedAtSequence = as.hooks.journalHighWater()
	switch {
	case confirmed:
		as.attachLocked()
	case oldConfirmed:
		as.detachLocked()
	}
	return as.attachedAtSequence, confirmed, true
}

// StreamEnd applies the Stream end transition: it changes nothing unless
// streamID is current, in which case no stream is current afterward, and a
// detached period starts if it was confirmed.
func (as *attachmentState) StreamEnd(streamID string) {
	as.mu.Lock()
	defer as.mu.Unlock()
	if as.current == nil || as.current.streamID != streamID {
		return
	}
	wasConfirmed := as.current.confirmed
	as.current = nil
	if wasConfirmed {
		as.detachLocked()
	}
}

// ConfirmResult is the outcome of a POST .../agent/stream/confirm call.
type ConfirmResult int

const (
	// ConfirmMatched means the caller should answer 204, whether this call
	// changed anything (first confirmation) or not (idempotent retry).
	ConfirmMatched ConfirmResult = iota
	// ConfirmNotCurrent means the caller should answer 409
	// ATTACH_NOT_CURRENT: no stream is current, or it has another attach_id.
	ConfirmNotCurrent
)

// Confirm implements the Confirm transition.
func (as *attachmentState) Confirm(attachID string) ConfirmResult {
	as.mu.Lock()
	defer as.mu.Unlock()
	if as.current == nil || as.current.attachID != attachID {
		return ConfirmNotCurrent
	}
	if !as.current.confirmed {
		as.current.confirmed = true
		as.attachLocked()
	}
	return ConfirmMatched
}

// attachLocked implements Offline budget's "Attach": stop the timer, close
// attachedCh, clear detachedSince, and clear any unjournaled budget pause.
// Called with mu held.
func (as *attachmentState) attachLocked() {
	if as.timer != nil {
		as.timer.Stop()
	}
	closeAttachWaitLocked(as.attachedCh)
	as.detachedSince = time.Time{}
	as.unjournaledPause = nil
}

// detachLocked implements Offline budget's "Detach": increment episode,
// record detachedSince, create a fresh channel pair, and arm the timer.
// Called with mu held.
func (as *attachmentState) detachLocked() {
	as.episode++
	as.detachedSince = as.hooks.now()
	as.attachedCh = make(chan struct{})
	as.exhaustedCh = make(chan struct{})
	episode := as.episode
	if as.timer != nil {
		as.timer.Stop()
	}
	as.timer = as.hooks.afterFunc(as.budget, func() { as.expire(episode) })
}

// expire implements Offline budget's "Expire". It is a no-op for a stale,
// attached, or already-exhausted episode. Otherwise it closes exhaustedCh,
// ends a current unconfirmed stream with close code 4002 reason
// offline_budget, and starts enforcement.
func (as *attachmentState) expire(episode uint64) {
	as.mu.Lock()
	if episode != as.episode || (as.current != nil && as.current.confirmed) {
		as.mu.Unlock()
		return
	}
	select {
	case <-as.exhaustedCh:
		as.mu.Unlock()
		return
	default:
	}
	close(as.exhaustedCh)
	var toClose *currentStream
	if as.current != nil {
		toClose = as.current
		as.current = nil
	}
	as.enforcing = true
	as.attachWaitCh = make(chan struct{})
	as.enforcementDoneCh = make(chan struct{})
	as.mu.Unlock()

	if toClose != nil {
		toClose.close(CloseCodeOfflineBudget, "offline_budget")
		if toClose.done != nil {
			<-toClose.done
		}
	}
	go as.enforce()
}
