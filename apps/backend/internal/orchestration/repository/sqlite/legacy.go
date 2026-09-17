package sqlite

import (
	"fmt"
	"github.com/kandev/kandev/internal/db"
)

// ImportLegacyState transfers registered personas once. A later Office write or
// a deleted Orchestration instruction must never reappear after a restart.
func (r *Repository) ImportLegacyState() error {
	tx, err := r.db.Beginx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	imported := false
	for _, pair := range [][2]string{{"office_agent_instructions", "orchestration_instructions"}, {"office_agent_memory", "orchestration_memory"}, {"office_channels", "orchestration_conversations"}} {
		exists, err := db.TableExists(tx, pair[0])
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		imported = true
		query := fmt.Sprintf(`INSERT INTO %s SELECT old.* FROM %s old
   JOIN workspace_orchestrators o ON o.agent_id=old.agent_profile_id
   WHERE NOT EXISTS (SELECT 1 FROM orchestration_legacy_imports i WHERE i.agent_profile_id=o.agent_id)
   ON CONFLICT DO NOTHING`, pair[1], pair[0])
		if pair[1] == "orchestration_memory" {
			query = `INSERT INTO orchestration_memory(id,agent_profile_id,layer,key,content,metadata,created_at,updated_at)
			SELECT old.id,old.agent_profile_id,old.layer,old.key,old.content,old.metadata,old.created_at,old.updated_at FROM office_agent_memory old
			JOIN workspace_orchestrators o ON o.agent_id=old.agent_profile_id
			WHERE NOT EXISTS (SELECT 1 FROM orchestration_legacy_imports i WHERE i.agent_profile_id=o.agent_id) ON CONFLICT DO NOTHING`
		}
		if pair[1] == "orchestration_conversations" {
			query = `INSERT INTO orchestration_conversations(id,workspace_id,agent_profile_id,platform,config,webhook_secret,status,task_id,created_at,updated_at)
			SELECT old.id,old.workspace_id,old.agent_profile_id,old.platform,old.config,old.webhook_secret,old.status,old.task_id,old.created_at,old.updated_at FROM office_channels old
			JOIN workspace_orchestrators o ON o.agent_id=old.agent_profile_id
			WHERE NOT EXISTS (SELECT 1 FROM orchestration_legacy_imports i WHERE i.agent_profile_id=o.agent_id) ON CONFLICT DO NOTHING`
		}
		if _, err := tx.Exec(query); err != nil {
			return fmt.Errorf("import legacy persona state: %w", err)
		}
	}
	if imported {
		if _, err := tx.Exec(`INSERT INTO orchestration_legacy_imports SELECT agent_id FROM workspace_orchestrators WHERE 1=1 ON CONFLICT DO NOTHING`); err != nil {
			return err
		}
	}
	return tx.Commit()
}
