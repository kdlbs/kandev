package sqlite

import "github.com/kandev/kandev/internal/db/dialect"

func (r *Repository) migrateMaintenance() error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS orchestration_maintenance_grants (
 candidate_id TEXT PRIMARY KEY REFERENCES orchestration_improvements(id) ON DELETE CASCADE,
 binding_id TEXT NOT NULL REFERENCES orchestration_assistant_bindings(id) ON DELETE CASCADE,
 owner_user_id TEXT NOT NULL, workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
 binding_version INTEGER NOT NULL, authority_revision TEXT NOT NULL, profile_revision TEXT NOT NULL,
 revision INTEGER NOT NULL, scope_json TEXT NOT NULL, base_oid TEXT NOT NULL,
 expires_at TIMESTAMP NOT NULL, revoked_at TIMESTAMP, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_orchestration_maintenance_owner ON orchestration_maintenance_grants(binding_id,candidate_id)`,
		`CREATE TABLE IF NOT EXISTS orchestration_maintenance_validation (
 candidate_id TEXT PRIMARY KEY REFERENCES orchestration_improvements(id) ON DELETE CASCADE,
 validation_json TEXT NOT NULL, updated_at TIMESTAMP NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_orchestration_maintenance_task ON orchestration_improvements(repair_task_id) WHERE repair_task_id<>''`,
	} {
		if _, err := r.db.Exec(dialect.MustRenderSchema(r.db.DriverName(), statement)); err != nil {
			return err
		}
	}
	return nil
}
