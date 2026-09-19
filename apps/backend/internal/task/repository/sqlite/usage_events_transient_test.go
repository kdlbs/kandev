package sqlite

// Transient-error retry sequencing for CreateTaskUsageEvent (AC-32(c)): a
// transient failure is retried in a fresh transaction, waiting per
// usageEventTransientBackoffs, up to maxUsageEventTransientAttempts total
// attempts. A genuine SQLITE_BUSY/serialization race cannot be triggered
// deterministically against this package's single-connection test
// repositories, so these tests drive the retry loop through the failpoint
// seams in base.go instead. They run inside synctest.Test so
// usageEventTransientBackoffs' real time.Timer waits (sleepOrDone) advance on
// the fake clock instead of costing real wall time.
//
// TestCreateTaskUsageEvent_TransientError_RetriesAndSucceeds and
// TestCreateTaskUsageEvent_TransientErrorExhaustsRetries_ReturnsError use
// failUsageEventRollupAttempts/failUsageEventRollupErr, which fires after
// insertUsageEventRowTx has actually attempted the row insert on each
// retried attempt - proving the retry loop rolls back and re-attempts a real
// insert, not just a pre-transaction error return.
// TestCreateTaskUsageEvent_TransientErrorDoesNotConsumeForeignKeyRetryBudget
// keeps the older failUsageEventAttempts/failUsageEventErr seam (fires
// before BeginTxx) because its scenario depends on the injected transient
// failure preceding a real foreign-key violation on the same attempt; the
// FK-violating session ("session-does-not-exist") is never created, so a
// post-insert failpoint would never fire on that attempt at all.

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mattn/go-sqlite3"
)

func transientSQLiteBusyErr() error {
	return sqlite3.Error{Code: sqlite3.ErrBusy}
}

// TestCreateTaskUsageEvent_TransientError_RetriesAndSucceeds pins that a
// single transient failure is retried (not surfaced as an error), and that
// the retry actually inserts the row.
func TestCreateTaskUsageEvent_TransientError_RetriesAndSucceeds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repo := newUsageEventsTestRepo(t)
		createUsageEventsTestTask(t, repo, "task-transient-retry")
		createUsageEventsTestSession(t, repo, "session-transient-retry", "task-transient-retry")

		repo.failUsageEventRollupAttempts = 1
		repo.failUsageEventRollupErr = transientSQLiteBusyErr()

		event := newTestUsageEvent("evt-transient-retry", "task-transient-retry", "session-transient-retry")
		start := time.Now()
		if err := repo.CreateTaskUsageEvent(context.Background(), event); err != nil {
			t.Fatalf("CreateTaskUsageEvent: %v", err)
		}
		elapsed := time.Since(start)

		if elapsed < usageEventTransientBackoffs[0] {
			t.Errorf("elapsed = %v, want at least the first backoff (%v)", elapsed, usageEventTransientBackoffs[0])
		}
		if got := countTaskUsageEventRows(t, repo); got != 1 {
			t.Fatalf("row count = %d, want 1", got)
		}
	})
}

// TestCreateTaskUsageEvent_TransientErrorExhaustsRetries_ReturnsError pins
// AC-32(c)'s cap: maxUsageEventTransientAttempts total attempts, after which
// the last transient error is returned as-is and no row is inserted.
func TestCreateTaskUsageEvent_TransientErrorExhaustsRetries_ReturnsError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repo := newUsageEventsTestRepo(t)
		createUsageEventsTestTask(t, repo, "task-transient-exhaust")
		createUsageEventsTestSession(t, repo, "session-transient-exhaust", "task-transient-exhaust")

		repo.failUsageEventRollupAttempts = maxUsageEventTransientAttempts
		repo.failUsageEventRollupErr = transientSQLiteBusyErr()

		event := newTestUsageEvent("evt-transient-exhaust", "task-transient-exhaust", "session-transient-exhaust")
		start := time.Now()
		err := repo.CreateTaskUsageEvent(context.Background(), event)
		elapsed := time.Since(start)

		if err == nil {
			t.Fatal("expected an error after exhausting all transient-retry attempts, got nil")
		}
		if !errors.Is(err, repo.failUsageEventRollupErr) && err.Error() != transientSQLiteBusyErr().Error() {
			t.Errorf("returned error = %v, want the injected transient error surfaced as-is", err)
		}

		var wantMinWait time.Duration
		for _, backoff := range usageEventTransientBackoffs {
			wantMinWait += backoff
		}
		if elapsed < wantMinWait {
			t.Errorf("elapsed = %v, want at least the sum of all backoffs (%v)", elapsed, wantMinWait)
		}
		if got := countTaskUsageEventRows(t, repo); got != 0 {
			t.Errorf("row count = %d, want 0 (exhausted retries must not leave a row)", got)
		}
	})
}

// TestCreateTaskUsageEvent_TransientErrorDoesNotConsumeForeignKeyRetryBudget
// pins that the transient-retry budget and the one-shot foreign-key retry
// are independent: a transient failure followed by a real foreign-key
// violation still gets its single FK retry.
func TestCreateTaskUsageEvent_TransientErrorDoesNotConsumeForeignKeyRetryBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repo := newUsageEventsTestRepo(t)
		createUsageEventsTestTask(t, repo, "task-transient-then-fk")

		repo.failUsageEventAttempts = 1
		repo.failUsageEventErr = transientSQLiteBusyErr()

		event := newTestUsageEvent("evt-transient-then-fk", "task-transient-then-fk", "session-does-not-exist")
		if err := repo.CreateTaskUsageEvent(context.Background(), event); err != nil {
			t.Fatalf("CreateTaskUsageEvent: %v", err)
		}

		if got := countTaskUsageEventRows(t, repo); got != 1 {
			t.Fatalf("row count = %d, want 1", got)
		}
		var sessionID *string
		if err := repo.db.Get(&sessionID, `SELECT session_id FROM task_usage_events WHERE usage_event_id = ?`, "evt-transient-then-fk"); err != nil {
			t.Fatalf("read row: %v", err)
		}
		if sessionID != nil {
			t.Errorf("session_id = %v, want nil (the FK retry must still be available after a transient retry)", *sessionID)
		}
	})
}
