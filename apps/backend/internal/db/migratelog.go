package db

import (
	"strings"

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
//   - success: logs "migration applied" at INFO
//   - "already exists" error: silent (idempotent re-run)
//   - anything else: logs "migration failed" at WARN
//
// The error is never returned - this matches the contract of the legacy
// `_, _ = db.Exec(...)` pattern, with observability added.
func (m *MigrateLogger) Apply(name, stmt string) {
	if _, err := m.db.Exec(stmt); err != nil {
		if isAlreadyExists(err) {
			return
		}
		if m.log != nil {
			m.log.Warn("migration failed",
				zap.String("name", name), zap.Error(err))
		}
		return
	}
	if m.log != nil {
		m.log.Info("migration applied", zap.String("name", name))
	}
}

// isAlreadyExists returns true when err describes a condition that means
// the migration has already been applied. SQLite uses these error strings
// for idempotent schema operations.
func isAlreadyExists(err error) bool {
	s := err.Error()
	return strings.Contains(s, "duplicate column name") ||
		strings.Contains(s, "already exists")
}
