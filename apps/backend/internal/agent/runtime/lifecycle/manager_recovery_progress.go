package lifecycle

import (
	"context"

	"github.com/kandev/kandev/internal/startup"
	"github.com/kandev/kandev/internal/task/models"
)

// recoveryProgressLedger tracks conclusive resolution of one startup pass's
// filtered recovery candidates. Progress is counted per input row, so a
// duplicate backend result can never advance more work than the pass was
// given, and a result naming no input session advances nothing.
type recoveryProgressLedger struct {
	resolved  []bool
	bySession map[string][]int
	pending   map[string]struct{}
}

func newRecoveryProgressLedger(records []*models.ExecutorRunning) *recoveryProgressLedger {
	ledger := &recoveryProgressLedger{
		resolved:  make([]bool, len(records)),
		bySession: make(map[string][]int, len(records)),
		pending:   make(map[string]struct{}),
	}
	for index, record := range records {
		if record != nil && record.SessionID != "" {
			ledger.bySession[record.SessionID] = append(ledger.bySession[record.SessionID], index)
		}
	}
	return ledger
}

// resolve marks every input row correlated to sessionID as conclusively
// classified and returns how many rows this call newly resolved. Repeated
// calls for the same session are idempotent.
func (l *recoveryProgressLedger) resolve(sessionID string) int {
	resolved := 0
	for _, index := range l.bySession[sessionID] {
		if l.resolved[index] {
			continue
		}
		l.resolved[index] = true
		resolved++
	}
	return resolved
}

// advance publishes newly resolved candidates immediately so live startup
// progress keeps moving while later candidates are still being processed.
func (l *recoveryProgressLedger) advance(ctx context.Context, sessionID string) {
	if resolved := l.resolve(sessionID); resolved > 0 {
		startup.Advance(ctx, startup.StepSessionsRecovery, int64(resolved))
	}
}

// resolveAfterStop defers a conclusive refusal until the pass's owned stop
// workers have settled. The candidate is classified, but the step must not
// claim completion while its cleanup obligation is still running.
func (l *recoveryProgressLedger) resolveAfterStop(sessionID string) {
	if sessionID == "" {
		return
	}
	l.pending[sessionID] = struct{}{}
}

// settleStops applies every deferred refusal after stop workers finish and
// returns how many rows this call newly resolved.
func (l *recoveryProgressLedger) settleStops() int {
	resolved := 0
	for sessionID := range l.pending {
		resolved += l.resolve(sessionID)
	}
	l.pending = make(map[string]struct{})
	return resolved
}
