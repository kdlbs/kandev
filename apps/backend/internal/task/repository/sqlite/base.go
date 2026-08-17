// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

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
	// failCutoverAfter is a test-only failpoint for the worktree ownership
	// cutover: when set to a cutover step name, the migration aborts at that
	// step so tests can prove rollback restores the pre-upgrade state.
	failCutoverAfter string
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
