package sqlite

import "github.com/kandev/kandev/internal/db/dialect"

// Orchestration owns persona state; only task comments use canonical task storage.
func (r *Repository) migratePersonaStorage() error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS orchestration_legacy_imports(agent_profile_id TEXT PRIMARY KEY)`,
		`CREATE TABLE IF NOT EXISTS orchestration_memory(id TEXT PRIMARY KEY,agent_profile_id TEXT NOT NULL,layer TEXT NOT NULL,key TEXT NOT NULL,content TEXT DEFAULT '',metadata TEXT DEFAULT '{}',created_at TIMESTAMP NOT NULL,updated_at TIMESTAMP NOT NULL,UNIQUE(agent_profile_id,layer,key))`,

		`CREATE TABLE IF NOT EXISTS orchestration_instructions(id TEXT PRIMARY KEY,agent_profile_id TEXT NOT NULL,filename TEXT NOT NULL,content TEXT NOT NULL DEFAULT '',is_entry INTEGER DEFAULT 0,created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,UNIQUE(agent_profile_id,filename))`,
		`CREATE TABLE IF NOT EXISTS orchestration_conversations(id TEXT PRIMARY KEY,workspace_id TEXT NOT NULL,agent_profile_id TEXT NOT NULL,platform TEXT NOT NULL,config TEXT NOT NULL DEFAULT '{}',webhook_secret TEXT NOT NULL DEFAULT '',status TEXT NOT NULL DEFAULT 'active',task_id TEXT NOT NULL DEFAULT '',created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`,
	} {
		if _, err := r.db.Exec(dialect.MustRenderSchema(r.db.DriverName(), q)); err != nil {
			return err
		}
	}
	return nil
}
