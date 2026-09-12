package db

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// MigrateLogger wraps a DB connection with per-statement migration logging.
// It preserves the existing "swallow-error" contract of legacy `_, _ = db.Exec(...)`
// calls while adding observability: applied migrations log at INFO, idempotent
// no-ops are silent, and unexpected failures log at WARN.
type MigrateLogger struct {
	db       *sqlx.DB
	log      *logger.Logger
	strict   bool
	ctx      context.Context
	firstErr error
}

// NewMigrateLogger creates a MigrateLogger for the given writer connection.
// log may be nil, in which case all output is suppressed (matches the existing
// no-op pattern used in tests).
func NewMigrateLogger(db *sqlx.DB, log *logger.Logger) *MigrateLogger {
	return newMigrateLogger(db, log, false)
}

// NewRequiredMigrateLogger creates a migration logger whose unexpected
// failures are fatal to the owning required store.
func NewRequiredMigrateLogger(db *sqlx.DB, log *logger.Logger) *MigrateLogger {
	return newMigrateLogger(db, log, true)
}

// NewRequiredMigrateLoggerContext creates a required migration logger whose
// statements observe ctx. The context is checked between migration steps and
// by the SQL driver while a statement is running. Once a required statement
// fails, later Apply calls become admission no-ops until the owning
// constructor returns the first error.
func NewRequiredMigrateLoggerContext(db *sqlx.DB, log *logger.Logger, ctx context.Context) *MigrateLogger {
	if ctx == nil {
		ctx = context.Background()
	}
	return newMigrateLoggerContext(db, log, true, ctx)
}

func newMigrateLogger(db *sqlx.DB, log *logger.Logger, strict bool) *MigrateLogger {
	return newMigrateLoggerContext(db, log, strict, context.Background())
}

func newMigrateLoggerContext(db *sqlx.DB, log *logger.Logger, strict bool, ctx context.Context) *MigrateLogger {
	return &MigrateLogger{db: db, log: log, strict: strict, ctx: ctx}
}

// Apply executes stmt and classifies the result:
//   - success: logs "migration applied" at INFO
//   - "already exists" error: silent (idempotent re-run)
//   - anything else: logs "migration failed" at WARN
//
// In strict mode, the first unexpected error is retained and returned. In
// compatibility mode, unexpected errors are logged and swallowed as before.
func (m *MigrateLogger) Apply(name, stmt string) error {
	return m.ApplyContext(m.ctx, name, stmt)
}

// ApplyContext executes a migration statement with an explicit context.
func (m *MigrateLogger) ApplyContext(ctx context.Context, name, stmt string) error {
	if m.strict && m.firstErr != nil {
		return m.firstErr
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return m.recordFailure(name, err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		if IsAlreadyExistsError(err) {
			return nil
		}
		return m.recordFailure(name, err)
	}
	if m.log != nil {
		m.log.Info("migration applied", zap.String("name", name))
	}
	return nil
}

func (m *MigrateLogger) recordFailure(name string, err error) error {
	wrapped := fmt.Errorf("migration %q failed: %w", name, err)
	if m.strict && m.firstErr == nil {
		m.firstErr = wrapped
	}
	if m.log != nil {
		m.log.Warn("migration failed",
			zap.String("name", name), zap.Error(wrapped))
	}
	if m.strict {
		return wrapped
	}
	return nil
}

// Context returns the context used by this migration logger. It is used by
// legacy migration helpers that need to switch a long-running query to
// ExecContext without changing their public constructor signatures.
func (m *MigrateLogger) Context() context.Context {
	if m.ctx == nil {
		return context.Background()
	}
	return m.ctx
}

// Err returns the first unexpected migration failure observed by this
// logger. Idempotent duplicate errors are not reported.
func (m *MigrateLogger) Err() error {
	return m.firstErr
}
