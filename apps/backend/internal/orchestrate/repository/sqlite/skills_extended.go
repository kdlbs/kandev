package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// GetSkillBySlug returns a skill by workspace ID and slug.
// Returns nil, nil if not found.
func (r *Repository) GetSkillBySlug(ctx context.Context, workspaceID, slug string) (*models.Skill, error) {
	var skill models.Skill
	query := `SELECT * FROM orchestrate_skills WHERE workspace_id = ? AND slug = ?`
	if err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(query), workspaceID, slug).StructScan(&skill); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &skill, nil
}

// CountSkillUsage returns a map of skill ID to the count of agent instances using it.
func (r *Repository) CountSkillUsage(ctx context.Context, workspaceID string) (map[string]int, error) {
	query := `SELECT desired_skills FROM orchestrate_agent_instances WHERE workspace_id = ?`
	var rows []string
	if err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(query), workspaceID); err != nil {
		return nil, err
	}
	counts := make(map[string]int)
	for _, raw := range rows {
		var ids []string
		if err := json.Unmarshal([]byte(raw), &ids); err != nil {
			continue
		}
		for _, id := range ids {
			counts[id]++
		}
	}
	return counts, nil
}
