package sqlite

func (r *Repository) initBuiltinMCPPreferencesSchema() error {
	_, err := r.db.ExecContext(r.migrationContext(), `
		CREATE TABLE IF NOT EXISTS task_builtin_mcp_preferences (
			task_id TEXT PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
			workspace_id TEXT NOT NULL,
			schema_version INTEGER NOT NULL,
			revision BIGINT NOT NULL CHECK (revision > 0),
			packages_json TEXT NOT NULL,
			updated_by TEXT NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`)
	return err
}
