package process

import (
	"context"
	"time"
)

// Budget enforcement outcomes, journaled in agent_link.offline_budget_exhausted.
const (
	budgetOutcomeCancelled  = "cancelled"
	budgetOutcomeStopped    = "stopped"
	budgetOutcomeStopFailed = "stop_failed"
)

const (
	cancelAttempts    = 3
	cancelTimeout     = 10 * time.Second
	cancelRetryDelay  = 2 * time.Second
	stopRetryWait     = 60 * time.Second
	journalAttempts   = 3
	journalRetryDelay = 1 * time.Second
)

// enforce runs budget enforcement (system design part 2 "Budget
// enforcement") in its own goroutine, outside attachMu. It ends without
// journaling anything if no turn is active; otherwise it cancels the turn,
// escalating to a process-group stop if cancellation fails, then journals
// the outcome. Only one enforce goroutine ever runs at a time: a new stream
// start blocks on enforcementDoneCh while enforcing is true, so no later
// episode can begin until this one ends.
func (as *attachmentState) enforce() {
	ctx := context.Background()
	if !as.hooks.hasActiveTurn() {
		as.endEnforcement(nil)
		return
	}

	outcome, lastErr := as.cancelWithRetries(ctx)
	if outcome == budgetOutcomeCancelled {
		as.hooks.cancelPendingPermissions()
		as.journalAndEnd(outcome, lastErr)
		return
	}

	for {
		stopErr := as.hooks.stopAgent(ctx)
		if stopErr == nil {
			as.hooks.cancelPendingPermissions()
			as.journalAndEnd(budgetOutcomeStopped, lastErr)
			return
		}
		lastErr = stopErr
		if as.awaitStopRetryOrAttachWait() {
			as.journalAndEnd(budgetOutcomeStopFailed, lastErr)
			return
		}
	}
}

// cancelWithRetries calls the adapter's Cancel up to cancelAttempts times,
// cancelRetryDelay apart, bounded by cancelTimeout each. It checks the turn
// again before each attempt, so a turn that ends on its own -- including
// while a retry is waiting -- ends the cancel with outcome cancelled.
func (as *attachmentState) cancelWithRetries(ctx context.Context) (outcome string, lastErr error) {
	for attempt := 0; attempt < cancelAttempts; attempt++ {
		if !as.hooks.hasActiveTurn() {
			return budgetOutcomeCancelled, nil
		}
		cctx, cancel := context.WithTimeout(ctx, cancelTimeout)
		err := as.hooks.cancelTurn(cctx)
		cancel()
		if err == nil {
			return budgetOutcomeCancelled, nil
		}
		lastErr = err
		if attempt < cancelAttempts-1 {
			as.hooks.sleep(cancelRetryDelay)
		}
	}
	return "", lastErr
}

// awaitStopRetryOrAttachWait waits for either a stream start's attachWaitCh
// signal or the stopRetryWait timer, with attachWaitCh checked first so a
// waiter that arrived before this wait began is never made to wait for
// another stop attempt. It returns true when attachWaitCh wins (the caller
// journals outcome stop_failed and ends without another stop) and false when
// the timer wins (the caller repeats the stop).
func (as *attachmentState) awaitStopRetryOrAttachWait() bool {
	as.mu.Lock()
	attachWaitCh := as.attachWaitCh
	as.mu.Unlock()

	select {
	case <-attachWaitCh:
		return true
	default:
	}
	select {
	case <-attachWaitCh:
		return true
	case <-as.hooks.after(stopRetryWait):
		return false
	}
}

// journalAndEnd journals agent_link.offline_budget_exhausted, retrying up to
// journalAttempts times journalRetryDelay apart, then ends enforcement. A
// non-durable instance's hook returns nil without writing anything, so this
// always succeeds for it.
func (as *attachmentState) journalAndEnd(outcome string, cancelErr error) {
	as.mu.Lock()
	pause := BudgetPause{DetachedSince: as.detachedSince, ExhaustedAt: as.hooks.now(), Outcome: outcome, CancelError: cancelErr}
	as.mu.Unlock()

	var journalErr error
	for attempt := 0; attempt < journalAttempts; attempt++ {
		journalErr = as.hooks.journalBudgetExhausted(context.Background(), pause)
		if journalErr == nil {
			break
		}
		if attempt < journalAttempts-1 {
			as.hooks.sleep(journalRetryDelay)
		}
	}
	if journalErr != nil {
		as.hooks.onBudgetJournalFailed()
		pauseCopy := pause
		as.endEnforcement(&pauseCopy)
		return
	}
	as.endEnforcement(nil)
}

// endEnforcement ends enforcement: it sets enforcing false and closes
// enforcementDoneCh, in one attachMu hold, releasing any stream start
// waiting on it. A non-nil pause is kept as the unjournaled budget pause
// until the next Confirm.
func (as *attachmentState) endEnforcement(unjournaledPause *BudgetPause) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.enforcing = false
	as.unjournaledPause = unjournaledPause
	closeAttachWaitLocked(as.enforcementDoneCh)
}
