package db

import (
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

func newMigrateLogger(db *sqlx.DB, log *logger.Logger, strict bool) *MigrateLogger {
	return &MigrateLogger{db: db, log: log, strict: strict}
}

// Apply executes stmt and classifies the result:
//   - success: logs "migration applied" at INFO
//   - "already exists" error: silent (idempotent re-run)
//   - anything else: logs "migration failed" at WARN
//
// In strict mode, the first unexpected error is retained and returned. In
// compatibility mode, unexpected errors are logged and swallowed as before.
func (m *MigrateLogger) Apply(name, stmt string) error {
	if _, err := m.db.Exec(stmt); err != nil {
		if IsAlreadyExistsError(err) {
			return nil
		}
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
	if m.log != nil {
		m.log.Info("migration applied", zap.String("name", name))
	}
	return nil
}

// Err returns the first unexpected migration failure observed by this
// logger. Idempotent duplicate errors are not reported.
func (m *MigrateLogger) Err() error {
	return m.firstErr
}
