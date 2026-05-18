package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/office/models"
)

// CreateSkill creates a new skill.
func (r *Repository) CreateSkill(ctx context.Context, skill *models.Skill) error {
	if skill.ID == "" {
		skill.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	skill.CreatedAt = now
	skill.UpdatedAt = now

	if skill.DefaultForRoles == "" {
		skill.DefaultForRoles = "[]"
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_skills (
			id, workspace_id, name, slug, description, source_type,
			source_locator, content, file_inventory, version, content_hash,
			approval_state, created_by_agent_profile_id,
			is_system, system_version, default_for_roles,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), skill.ID, skill.WorkspaceID, skill.Name, skill.Slug, skill.Description,
		skill.SourceType, skill.SourceLocator, skill.Content, skill.FileInventory,
		skill.Version, skill.ContentHash, skill.ApprovalState,
		skill.CreatedByAgentProfileID,
		skill.IsSystem, skill.SystemVersion, skill.DefaultForRoles,
		skill.CreatedAt, skill.UpdatedAt)
	return err
}

// GetSkill returns a skill by ID.
func (r *Repository) GetSkill(ctx context.Context, id string) (*models.Skill, error) {
	var skill models.Skill
	query := `SELECT * FROM office_skills WHERE id = ?`
	if err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(query), id).StructScan(&skill); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("skill not found: %s", id)
		}
		return nil, err
	}
	return &skill, nil
}

// GetSkillBySlug returns a skill by workspace+slug.
func (r *Repository) GetSkillBySlug(
	ctx context.Context, workspaceID, slug string,
) (*models.Skill, error) {
	var skill models.Skill
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT * FROM office_skills WHERE workspace_id = ? AND slug = ?`),
		workspaceID, slug).StructScan(&skill)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("skill not found: %s", slug)
	}
	return &skill, err
}

// ListSkills returns all skills for a workspace, ordered by name.
// An empty workspaceID returns rows from all workspaces.
func (r *Repository) ListSkills(ctx context.Context, workspaceID string) ([]*models.Skill, error) {
	var (
		skills []*models.Skill
		err    error
	)
	if workspaceID == "" {
		err = r.ro.SelectContext(ctx, &skills,
			`SELECT * FROM office_skills ORDER BY name`)
	} else {
		err = r.ro.SelectContext(ctx, &skills, r.ro.Rebind(
			`SELECT * FROM office_skills WHERE workspace_id = ? ORDER BY name`), workspaceID)
	}
	if err != nil {
		return nil, err
	}
	if skills == nil {
		return []*models.Skill{}, nil
	}
	return skills, nil
}

// ListSystemSkills returns all is_system = true skills for a
// workspace, ordered by slug for deterministic startup logs. An
// empty workspaceID returns system rows across every workspace.
func (r *Repository) ListSystemSkills(
	ctx context.Context, workspaceID string,
) ([]*models.Skill, error) {
	var (
		skills []*models.Skill
		err    error
	)
	if workspaceID == "" {
		err = r.ro.SelectContext(ctx, &skills,
			`SELECT * FROM office_skills WHERE is_system = 1 ORDER BY workspace_id, slug`)
	} else {
		err = r.ro.SelectContext(ctx, &skills, r.ro.Rebind(
			`SELECT * FROM office_skills WHERE workspace_id = ? AND is_system = 1 ORDER BY slug`),
			workspaceID)
	}
	if err != nil {
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
	if skill.DefaultForRoles == "" {
		skill.DefaultForRoles = "[]"
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_skills SET
			name = ?, slug = ?, description = ?, source_type = ?,
			source_locator = ?, content = ?, file_inventory = ?,
			version = ?, content_hash = ?, approval_state = ?,
			created_by_agent_profile_id = ?,
			is_system = ?, system_version = ?, default_for_roles = ?,
			updated_at = ?
		WHERE id = ?
	`), skill.Name, skill.Slug, skill.Description, skill.SourceType,
		skill.SourceLocator, skill.Content, skill.FileInventory,
		skill.Version, skill.ContentHash, skill.ApprovalState,
		skill.CreatedByAgentProfileID,
		skill.IsSystem, skill.SystemVersion, skill.DefaultForRoles,
		skill.UpdatedAt, skill.ID)
	return err
}

// DeleteSkill deletes a skill by ID.
func (r *Repository) DeleteSkill(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM office_skills WHERE id = ?`), id)
	return err
}

// CreateRunSkillSnapshots records the immutable skill set used by a run.
func (r *Repository) CreateRunSkillSnapshots(ctx context.Context, snapshots []models.RunSkillSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PreparexContext(ctx, r.db.Rebind(`
		INSERT OR REPLACE INTO office_run_skills (
			run_id, skill_id, version, content_hash, materialized_path
		) VALUES (?, ?, ?, ?, ?)
	`))
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()
	for _, snap := range snapshots {
		if _, err := stmt.ExecContext(ctx,
			snap.RunID, snap.SkillID, snap.Version,
			snap.ContentHash, snap.MaterializedPath,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListRunSkillSnapshots returns skill snapshots for a run.
func (r *Repository) ListRunSkillSnapshots(ctx context.Context, runID string) ([]models.RunSkillSnapshot, error) {
	var snapshots []models.RunSkillSnapshot
	err := r.ro.SelectContext(ctx, &snapshots, r.ro.Rebind(`
		SELECT * FROM office_run_skills
		WHERE run_id = ?
		ORDER BY skill_id
	`), runID)
	if err != nil {
		return nil, err
	}
	if snapshots == nil {
		return []models.RunSkillSnapshot{}, nil
	}
	return snapshots, nil
}
