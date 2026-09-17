package sqlite

import (
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

func (r *Repository) migrateAssistantStorage() error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS orchestration_assistant_bindings (
			id TEXT PRIMARY KEY, owner_user_id TEXT NOT NULL UNIQUE,
			orchestrator_id TEXT NOT NULL UNIQUE REFERENCES workspace_orchestrators(agent_id) ON DELETE CASCADE,
			workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
			conversation_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			version INTEGER NOT NULL, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS orchestration_conversation_intents (
			task_id TEXT PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE, revision INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS orchestration_intake (
			comment_id TEXT PRIMARY KEY REFERENCES task_comments(id) ON DELETE CASCADE,
			task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			agent_id TEXT NOT NULL REFERENCES workspace_orchestrators(agent_id) ON DELETE CASCADE,
			owner_user_id TEXT NOT NULL, client_message_id TEXT NOT NULL, payload_hash TEXT NOT NULL,
			sequence INTEGER NOT NULL, status TEXT NOT NULL DEFAULT 'accepted', run_id TEXT NOT NULL DEFAULT '',
			UNIQUE(task_id,owner_user_id,client_message_id), UNIQUE(task_id,sequence))`,
		`CREATE INDEX IF NOT EXISTS idx_orchestration_intake_pending ON orchestration_intake(status,task_id,sequence)`,
		`CREATE TABLE IF NOT EXISTS orchestration_operations (
			id TEXT PRIMARY KEY, binding_id TEXT NOT NULL REFERENCES orchestration_assistant_bindings(id) ON DELETE CASCADE,
			operation_id TEXT NOT NULL, conversation_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			run_id TEXT NOT NULL, target TEXT NOT NULL, request_hash TEXT NOT NULL,
			intent_revision INTEGER NOT NULL, binding_version INTEGER NOT NULL, state TEXT NOT NULL,
			response_json TEXT NOT NULL DEFAULT '{}', http_status INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL,updated_at TIMESTAMP NOT NULL,UNIQUE(binding_id,operation_id))`,
	} {
		if _, err := r.db.Exec(dialect.MustRenderSchema(r.db.DriverName(), q)); err != nil {
			return err
		}
	}
	exists, err := db.ColumnExists(r.db, "orchestration_assistant_bindings", "execution_mode")
	if err != nil || exists {
		return err
	}
	_, err = r.db.Exec(`ALTER TABLE orchestration_assistant_bindings ADD COLUMN execution_mode TEXT NOT NULL DEFAULT 'inspect'`)
	return err
}
