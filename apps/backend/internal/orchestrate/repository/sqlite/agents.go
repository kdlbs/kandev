package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// CreateAgentInstance creates a new agent instance.
func (r *Repository) CreateAgentInstance(ctx context.Context, agent *models.AgentInstance) error {
	if agent.ID == "" {
		agent.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	agent.CreatedAt = now
	agent.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO orchestrate_agent_instances (
			id, workspace_id, name, agent_profile_id, role, icon, status,
			reports_to, permissions, budget_monthly_cents, max_concurrent_sessions,
			cooldown_sec, last_wakeup_finished_at,
			desired_skills, executor_preference, pause_reason, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), agent.ID, agent.WorkspaceID, agent.Name, agent.AgentProfileID,
		agent.Role, agent.Icon, agent.Status, agent.ReportsTo, agent.Permissions,
		agent.BudgetMonthlyCents, agent.MaxConcurrentSessions,
		agent.CooldownSec, agent.LastWakeupFinishedAt,
		agent.DesiredSkills,
		agent.ExecutorPreference, agent.PauseReason, agent.CreatedAt, agent.UpdatedAt)
	return err
}

// GetAgentInstance returns an agent instance by ID.
func (r *Repository) GetAgentInstance(ctx context.Context, id string) (*models.AgentInstance, error) {
	var agent models.AgentInstance
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT * FROM orchestrate_agent_instances WHERE id = ?`), id).StructScan(&agent)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent instance not found: %s", id)
	}
	return &agent, err
}

// GetAgentInstanceByName returns an agent instance by workspace+name.
func (r *Repository) GetAgentInstanceByName(
	ctx context.Context, workspaceID, name string,
) (*models.AgentInstance, error) {
	var agent models.AgentInstance
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT * FROM orchestrate_agent_instances WHERE workspace_id = ? AND name = ?`),
		workspaceID, name).StructScan(&agent)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent instance not found: %s", name)
	}
	return &agent, err
}

// ListAgentInstances returns all agent instances for a workspace.
// An empty workspaceID returns rows from all workspaces.
func (r *Repository) ListAgentInstances(ctx context.Context, workspaceID string) ([]*models.AgentInstance, error) {
	var (
		agents []*models.AgentInstance
		err    error
	)
	if workspaceID == "" {
		err = r.ro.SelectContext(ctx, &agents,
			`SELECT * FROM orchestrate_agent_instances ORDER BY created_at`)
	} else {
		err = r.ro.SelectContext(ctx, &agents, r.ro.Rebind(
			`SELECT * FROM orchestrate_agent_instances WHERE workspace_id = ? ORDER BY created_at`), workspaceID)
	}
	if err != nil {
		return nil, err
	}
	if agents == nil {
		agents = []*models.AgentInstance{}
	}
	return agents, nil
}

// UpdateAgentInstance updates an existing agent instance.
func (r *Repository) UpdateAgentInstance(ctx context.Context, agent *models.AgentInstance) error {
	agent.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE orchestrate_agent_instances SET
			name = ?, agent_profile_id = ?, role = ?, icon = ?, status = ?,
			reports_to = ?, permissions = ?, budget_monthly_cents = ?,
			max_concurrent_sessions = ?, cooldown_sec = ?, last_wakeup_finished_at = ?,
			desired_skills = ?, executor_preference = ?,
			pause_reason = ?, updated_at = ?
		WHERE id = ?
	`), agent.Name, agent.AgentProfileID, agent.Role, agent.Icon, agent.Status,
		agent.ReportsTo, agent.Permissions, agent.BudgetMonthlyCents,
		agent.MaxConcurrentSessions, agent.CooldownSec, agent.LastWakeupFinishedAt,
		agent.DesiredSkills, agent.ExecutorPreference,
		agent.PauseReason, agent.UpdatedAt, agent.ID)
	return err
}

// UpdateAgentStatusFields persists status + pause_reason for an agent.
func (r *Repository) UpdateAgentStatusFields(
	ctx context.Context, id, status, pauseReason string,
) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE orchestrate_agent_instances
		SET status = ?, pause_reason = ?, updated_at = ?
		WHERE id = ?
	`), status, pauseReason, now, id)
	return err
}

// DeleteAgentInstance deletes an agent instance by ID.
func (r *Repository) DeleteAgentInstance(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM orchestrate_agent_instances WHERE id = ?`), id)
	return err
}

// AgentListFilter specifies optional filters for listing agents.
type AgentListFilter struct {
	Role      string
	Status    string
	ReportsTo string
}

// ListAgentInstancesFiltered returns agent instances for a workspace matching filters.
// An empty workspaceID returns rows from all workspaces.
func (r *Repository) ListAgentInstancesFiltered(
	ctx context.Context, workspaceID string, filter AgentListFilter,
) ([]*models.AgentInstance, error) {
	var (
		conds []string
		args  []interface{}
	)
	if workspaceID != "" {
		conds = append(conds, "workspace_id = ?")
		args = append(args, workspaceID)
	}
	if filter.Role != "" {
		conds = append(conds, "role = ?")
		args = append(args, filter.Role)
	}
	if filter.Status != "" {
		conds = append(conds, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.ReportsTo != "" {
		conds = append(conds, "reports_to = ?")
		args = append(args, filter.ReportsTo)
	}
	query := `SELECT * FROM orchestrate_agent_instances`
	for i, c := range conds {
		if i == 0 {
			query += ` WHERE ` + c
		} else {
			query += ` AND ` + c
		}
	}
	query += ` ORDER BY created_at`

	var agents []*models.AgentInstance
	err := r.ro.SelectContext(ctx, &agents, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	if agents == nil {
		agents = []*models.AgentInstance{}
	}
	return agents, nil
}
