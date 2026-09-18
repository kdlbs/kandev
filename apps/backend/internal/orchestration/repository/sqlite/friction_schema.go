package sqlite

import "github.com/kandev/kandev/internal/db/dialect"

func (r *Repository) migrateFriction() error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS orchestration_friction (
 id TEXT PRIMARY KEY, binding_id TEXT NOT NULL REFERENCES orchestration_assistant_bindings(id) ON DELETE CASCADE,
 workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
 task_id TEXT NOT NULL, session_id TEXT NOT NULL, occurrence_id TEXT NOT NULL,
 profile_id TEXT NOT NULL, account_revision TEXT NOT NULL, origin TEXT NOT NULL,
 operation TEXT NOT NULL, reason TEXT NOT NULL, cause TEXT NOT NULL DEFAULT '', policy_version TEXT NOT NULL,
 fingerprint TEXT NOT NULL, outcome TEXT NOT NULL, observed_at TIMESTAMP NOT NULL,
 UNIQUE(binding_id,occurrence_id,outcome))`,
		`CREATE INDEX IF NOT EXISTS idx_orchestration_friction_scope ON orchestration_friction(binding_id,fingerprint,outcome,observed_at)`,
		`CREATE TABLE IF NOT EXISTS orchestration_improvements (
 id TEXT PRIMARY KEY, binding_id TEXT NOT NULL REFERENCES orchestration_assistant_bindings(id) ON DELETE CASCADE,
 workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
 fingerprint TEXT NOT NULL, profile_id TEXT NOT NULL, account_revision TEXT NOT NULL,
 origin TEXT NOT NULL, operation TEXT NOT NULL, reason TEXT NOT NULL, cause TEXT NOT NULL DEFAULT '', policy_version TEXT NOT NULL,
 state TEXT NOT NULL, revision INTEGER NOT NULL, incident_count INTEGER NOT NULL, task_count INTEGER NOT NULL,
 repair_task_id TEXT NOT NULL DEFAULT '', objective_id TEXT NOT NULL DEFAULT '', commit_oid TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL, prepared_at TIMESTAMP, resolved_at TIMESTAMP)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_orchestration_improvements_open ON orchestration_improvements(binding_id,fingerprint) WHERE state NOT IN ('resolved','rejected')`,
	} {
		if _, err := r.db.Exec(dialect.MustRenderSchema(r.db.DriverName(), statement)); err != nil {
			return err
		}
	}
	return nil
}
