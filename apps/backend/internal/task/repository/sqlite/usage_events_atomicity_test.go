package sqlite

// Non-transient rollup-failure atomicity for CreateTaskUsageEvent
// (docs/specs/task-cost-ledger/spec.md AC-11, AC-32(d), AC-36): the ledger
// insert and the task_sessions rollup increment are applied in one
// transaction, so a failure on either side rolls both back rather than
// leaving a partial write. This is the ledger-writer replacement for the
// deleted office/service test
// TestPromptUsage_RollupFailureRollsBackLedgerInsertThenRetrySucceeds (AC-21,
// AC-36): the recovery path PR #2606 review asked to see covered - force a
// rollup failure, then redeliver the same usage_event_id - now proven at the
// writer that actually owns the transaction, since internal/task/usage's
// Writer no longer shares one with an Office caller.

import (
	"context"
	"errors"
	"testing"
)

func TestCreateTaskUsageEvent_NonTransientFailureRollsBackThenRedeliveryOfSameIDSucceeds(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-nontransient")
	createUsageEventsTestSession(t, repo, "session-nontransient", "task-nontransient")

	repo.failUsageEventRollupAttempts = 1
	repo.failUsageEventRollupErr = errors.New("injected rollup failure")

	event := newTestUsageEvent("evt-nontransient", "task-nontransient", "session-nontransient")

	// First delivery: the failpoint fires after the ledger row insert has
	// already been attempted inside the transaction, but before the rollup
	// increment - an error that is neither a duplicate, a foreign-key
	// violation, nor transient, so CreateTaskUsageEvent returns it
	// immediately with no internal retry. Because the insert and the rollup
	// share one transaction, the deferred rollback unwinds the already
	// -attempted insert along with everything else; a non-atomic
	// implementation that committed the insert in its own transaction before
	// attempting the rollup would leave the row behind here.
	err := repo.CreateTaskUsageEvent(context.Background(), event)
	if err == nil {
		t.Fatal("expected the injected rollup failure to surface, got nil")
	}
	if errors.Is(err, ErrDuplicateUsageEvent) {
		t.Fatalf("error = ErrDuplicateUsageEvent, want the raw injected failure")
	}
	if got := countTaskUsageEventRows(t, repo); got != 0 {
		t.Fatalf("row count after failed attempt = %d, want 0 (ledger insert must roll back with the failed rollup)", got)
	}
	tokensIn, tokensCachedIn, tokensOut, costSubcents := readTaskSessionRollup(t, repo, "session-nontransient")
	if tokensIn != 0 || tokensCachedIn != 0 || tokensOut != 0 || costSubcents != 0 {
		t.Fatalf("rollup after failed attempt = (%d,%d,%d,%d), want all zero: a rolled-back attempt must not partially apply",
			tokensIn, tokensCachedIn, tokensOut, costSubcents)
	}

	// Redelivery of the same usage_event_id, this time with nothing armed to
	// fail. Because nothing committed on the first attempt, the unique index
	// must not treat this as a duplicate.
	redelivered := newTestUsageEvent("evt-nontransient", "task-nontransient", "session-nontransient")
	if err := repo.CreateTaskUsageEvent(context.Background(), redelivered); err != nil {
		t.Fatalf("redelivered CreateTaskUsageEvent: %v", err)
	}

	if got := countTaskUsageEventRows(t, repo); got != 1 {
		t.Fatalf("row count after retry = %d, want 1", got)
	}
	tokensIn, tokensCachedIn, tokensOut, costSubcents = readTaskSessionRollup(t, repo, "session-nontransient")
	if tokensIn != 100 || tokensCachedIn != 25 || tokensOut != 30 || costSubcents != 42 {
		t.Errorf("rollup after retry = (%d,%d,%d,%d), want (100,25,30,42)", tokensIn, tokensCachedIn, tokensOut, costSubcents)
	}
}
