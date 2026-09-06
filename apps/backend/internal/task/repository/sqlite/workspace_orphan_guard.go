package sqlite

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// SetTaskWorkspaceMetadataIfUnchanged writes the whole $.workspace object
// only when every clause the guard asks for still holds — the check and the
// write are one statement, so a concurrent archive/unarchive/reparent cannot
// interleave into a mixed marker. Reports whether it landed.
//
// The two CAS clauses (ExpectedOrphanedParentID, ExpectedMode) are always
// compared, including when empty. Each Require* clause is appended only when
// its field is non-zero. See models.OrphanWriteGuard for the full contract
// this mirrors in SQL.
func (r *Repository) SetTaskWorkspaceMetadataIfUnchanged(
	ctx context.Context, taskID string,
	guard models.OrphanWriteGuard, value map[string]interface{},
) (bool, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return false, err
	}

	isPostgres := dialect.IsPostgres(r.db.DriverName())
	var b strings.Builder
	args := make([]interface{}, 0, 12)

	if isPostgres {
		const m = `CASE WHEN metadata IS NULL OR metadata = 'null' OR metadata = '' THEN '{}'::jsonb ELSE metadata::jsonb END`
		b.WriteString(`UPDATE tasks SET metadata = jsonb_set(` + m + `, ARRAY['workspace'], ?::jsonb, true)::text, updated_at = ? WHERE id = ?`)
		args = append(args, string(payload), time.Now().UTC(), taskID)
		b.WriteString(` AND COALESCE(CASE WHEN jsonb_typeof(` + m + ` -> 'workspace' -> 'orphaned_parent_id') = 'string' THEN ` + m + ` #>> ARRAY['workspace','orphaned_parent_id'] END, '') = ?`)
		args = append(args, guard.ExpectedOrphanedParentID)
		b.WriteString(` AND COALESCE(CASE WHEN jsonb_typeof(` + m + ` -> 'workspace' -> 'mode') = 'string' THEN ` + m + ` #>> ARRAY['workspace','mode'] END, '') = ?`)
		args = append(args, guard.ExpectedMode)
	} else {
		b.WriteString(`UPDATE tasks SET metadata = json_set(CASE WHEN metadata IS NULL OR metadata = 'null' OR metadata = '' THEN '{}' ELSE metadata END, '$.workspace', json(?)), updated_at = ? WHERE id = ?`)
		args = append(args, string(payload), time.Now().UTC(), taskID)
		b.WriteString(` AND COALESCE(CASE WHEN json_valid(metadata) AND json_type(metadata,'$.workspace.orphaned_parent_id') = 'text' THEN json_extract(metadata,'$.workspace.orphaned_parent_id') END,'') = ?`)
		args = append(args, guard.ExpectedOrphanedParentID)
		b.WriteString(` AND COALESCE(CASE WHEN json_valid(metadata) AND json_type(metadata,'$.workspace.mode') = 'text' THEN json_extract(metadata,'$.workspace.mode') END,'') = ?`)
		args = append(args, guard.ExpectedMode)
	}

	if guard.RequireParentID != "" {
		b.WriteString(` AND parent_id = ?`)
		args = append(args, guard.RequireParentID)
	}
	if guard.RequireParentArchivedID != "" {
		b.WriteString(` AND EXISTS (SELECT 1 FROM tasks p WHERE p.id = ? AND p.archived_at IS NOT NULL)`)
		args = append(args, guard.RequireParentArchivedID)
	}
	if guard.RequireParentUnarchivedID != "" {
		b.WriteString(` AND EXISTS (SELECT 1 FROM tasks p WHERE p.id = ? AND p.archived_at IS NULL)`)
		args = append(args, guard.RequireParentUnarchivedID)
	}
	if guard.RequireNoOwnEnvironment {
		b.WriteString(` AND NOT EXISTS (SELECT 1 FROM task_environments e WHERE e.task_id = ?)`)
		args = append(args, taskID)
	}
	if guard.RequireTaskNotArchived {
		b.WriteString(` AND archived_at IS NULL`)
	}

	result, err := r.db.ExecContext(ctx, r.db.Rebind(b.String()), args...)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}
