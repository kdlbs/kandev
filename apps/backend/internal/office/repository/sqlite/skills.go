package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

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

// ListNonSystemSkills returns all is_system = false skills for a
// workspace, ordered by slug. Used by the system-skill sync's
// slug-migration pass: user/provider-imported rows that need a
// well-formed-but-non-canonical slug normalized to canonical, and the
// conflict check before inserting a newly-bundled canonical slug.
func (r *Repository) ListNonSystemSkills(
	ctx context.Context, workspaceID string,
) ([]*models.Skill, error) {
	var skills []*models.Skill
	err := r.ro.SelectContext(ctx, &skills, r.ro.Rebind(
		`SELECT * FROM office_skills WHERE workspace_id = ? AND is_system = 0 ORDER BY slug`),
		workspaceID)
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

// NormalizeSkillSlug atomically changes a non-system skill's slug and
// rewrites matching desired_skills references on all active office agents.
// The skill ID and old slug are both part of the update predicate, so a
// concurrent writer cannot cause references to be rewritten for a different
// row. It returns false when the row no longer matches that identity.
func (r *Repository) NormalizeSkillSlug(
	ctx context.Context,
	workspaceID, skillID, oldSlug, newSlug string,
) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin skill slug normalization: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE office_skills
		SET slug = ?, updated_at = ?
		WHERE id = ? AND workspace_id = ? AND slug = ? AND is_system = 0
	`), newSlug, time.Now().UTC(), skillID, workspaceID, oldSlug)
	if err != nil {
		return false, err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check skill slug normalization: %w", err)
	}
	if updated == 0 {
		return false, nil
	}

	updates, err := collectAgentDesiredSkillUpdates(ctx, tx, workspaceID, oldSlug, newSlug)
	if err != nil {
		return false, err
	}
	if err := applyAgentDesiredSkillUpdates(ctx, tx, workspaceID, updates); err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit skill slug normalization: %w", err)
	}
	return true, nil
}

type agentDesiredSkillUpdate struct {
	id       string
	original string
	desired  string
}

func collectAgentDesiredSkillUpdates(
	ctx context.Context,
	tx *sqlx.Tx,
	workspaceID, oldSlug, newSlug string,
) ([]agentDesiredSkillUpdate, error) {
	rows, err := tx.QueryxContext(ctx, tx.Rebind(`
		SELECT id, COALESCE(desired_skills, '') AS desired_skills
		FROM agent_profiles
		WHERE workspace_id = ? AND deleted_at IS NULL
	`), workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list agents for skill slug normalization: %w", err)
	}
	defer func() { _ = rows.Close() }()

	updates := make([]agentDesiredSkillUpdate, 0)
	for rows.Next() {
		var update agentDesiredSkillUpdate
		if err := rows.Scan(&update.id, &update.original); err != nil {
			return nil, fmt.Errorf("scan agent for skill slug normalization: %w", err)
		}
		update.desired, _ = replaceSkillSlugInAgentList(update.original, oldSlug, newSlug)
		if update.desired != update.original {
			updates = append(updates, update)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read agents for skill slug normalization: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close agents for skill slug normalization: %w", err)
	}
	return updates, nil
}

func applyAgentDesiredSkillUpdates(
	ctx context.Context,
	tx *sqlx.Tx,
	workspaceID string,
	updates []agentDesiredSkillUpdate,
) error {
	now := time.Now().UTC()
	for _, update := range updates {
		result, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE agent_profiles
			SET desired_skills = ?, updated_at = ?
			WHERE id = ? AND workspace_id = ? AND deleted_at IS NULL
				AND COALESCE(desired_skills, '') = ?
		`), update.desired, now, update.id, workspaceID, update.original)
		if err != nil {
			return fmt.Errorf("update agent %s for skill slug normalization: %w", update.id, err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("check agent %s for skill slug normalization: %w", update.id, err)
		}
		if rowsAffected != 1 {
			return fmt.Errorf("agent %s changed during skill slug normalization", update.id)
		}
	}
	return nil
}

// replaceSkillSlugInAgentList mirrors the office desired_skills reader. It
// accepts both the canonical JSON-array format and the legacy CSV format, and
// emits a deduplicated JSON array only when a value changes.
func replaceSkillSlugInAgentList(raw, oldSlug, newSlug string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return raw, false
	}
	values, ok := parseAgentSkillList(raw)
	if !ok {
		return raw, false
	}

	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	changed := false
	for _, value := range values {
		if value == oldSlug {
			value = newSlug
			changed = true
		}
		if value == "" || seen[value] {
			if value != "" {
				changed = true
			}
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	if !changed {
		return raw, false
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return raw, false
	}
	return string(encoded), true
}

func parseAgentSkillList(raw string) ([]string, bool) {
	if strings.HasPrefix(raw, "[") {
		var values []string
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			return nil, false
		}
		return values, true
	}
	parts := strings.Split(raw, ",")
	values := make([]string, len(parts))
	for i, part := range parts {
		values[i] = strings.TrimSpace(part)
	}
	return values, true
}

// SkillConfigFields is the subset of office_skills columns a config import
// owns (see UpdateSkillConfigFields).
type SkillConfigFields struct {
	Name        string
	Description string
	SourceType  models.SkillSourceType
	Content     string
}

// UpdateSkillConfigFields updates only the columns a config import owns
// (name, description, source type, content) and the content_hash derived from
// content plus the current package metadata. It leaves source_locator,
// file_inventory, version, approval_state, and the other columns untouched
// instead of reverting them to a stale read-then-write snapshot.
func (r *Repository) UpdateSkillConfigFields(
	ctx context.Context, id string, fields SkillConfigFields,
) error {
	const maxMetadataReadAttempts = 3
	type packageMetadata struct {
		SourceLocator string `db:"source_locator"`
		FileInventory string `db:"file_inventory"`
	}

	for range maxMetadataReadAttempts {
		var metadata packageMetadata
		if err := r.db.QueryRowxContext(ctx, r.db.Rebind(`
			SELECT COALESCE(source_locator, '') AS source_locator,
				COALESCE(file_inventory, '') AS file_inventory
			FROM office_skills
			WHERE id = ?
		`), id).StructScan(&metadata); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("skill not found: %s", id)
			}
			return err
		}

		contentHash := models.SkillPackageContentHash(
			fields.Content, metadata.FileInventory, metadata.SourceLocator,
		)
		result, err := r.db.ExecContext(ctx, r.db.Rebind(`
			UPDATE office_skills SET
				name = ?, description = ?, source_type = ?, content = ?,
				content_hash = ?, updated_at = ?
			WHERE id = ?
				AND COALESCE(source_locator, '') = ?
				AND COALESCE(file_inventory, '') = ?
		`), fields.Name, fields.Description, fields.SourceType, fields.Content,
			contentHash, time.Now().UTC(), id, metadata.SourceLocator, metadata.FileInventory)
		if err != nil {
			return err
		}
		updated, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if updated == 1 {
			return nil
		}
	}

	return fmt.Errorf("update skill %s: package metadata changed concurrently", id)
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
