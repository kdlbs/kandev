package sqlite

import "fmt"

// initCoordinatorAuthoritySchema creates the persisted, revocable grant and
// audit store for explicit task coordination. The tables are additive and use
// portable SQL shared by SQLite and Postgres.
func (r *Repository) initCoordinatorAuthoritySchema() error {
	_, err := r.db.Exec(`
		CREATE TABLE IF NOT EXISTS workspace_agent_principals (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
			plugin_installation_id TEXT NOT NULL CHECK(plugin_installation_id != ''),
			logical_key TEXT NOT NULL,
			backing_task_id TEXT NOT NULL DEFAULT '',
			backing_session_id TEXT NOT NULL DEFAULT '',
			revoked_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			UNIQUE(workspace_id, plugin_installation_id, logical_key)
		);
		CREATE UNIQUE INDEX IF NOT EXISTS uniq_active_workspace_agent_principal_task
			ON workspace_agent_principals(workspace_id, backing_task_id)
			WHERE revoked_at IS NULL AND backing_task_id != '';
		CREATE UNIQUE INDEX IF NOT EXISTS uniq_active_workspace_agent_principal_session
			ON workspace_agent_principals(workspace_id, backing_session_id)
			WHERE revoked_at IS NULL AND backing_session_id != '';

		CREATE TABLE IF NOT EXISTS task_coordinator_grants (
			id TEXT PRIMARY KEY,
			coordinator_task_id TEXT NOT NULL DEFAULT '',
			principal_id TEXT NOT NULL DEFAULT '',
			workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
			scope_kind TEXT NOT NULL,
			scope_id TEXT NOT NULL,
			capabilities TEXT NOT NULL,
			note TEXT NOT NULL DEFAULT '',
			granted_by_user_id TEXT NOT NULL DEFAULT '',
			granted_at TIMESTAMP NOT NULL,
			revoked_at TIMESTAMP,
			revoked_by_user_id TEXT NOT NULL DEFAULT ''
		);
		CREATE UNIQUE INDEX IF NOT EXISTS uniq_active_task_coordinator_grants_scope
			ON task_coordinator_grants(coordinator_task_id, scope_kind, scope_id)
			WHERE revoked_at IS NULL;
		CREATE UNIQUE INDEX IF NOT EXISTS uniq_active_principal_coordinator_grants_scope
			ON task_coordinator_grants(principal_id, scope_kind, scope_id)
			WHERE revoked_at IS NULL AND principal_id != '';
		CREATE INDEX IF NOT EXISTS idx_task_coordinator_grants_workspace
			ON task_coordinator_grants(workspace_id, coordinator_task_id);
		CREATE INDEX IF NOT EXISTS idx_task_coordinator_grants_principal
			ON task_coordinator_grants(workspace_id, principal_id);

		CREATE TABLE IF NOT EXISTS task_coordinator_audit_events (
			id TEXT PRIMARY KEY,
			occurred_at TIMESTAMP NOT NULL,
			principal_id TEXT NOT NULL DEFAULT '',
			actor_task_id TEXT NOT NULL,
			actor_session_id TEXT NOT NULL DEFAULT '',
			target_task_id TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			action TEXT NOT NULL,
			capability TEXT NOT NULL,
			decision TEXT NOT NULL,
			deny_reason TEXT NOT NULL DEFAULT '',
			grant_id TEXT NOT NULL DEFAULT '',
			result TEXT NOT NULL DEFAULT 'pending',
			detail TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_task_coordinator_audit_workspace_occurred
			ON task_coordinator_audit_events(workspace_id, occurred_at DESC);
		CREATE INDEX IF NOT EXISTS idx_task_coordinator_audit_actor_occurred
			ON task_coordinator_audit_events(actor_task_id, occurred_at DESC);
		CREATE INDEX IF NOT EXISTS idx_task_coordinator_audit_principal_occurred
			ON task_coordinator_audit_events(workspace_id, principal_id, occurred_at DESC);
	`)
	if err != nil {
		return fmt.Errorf("init coordinator authority schema: %w", err)
	}
	return nil
}
