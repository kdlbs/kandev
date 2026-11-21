package repository

import "fmt"

func (r *Repository) initPromptSourceSchema() error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS prompt_source_templates (
   workspace_id TEXT NOT NULL, id TEXT NOT NULL, revision INTEGER NOT NULL CHECK (revision > 0), definition TEXT NOT NULL,
   created_at TIMESTAMP NOT NULL, PRIMARY KEY (workspace_id, id, revision), FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS prompt_source_instances (
   id TEXT PRIMARY KEY, template_id TEXT NOT NULL, workspace_id TEXT NOT NULL,
   workflow_id TEXT NOT NULL UNIQUE, applied_revision INTEGER NOT NULL,
   source_locked INTEGER NOT NULL CHECK (source_locked IN (0,1)), variables TEXT NOT NULL,
   updated_at TIMESTAMP NOT NULL, UNIQUE (workspace_id, template_id),
   FOREIGN KEY (workspace_id, template_id, applied_revision) REFERENCES prompt_source_templates(workspace_id, id, revision),
   FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
   FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE)`} {
		if _, err := r.db.Exec(statement); err != nil {
			return fmt.Errorf("initialize prompt sources: %w", err)
		}
	}
	return nil
}
