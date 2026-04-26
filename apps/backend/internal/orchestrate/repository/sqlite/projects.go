package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// CreateProject creates a new project.
func (r *Repository) CreateProject(ctx context.Context, project *models.Project) error {
	if project.ID == "" {
		project.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	project.CreatedAt = now
	project.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO orchestrate_projects (
			id, workspace_id, name, description, status, lead_agent_instance_id,
			color, budget_cents, repositories, executor_config, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), project.ID, project.WorkspaceID, project.Name, project.Description,
		project.Status, project.LeadAgentInstanceID, project.Color, project.BudgetCents,
		project.Repositories, project.ExecutorConfig, project.CreatedAt, project.UpdatedAt)
	return err
}

// GetProject returns a project by ID.
func (r *Repository) GetProject(ctx context.Context, id string) (*models.Project, error) {
	var project models.Project
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT * FROM orchestrate_projects WHERE id = ?`), id).StructScan(&project)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("project not found: %s", id)
	}
	return &project, err
}

// ListProjects returns all projects for a workspace.
func (r *Repository) ListProjects(ctx context.Context, workspaceID string) ([]*models.Project, error) {
	var projects []*models.Project
	err := r.ro.SelectContext(ctx, &projects, r.ro.Rebind(
		`SELECT * FROM orchestrate_projects WHERE workspace_id = ? ORDER BY name`), workspaceID)
	if err != nil {
		return nil, err
	}
	if projects == nil {
		projects = []*models.Project{}
	}
	return projects, nil
}

// UpdateProject updates an existing project.
func (r *Repository) UpdateProject(ctx context.Context, project *models.Project) error {
	project.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE orchestrate_projects SET
			name = ?, description = ?, status = ?, lead_agent_instance_id = ?,
			color = ?, budget_cents = ?, repositories = ?, executor_config = ?,
			updated_at = ?
		WHERE id = ?
	`), project.Name, project.Description, project.Status, project.LeadAgentInstanceID,
		project.Color, project.BudgetCents, project.Repositories, project.ExecutorConfig,
		project.UpdatedAt, project.ID)
	return err
}

// DeleteProject deletes a project by ID.
func (r *Repository) DeleteProject(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM orchestrate_projects WHERE id = ?`), id)
	return err
}
