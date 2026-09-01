// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
)

// Repository provides SQLite-based task storage operations.
type Repository struct {
	db               *sqlx.DB // writer
	ro               *sqlx.DB // reader (read-only pool)
	ownsDB           bool
	log              *logger.Logger
	migrate          *db.MigrateLogger
	queuePurgeMu     sync.RWMutex
	queuePurger      func(context.Context, string)
	queuePurgeNotify func(context.Context, string)
	// clockNow is a test-only clock seam. Set it before any concurrent
	// repository call; it carries no synchronization.
	clockNow func() time.Time
	// failCutoverAfter is a test-only failpoint for the worktree ownership
	// cutover: when set to a cutover step name, the migration aborts at that
	// step so tests can prove rollback restores the pre-upgrade state.
	failCutoverAfter string
	// failGitSnapshotCutoverAfter is a test-only failpoint for the Git snapshot
	// ownership cutover. It is separate from the worktree failpoint because the
	// two migrations can be exercised independently in the same repository.
	failGitSnapshotCutoverAfter string
	// failUsageEventAttempts/failUsageEventErr are a test-only failpoint for
	// CreateTaskUsageEvent's AC-32 transient-retry loop: while
	// failUsageEventAttempts > 0, insertUsageEventAndRollup returns
	// failUsageEventErr before opening a transaction, without touching the
	// database, and decrements the counter. A genuine SQLITE_BUSY/serialization
	// race cannot be triggered deterministically against this package's
	// single-connection test repositories, so this stands in for one. Use this
	// (not the mid-transaction seam below) when a scenario needs the injected
	// failure to happen before any real driver-level activity - e.g. before a
	// same-attempt foreign-key violation the test also wants to observe.
	failUsageEventAttempts int
	failUsageEventErr      error
	// failUsageEventRollupAttempts/failUsageEventRollupErr are a test-only
	// failpoint inside insertUsageEventAndRollup's transaction, checked after
	// the ledger row insert succeeds but before the task_sessions rollup
	// increment: while failUsageEventRollupAttempts > 0, the function returns
	// failUsageEventRollupErr and decrements the counter, so the deferred
	// tx.Rollback() unwinds a real, already-attempted INSERT. Unlike
	// failUsageEventAttempts (which fires before BeginTxx and so cannot tell
	// an atomic single-transaction implementation from a hypothetical
	// two-transaction one, since neither has touched the database yet), this
	// seam proves AC-11's atomicity: an implementation that committed the
	// insert in its own transaction before attempting the rollup would leave a
	// row behind here, while the real single-transaction implementation rolls
	// it back with everything else.
	// failParticipantSeatReconcileAttempts is a test-only failpoint for the
	// participant-seat reconciler's AC-OFFICE-REVIEW-SEATS-005.9 bounded
	// retry loop: while > 0, tryHealParticipantSeatRow reports a synthetic
	// concurrent-modification retry without touching the database, and
	// decrements the counter. A genuine SQLite write race is not
	// reproducible deterministically against this package's
	// single-connection test repositories (SetMaxOpenConns(1) serializes
	// all writes), so this stands in for one.
	failParticipantSeatReconcileAttempts int
	failUsageEventRollupAttempts         int
	failUsageEventRollupErr              error
	// usageEventPreRollupHook is a test-only synchronization seam, called (if
	// set) inside insertUsageEventAndRollup's transaction at the same point as
	// the failUsageEventRollup* failpoint - after the ledger row insert
	// succeeds, before the task_sessions rollup increment. Unlike the
	// failpoints, it does not alter control flow; it exists so a Postgres test
	// can pause a real production transaction at this exact boundary and open
	// a second, genuinely concurrent connection to construct a real lock
	// conflict there (a real SQLITE_BUSY/serialization race cannot be
	// provoked deterministically against SQLite's single test connection, but
	// Postgres supports true concurrent connections, so this proves AC-32's
	// retry classification against a real driver-reported 40001/40P01 rather
	// than only the injected failpoint errors). Nil in production and in
	// every test but the one that sets it.
	usageEventPreRollupHook func()
	// stepEntryDispatcher fires a step's session-independent on_enter
	// sequence after a registered step-transition writer commits. Nil-safe
	// (see dispatchStepEntry in step_entry_dispatch.go): unset in every
	// test that doesn't exercise it, and in production until
	// SetStepEntryDispatcher is called during boot wiring.
	stepEntryDispatcher StepEntryDispatcher
}

func (r *Repository) nowUTC() time.Time {
	if r.clockNow != nil {
		return r.clockNow().UTC()
	}
	return time.Now().UTC()
}

// SetTaskQueuePurger registers the orchestrator-owned queue cleanup for
// in-memory queues. SQLite queues are also purged in the task mutation
// transaction; this callback keeps the explicitly ephemeral queue equivalent.
func (r *Repository) SetTaskQueuePurger(purger func(context.Context, string)) {
	r.queuePurgeMu.Lock()
	defer r.queuePurgeMu.Unlock()
	r.queuePurger = purger
}

// SetTaskQueuePurgeNotifier registers a post-commit observer for every path
// that purges a task's queued_messages (archive/delete/workspace cascade).
// Callers must not purge the production SQLite queue again — that already
// happened in-transaction. The intended use is publishing
// message.queue.status_changed so the status-summary projector zeros
// queued_prompt_count on live clients.
func (r *Repository) SetTaskQueuePurgeNotifier(notifier func(context.Context, string)) {
	r.queuePurgeMu.Lock()
	defer r.queuePurgeMu.Unlock()
	r.queuePurgeNotify = notifier
}

func (r *Repository) notifyTaskQueuePurged(ctx context.Context, taskID string) {
	r.queuePurgeMu.RLock()
	purger := r.queuePurger
	notifier := r.queuePurgeNotify
	r.queuePurgeMu.RUnlock()
	if purger != nil {
		purger(ctx, taskID)
	}
	if notifier != nil {
		notifier(ctx, taskID)
	}
}

// NewWithDB creates a new SQLite repository with an existing database connection (shared ownership).
func NewWithDB(writer, reader *sqlx.DB, log *logger.Logger) (*Repository, error) {
	return newRepository(writer, reader, log, false)
}

func newRepository(writer, reader *sqlx.DB, log *logger.Logger, ownsDB bool) (*Repository, error) {
	repo := &Repository{
		db:      writer,
		ro:      reader,
		ownsDB:  ownsDB,
		log:     log,
		migrate: db.NewMigrateLogger(writer, log),
	}
	if err := repo.initSchema(); err != nil {
		if ownsDB {
			if closeErr := writer.Close(); closeErr != nil {
				return nil, fmt.Errorf("failed to close database after schema error: %w", closeErr)
			}
		}
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}
	return repo, nil
}

// Close closes the database connection
func (r *Repository) Close() error {
	if !r.ownsDB {
		return nil
	}
	return r.db.Close()
}

// DB returns the underlying sql.DB instance for shared access
func (r *Repository) DB() *sql.DB {
	return r.db.DB
}
