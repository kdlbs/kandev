package store

import (
	"fmt"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

// initSchema creates the auth tables. The DDL is shared between SQLite and
// Postgres with per-driver timestamp-type substitution (same pattern as
// internal/runtimeflags). All statements are replay-safe (ADR 0027).
func (s *Store) initSchema() error {
	timestampType := "DATETIME"
	if dialect.IsPostgres(s.db.DriverName()) {
		timestampType = "TIMESTAMPTZ"
	}
	statements := []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS auth_identities (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			provider TEXT NOT NULL DEFAULT 'local',
			subject TEXT NOT NULL DEFAULT '',
			password_hash TEXT NOT NULL DEFAULT '',
			created_at %s NOT NULL,
			updated_at %s NOT NULL,
			UNIQUE (user_id, provider),
			UNIQUE (provider, subject)
		)`, timestampType, timestampType),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS auth_sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			token_sha256 TEXT NOT NULL UNIQUE,
			created_at %s NOT NULL,
			expires_at %s NOT NULL,
			last_seen_at %s NOT NULL,
			user_agent TEXT NOT NULL DEFAULT '',
			ip TEXT NOT NULL DEFAULT ''
		)`, timestampType, timestampType, timestampType),
		`CREATE INDEX IF NOT EXISTS idx_auth_sessions_user ON auth_sessions(user_id)`,
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS auth_api_tokens (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			name TEXT NOT NULL,
			prefix TEXT NOT NULL,
			token_sha256 TEXT NOT NULL UNIQUE,
			created_at %s NOT NULL,
			last_used_at %s,
			expires_at %s
		)`, timestampType, timestampType, timestampType),
		`CREATE INDEX IF NOT EXISTS idx_auth_api_tokens_user ON auth_api_tokens(user_id)`,
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS auth_invites (
			id TEXT PRIMARY KEY,
			token_sha256 TEXT NOT NULL UNIQUE,
			email TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT 'member',
			created_by TEXT NOT NULL,
			created_at %s NOT NULL,
			expires_at %s NOT NULL,
			used_by TEXT,
			used_at %s
		)`, timestampType, timestampType, timestampType),
	}
	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("auth schema: %w", err)
		}
	}
	// An invite mints a member of exactly one organization, so it carries the
	// minting admin's org. CREATE TABLE IF NOT EXISTS is a no-op on an
	// existing database, so the column also needs an ADD COLUMN (ADR 0027).
	db.NewMigrateLogger(s.db, nil).Apply(
		"auth_invites.org_id",
		`ALTER TABLE auth_invites ADD COLUMN org_id TEXT NOT NULL DEFAULT ''`,
	)
	return nil
}
