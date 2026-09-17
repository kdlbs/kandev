package sqlite

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/orchestration/models"
	"strings"
)

func (r *Repository) SaveOrchestratorRole(ctx context.Context, role *models.OrchestratorRole) error {
	role.Name = strings.TrimSpace(role.Name)
	if role.ID == "" || role.Name == "" || len(role.Name) > 100 || len(role.Instructions) > 32000 || !models.ValidRoleIcon(role.Icon) {
		return fmt.Errorf("role requires a name up to 100 bytes, instructions up to 32000 bytes and an available icon")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO orchestration_roles(id,name,icon,instructions) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,icon=excluded.icon,instructions=excluded.instructions`), role.ID, role.Name, role.Icon, role.Instructions); err != nil {
		return err
	}
	// Update the core identity cache without touching status, account or execution settings.
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE agent_profiles SET name=? WHERE id IN (SELECT agent_id FROM workspace_orchestrators WHERE role_id=?)`), role.Name, role.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) AssignedRole(ctx context.Context, agentID string) (*models.OrchestratorRole, error) {
	id, err := r.OrchestratorRoleID(ctx, agentID)
	if err != nil {
		return nil, err
	}
	return r.GetOrchestratorRole(ctx, id)
}
