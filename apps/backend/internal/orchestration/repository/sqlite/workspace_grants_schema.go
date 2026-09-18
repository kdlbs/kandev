package sqlite

import "github.com/kandev/kandev/internal/db/dialect"

func (r *Repository) migrateWorkspaceGrants() error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS orchestration_workspace_grants (
 id TEXT PRIMARY KEY, binding_id TEXT NOT NULL REFERENCES orchestration_assistant_bindings(id) ON DELETE CASCADE,
 owner_user_id TEXT NOT NULL, workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
 binding_version INTEGER NOT NULL, revision INTEGER NOT NULL,
 receiver_profile_id TEXT NOT NULL, receiver_profile_revision TEXT NOT NULL, authority_revision TEXT NOT NULL,
 scope_json TEXT NOT NULL, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL, revoked_at TIMESTAMP,
 UNIQUE(binding_id,workspace_id))`,
		`CREATE TABLE IF NOT EXISTS orchestration_workspace_grant_events (
 id TEXT PRIMARY KEY, grant_id TEXT NOT NULL REFERENCES orchestration_workspace_grants(id) ON DELETE CASCADE,
 revision INTEGER NOT NULL, action TEXT NOT NULL, snapshot_json TEXT NOT NULL, created_at TIMESTAMP NOT NULL,
 UNIQUE(grant_id,revision))`,
		`CREATE INDEX IF NOT EXISTS idx_orchestration_workspace_grant_events_page ON orchestration_workspace_grant_events(grant_id,id)`,
		`CREATE TABLE IF NOT EXISTS orchestration_workspace_exports (
 id TEXT PRIMARY KEY, binding_id TEXT NOT NULL REFERENCES orchestration_assistant_bindings(id) ON DELETE CASCADE,
 owner_user_id TEXT NOT NULL, conversation_id TEXT NOT NULL, workspace_id TEXT NOT NULL,
 grant_id TEXT NOT NULL, grant_revision INTEGER NOT NULL, receiver_profile_id TEXT NOT NULL,
 receiver_profile_revision TEXT NOT NULL, authority_revision TEXT NOT NULL, kind TEXT NOT NULL,
 created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL,
 UNIQUE(binding_id,conversation_id,workspace_id,receiver_profile_id,receiver_profile_revision,authority_revision,kind))`,
		`CREATE INDEX IF NOT EXISTS idx_orchestration_workspace_exports_page ON orchestration_workspace_exports(binding_id,conversation_id,id)`,
	} {
		if _, err := r.db.Exec(dialect.MustRenderSchema(r.db.DriverName(), statement)); err != nil {
			return err
		}
	}
	return nil
}
