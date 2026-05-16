package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/office/models"
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
		INSERT INTO office_projects (
			id, workspace_id, name, description, status, lead_agent_profile_id,
			color, budget_cents, repositories, executor_config, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), project.ID, project.WorkspaceID, project.Name, project.Description,
		project.Status, project.LeadAgentProfileID, project.Color, project.BudgetCents,
		project.Repositories, project.ExecutorConfig, project.CreatedAt, project.UpdatedAt)
	return err
}

// GetProject returns a project by ID.
func (r *Repository) GetProject(ctx context.Context, id string) (*models.Project, error) {
	var project models.Project
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT * FROM office_projects WHERE id = ?`), id).StructScan(&project)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("project not found: %s", id)
	}
	return &project, err
}

// GetProjectByName returns a project by workspace+name.
func (r *Repository) GetProjectByName(
	ctx context.Context, workspaceID, name string,
) (*models.Project, error) {
	var project models.Project
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT * FROM office_projects WHERE workspace_id = ? AND name = ?`),
		workspaceID, name).StructScan(&project)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("project not found: %s", name)
	}
	return &project, err
}

// ListProjects returns all projects for a workspace.
// An empty workspaceID returns rows from all workspaces.
func (r *Repository) ListProjects(ctx context.Context, workspaceID string) ([]*models.Project, error) {
	var (
		projects []*models.Project
		err      error
	)
	if workspaceID == "" {
		err = r.ro.SelectContext(ctx, &projects,
			`SELECT * FROM office_projects ORDER BY name`)
	} else {
		err = r.ro.SelectContext(ctx, &projects, r.ro.Rebind(
			`SELECT * FROM office_projects WHERE workspace_id = ? ORDER BY name`), workspaceID)
	}
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
		UPDATE office_projects SET
			name = ?, description = ?, status = ?, lead_agent_profile_id = ?,
			color = ?, budget_cents = ?, repositories = ?, executor_config = ?,
			updated_at = ?
		WHERE id = ?
	`), project.Name, project.Description, project.Status, project.LeadAgentProfileID,
		project.Color, project.BudgetCents, project.Repositories, project.ExecutorConfig,
		project.UpdatedAt, project.ID)
	return err
}

// DeleteProject deletes a project by ID.
func (r *Repository) DeleteProject(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM office_projects WHERE id = ?`), id)
	return err
}

// projectCountsRow holds the per-project task aggregate from the batched
// LEFT JOIN query in ListProjectsWithCounts.
type projectCountsRow struct {
	ProjectID  string `db:"project_id"`
	Total      int    `db:"total"`
	InProgress int    `db:"in_progress"`
	Done       int    `db:"done"`
	Blocked    int    `db:"blocked"`
}

// ListProjectsWithCounts returns all projects for a workspace with task
// counts. Counts are fetched in a single batched query (one round-trip
// regardless of project count) — was N+1.
func (r *Repository) ListProjectsWithCounts(
	ctx context.Context, workspaceID string,
) ([]*models.ProjectWithCounts, error) {
	projects, err := r.ListProjects(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if len(projects) == 0 {
		return []*models.ProjectWithCounts{}, nil
	}

	// Batch task counts grouped by project_id. The LEFT JOIN ensures
	// projects with zero tasks still appear in the result with all
	// counts at 0.
	placeholders := make([]string, len(projects))
	args := make([]interface{}, 0, len(projects))
	for i, p := range projects {
		placeholders[i] = "?"
		args = append(args, p.ID)
	}
	query := `SELECT p.id AS project_id,
	                 COUNT(t.id) AS total,
	                 COALESCE(SUM(CASE WHEN t.state IN ('IN_PROGRESS', 'SCHEDULING') THEN 1 ELSE 0 END), 0) AS in_progress,
	                 COALESCE(SUM(CASE WHEN t.state = 'COMPLETED' THEN 1 ELSE 0 END), 0) AS done,
	                 COALESCE(SUM(CASE WHEN t.state = 'BLOCKED' THEN 1 ELSE 0 END), 0) AS blocked
	          FROM office_projects p
	          LEFT JOIN tasks t ON t.project_id = p.id
	          WHERE p.id IN (` + strings.Join(placeholders, ",") + `)
	          GROUP BY p.id`
	var rows []projectCountsRow
	countsByProject := make(map[string]models.TaskCounts, len(projects))
	if err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(query), args...); err == nil {
		for _, row := range rows {
			countsByProject[row.ProjectID] = models.TaskCounts{
				Total:      row.Total,
				InProgress: row.InProgress,
				Done:       row.Done,
				Blocked:    row.Blocked,
			}
		}
	}

	result := make([]*models.ProjectWithCounts, 0, len(projects))
	for _, p := range projects {
		result = append(result, &models.ProjectWithCounts{
			Project:    *p,
			TaskCounts: countsByProject[p.ID],
		})
	}
	return result, nil
}
