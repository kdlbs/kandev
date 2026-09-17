package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (r *Repository) Migrate() error {
	if err := r.migratePersonaStorage(); err != nil {
		return err
	}
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS orchestration_roles (id TEXT PRIMARY KEY, name TEXT NOT NULL, instructions TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS workspace_orchestrators (agent_id TEXT PRIMARY KEY REFERENCES agent_profiles(id) ON DELETE CASCADE, workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE, role_id TEXT NOT NULL REFERENCES orchestration_roles(id))`,
		`CREATE INDEX IF NOT EXISTS idx_workspace_orchestrators_workspace ON workspace_orchestrators(workspace_id)`,
		`INSERT INTO orchestration_roles (id,name,instructions) VALUES ('chief-of-staff','Chief of staff','Coordinate the workspace. Delegate substantive work to the agent assigned to each task. Keep updates concise, inspect evidence, and surface decisions that need human input.') ON CONFLICT(id) DO NOTHING`,
	} {
		if _, err := r.db.Exec(q); err != nil {
			return err
		}
	}
	if err := r.migrateAssistantStorage(); err != nil {
		return err
	}
	if err := r.migrateObjectives(); err != nil {
		return err
	}
	if err := r.ImportLegacyState(); err != nil {
		return err
	}
	if err := r.migrateConversationOwnership(); err != nil {
		return err
	}
	if err := r.migrateMemoryContext(); err != nil {
		return err
	}
	if err := r.migrateRoleConfiguration(); err != nil {
		return err
	}
	// Settings initializes profiles before Orchestration in the application; standalone
	// schema migrations may run before that store is present.
	profilesExist, err := db.TableExists(r.db, "agent_profiles")
	if err != nil {
		return err
	}
	if profilesExist {
		return r.createProfileDeletionTrigger()
	}
	return nil
}

func (r *Repository) ListOrchestratorRoles(ctx context.Context) ([]models.OrchestratorRole, error) {
	rows := []models.OrchestratorRole{}
	err := r.ro.SelectContext(ctx, &rows, `SELECT id,name,icon,instructions FROM orchestration_roles ORDER BY name,id`)
	return rows, err
}
func (r *Repository) GetOrchestratorRole(ctx context.Context, id string) (*models.OrchestratorRole, error) {
	var row models.OrchestratorRole
	err := r.ro.GetContext(ctx, &row, r.ro.Rebind(`SELECT id,name,icon,instructions FROM orchestration_roles WHERE id=?`), id)
	return &row, err
}
func (r *Repository) DeleteOrchestratorRole(ctx context.Context, id string) error {
	if id == "chief-of-staff" {
		return fmt.Errorf("the built-in role can be edited but not deleted")
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM orchestration_roles WHERE id=? AND NOT EXISTS (SELECT 1 FROM workspace_orchestrators WHERE role_id=?)`), id, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return fmt.Errorf("role is in use or unavailable")
	}
	return err
}
func (r *Repository) RegisterOrchestrator(ctx context.Context, agentID, workspaceID, roleID string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO workspace_orchestrators (agent_id,workspace_id,role_id) SELECT id,workspace_id,? FROM agent_profiles WHERE id=? AND workspace_id=? AND deleted_at IS NULL AND role='assistant' ON CONFLICT(agent_id) DO UPDATE SET role_id=excluded.role_id`), roleID, agentID, workspaceID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return fmt.Errorf("orchestrator must belong to this workspace")
	}
	return err
}
func (r *Repository) ListOrchestratorIDs(ctx context.Context, workspaceID string) ([]string, error) {
	ids := []string{}
	err := r.ro.SelectContext(ctx, &ids, r.ro.Rebind(`SELECT o.agent_id FROM workspace_orchestrators o JOIN agent_profiles a ON a.id=o.agent_id WHERE o.workspace_id=? AND a.deleted_at IS NULL ORDER BY a.name,o.agent_id`), workspaceID)
	return ids, err
}
func (r *Repository) OrchestratorRoleID(ctx context.Context, agentID string) (string, error) {
	var id string
	err := r.ro.GetContext(ctx, &id, r.ro.Rebind(`SELECT o.role_id FROM workspace_orchestrators o JOIN agent_profiles a ON a.id=o.agent_id WHERE o.agent_id=? AND a.deleted_at IS NULL`), agentID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}
func (r *Repository) UnregisterOrchestrator(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM workspace_orchestrators WHERE agent_id=?`), id)
	return err
}
