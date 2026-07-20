package db

import (
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// MigrateLogger wraps a DB connection with per-statement migration logging.
// It preserves the existing "swallow-error" contract of legacy `_, _ = db.Exec(...)`
// calls while adding observability: applied migrations log at INFO, idempotent
// no-ops are silent, and unexpected failures log at WARN.
type MigrateLogger struct {
	db  *sqlx.DB
	log *logger.Logger
}

// NewMigrateLogger creates a MigrateLogger for the given writer connection.
// log may be nil, in which case all output is suppressed (matches the existing
// no-op pattern used in tests).
func NewMigrateLogger(db *sqlx.DB, log *logger.Logger) *MigrateLogger {
	return &MigrateLogger{db: db, log: log}
}

// Apply executes stmt and classifies the result:
//   - success: logs "migration applied" at INFO, returns true
//   - "already exists" error: silent (idempotent re-run), returns false
//   - anything else: logs "migration failed" at WARN, returns false
//
// The error is never returned - this matches the contract of the legacy
// `_, _ = db.Exec(...)` pattern, with observability added. The returned bool
// lets callers gate one-time follow-up work (e.g. a backfill) so it only
// runs on the boot where the column/table was actually newly created,
// instead of re-running (and re-scanning the table) on every subsequent
// boot once the ALTER becomes a no-op.
func (m *MigrateLogger) Apply(name, stmt string) bool {
	if _, err := m.db.Exec(stmt); err != nil {
		if IsAlreadyExistsError(err) {
			return false
		}
		if m.log != nil {
			m.log.Warn("migration failed",
				zap.String("name", name), zap.Error(err))
		}
		return false
	}
	if m.log != nil {
		m.log.Info("migration applied", zap.String("name", name))
	}
	return true
}
