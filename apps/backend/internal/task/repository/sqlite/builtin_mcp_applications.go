package sqlite

func (r *Repository) initBuiltinMCPApplicationsSchema() error {
	_, err := r.db.ExecContext(r.migrationContext(), `
 CREATE TABLE IF NOT EXISTS task_builtin_mcp_applications (
  session_id TEXT PRIMARY KEY REFERENCES task_sessions(id) ON DELETE CASCADE,
  task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  workspace_id TEXT NOT NULL,
  target_revision BIGINT NOT NULL,
  policy_revision BIGINT NOT NULL,
  selection_hash TEXT NOT NULL,
  requested_by TEXT NOT NULL,
  request_id TEXT NOT NULL,
  operation_id TEXT NOT NULL DEFAULT '',
  attempt_source TEXT NOT NULL DEFAULT '',
  attempt_actor_id TEXT NOT NULL DEFAULT '',
  attempt_selection_hash TEXT NOT NULL DEFAULT '',
  attempt_policy_revision BIGINT NOT NULL DEFAULT 0,
  target_binding_id TEXT NOT NULL DEFAULT '',
  target_binding_json TEXT NOT NULL DEFAULT '',
  verification_deadline TIMESTAMP,
  retry_of_operation_id TEXT NOT NULL DEFAULT '',

  attempt_generation BIGINT NOT NULL DEFAULT 0,
  attempt_revision BIGINT NOT NULL DEFAULT 0,
  source_execution_id TEXT NOT NULL DEFAULT '',
  replacement_execution_id TEXT NOT NULL DEFAULT '',
  stage TEXT NOT NULL DEFAULT 'pending',
  reason TEXT NOT NULL DEFAULT '',
  row_version BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL
 );
 CREATE TABLE IF NOT EXISTS agent_provider_mcp_bindings (
  binding_id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES task_sessions(id) ON DELETE CASCADE,
  task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  workspace_id TEXT NOT NULL,
  execution_id TEXT NOT NULL,
  operation_id TEXT NOT NULL,
  binding_json TEXT NOT NULL,
  catalog_json TEXT NOT NULL DEFAULT '{}',
  target_revision BIGINT NOT NULL,
  policy_revision BIGINT NOT NULL,
  native_session_id TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL,
  accepted_at TIMESTAMP
 )`)
	return err
}
