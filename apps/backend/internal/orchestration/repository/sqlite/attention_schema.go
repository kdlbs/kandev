package sqlite

import "github.com/kandev/kandev/internal/db/dialect"

func (r *Repository) migrateAttention() error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS orchestration_attention (
 id TEXT PRIMARY KEY, binding_id TEXT NOT NULL REFERENCES orchestration_assistant_bindings(id) ON DELETE CASCADE,
 workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
 task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
 session_id TEXT NOT NULL,source_id TEXT NOT NULL,kind TEXT NOT NULL,state TEXT NOT NULL,
 source_revision TEXT NOT NULL,summary TEXT NOT NULL,revision INTEGER NOT NULL,
 last_notified_revision INTEGER NOT NULL DEFAULT 0, updated_at TIMESTAMP NOT NULL,
 UNIQUE(binding_id,task_id,session_id,source_id,kind))`,
		`CREATE INDEX IF NOT EXISTS idx_orchestration_attention_binding ON orchestration_attention(binding_id,id)`,
		`CREATE INDEX IF NOT EXISTS idx_orchestration_attention_task ON orchestration_attention(binding_id,task_id)`,
		`CREATE TABLE IF NOT EXISTS orchestration_attention_wakes (
 id TEXT PRIMARY KEY, attention_id TEXT NOT NULL REFERENCES orchestration_attention(id) ON DELETE CASCADE,
 source_revision TEXT NOT NULL,revision INTEGER NOT NULL,state TEXT NOT NULL,
 UNIQUE(attention_id,source_revision))`,
	} {
		if _, err := r.db.Exec(dialect.MustRenderSchema(r.db.DriverName(), q)); err != nil {
			return err
		}
	}
	return nil
}
