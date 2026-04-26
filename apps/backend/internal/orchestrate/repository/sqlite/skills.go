package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// CreateSkill creates a new skill.
func (r *Repository) CreateSkill(ctx context.Context, skill *models.Skill) error {
	if skill.ID == "" {
		skill.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	skill.CreatedAt = now
	skill.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO orchestrate_skills (
			id, workspace_id, name, slug, description, source_type,
			source_locator, content, file_inventory, created_by_agent_instance_id,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), skill.ID, skill.WorkspaceID, skill.Name, skill.Slug, skill.Description,
		skill.SourceType, skill.SourceLocator, skill.Content, skill.FileInventory,
		skill.CreatedByAgentInstanceID, skill.CreatedAt, skill.UpdatedAt)
	return err
}

// GetSkill returns a skill by ID.
func (r *Repository) GetSkill(ctx context.Context, id string) (*models.Skill, error) {
	var skill models.Skill
	query := `SELECT * FROM orchestrate_skills WHERE id = ?`
	if err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(query), id).StructScan(&skill); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("skill not found: %s", id)
		}
		return nil, err
	}
	return &skill, nil
}

// ListSkills returns all skills for a workspace, ordered by name.
func (r *Repository) ListSkills(ctx context.Context, workspaceID string) ([]*models.Skill, error) {
	query := `SELECT * FROM orchestrate_skills WHERE workspace_id = ? ORDER BY name`
	var skills []*models.Skill
	if err := r.ro.SelectContext(ctx, &skills, r.ro.Rebind(query), workspaceID); err != nil {
		return nil, err
	}
	if skills == nil {
		return []*models.Skill{}, nil
	}
	return skills, nil
}

// UpdateSkill updates an existing skill.
func (r *Repository) UpdateSkill(ctx context.Context, skill *models.Skill) error {
	skill.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE orchestrate_skills SET
			name = ?, slug = ?, description = ?, source_type = ?,
			source_locator = ?, content = ?, file_inventory = ?,
			created_by_agent_instance_id = ?, updated_at = ?
		WHERE id = ?
	`), skill.Name, skill.Slug, skill.Description, skill.SourceType,
		skill.SourceLocator, skill.Content, skill.FileInventory,
		skill.CreatedByAgentInstanceID, skill.UpdatedAt, skill.ID)
	return err
}

// DeleteSkill deletes a skill by ID.
func (r *Repository) DeleteSkill(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM orchestrate_skills WHERE id = ?`), id)
	return err
}
