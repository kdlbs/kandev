package sqlite

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// ListOrphanRepairCandidates returns every live, non-ephemeral,
// non-automation-origin inherit_parent child of an archived parent that is
// not already marked and has no task_environments row of its own. Mirrors
// the producer's ListChildren filter exactly (is_ephemeral, origin), per the
// startup-repair design's `## Startup repair`.
func (r *Repository) ListOrphanRepairCandidates(ctx context.Context) ([]models.OrphanRepairCandidate, error) {
	isPostgres := dialect.IsPostgres(r.db.DriverName())
	var query string
	if isPostgres {
		query = `
			SELECT c.id, p.id AS parent_id,
			       CASE WHEN c.metadata IS NULL OR c.metadata = '' THEN '{}'::jsonb ELSE c.metadata::jsonb END -> 'workspace' AS workspace
			FROM tasks c JOIN tasks p ON c.parent_id = p.id
			WHERE c.archived_at IS NULL AND p.archived_at IS NOT NULL
			  AND c.is_ephemeral = 0
			  AND COALESCE(c.origin,'') != '` + models.TaskOriginAutomationRun + `'
			  AND (CASE WHEN c.metadata IS NULL OR c.metadata = '' THEN '{}'::jsonb ELSE c.metadata::jsonb END) #>> '{workspace,mode}' = 'inherit_parent'
			  AND (CASE WHEN c.metadata IS NULL OR c.metadata = '' THEN '{}'::jsonb ELSE c.metadata::jsonb END) #> '{workspace,orphaned}' IS DISTINCT FROM 'true'::jsonb
			  AND NOT EXISTS (SELECT 1 FROM task_environments e WHERE e.task_id = c.id)
			ORDER BY c.created_at, c.id
		`
	} else {
		query = `
			SELECT c.id, p.id AS parent_id,
			       json_extract(c.metadata,'$.workspace') AS workspace
			FROM tasks c JOIN tasks p ON c.parent_id = p.id
			WHERE c.archived_at IS NULL AND p.archived_at IS NOT NULL
			  AND c.is_ephemeral = 0
			  AND COALESCE(c.origin,'') != '` + models.TaskOriginAutomationRun + `'
			  AND json_valid(c.metadata)
			  AND json_extract(c.metadata,'$.workspace.mode') = 'inherit_parent'
			  AND json_type(c.metadata,'$.workspace.orphaned') IS NOT 'true'
			  AND NOT EXISTS (SELECT 1 FROM task_environments e WHERE e.task_id = c.id)
			ORDER BY c.created_at, c.id
		`
	}

	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []models.OrphanRepairCandidate
	for rows.Next() {
		var taskID, parentID string
		var workspaceJSON []byte
		if err := rows.Scan(&taskID, &parentID, &workspaceJSON); err != nil {
			return nil, err
		}
		var workspace map[string]interface{}
		if len(workspaceJSON) > 0 {
			if err := json.Unmarshal(workspaceJSON, &workspace); err != nil {
				return nil, err
			}
		}
		out = append(out, models.OrphanRepairCandidate{TaskID: taskID, ParentID: parentID, Workspace: workspace})
	}
	return out, rows.Err()
}

// ListStaleOrphanMarkers returns every task (archived or not — AC-003.9a)
// carrying workspace.orphaned == true whose orphaned_parent_id names a task
// that exists and is not archived: a provably false claim.
func (r *Repository) ListStaleOrphanMarkers(ctx context.Context) ([]models.StaleOrphanMarker, error) {
	isPostgres := dialect.IsPostgres(r.db.DriverName())
	var query string
	if isPostgres {
		query = `
			WITH claims AS (
				SELECT id, created_at, workspace_id,
				       (CASE WHEN metadata IS NULL OR metadata = '' THEN '{}'::jsonb ELSE metadata::jsonb END) -> 'workspace' AS workspace,
				       (CASE WHEN metadata IS NULL OR metadata = '' THEN '{}'::jsonb ELSE metadata::jsonb END) #>> '{workspace,orphaned_parent_id}' AS claim_id
				FROM tasks
				WHERE jsonb_typeof((CASE WHEN metadata IS NULL OR metadata = '' THEN '{}'::jsonb ELSE metadata::jsonb END) #> '{workspace,orphaned}') = 'boolean'
				  AND (CASE WHEN metadata IS NULL OR metadata = '' THEN '{}'::jsonb ELSE metadata::jsonb END) #> '{workspace,orphaned}' = 'true'::jsonb
			)
			SELECT c.id, c.workspace
			FROM claims c JOIN tasks p ON c.claim_id = p.id AND p.workspace_id = c.workspace_id
			WHERE p.archived_at IS NULL
			ORDER BY c.created_at, c.id
		`
	} else {
		query = `
			WITH claims AS (
			  SELECT id, created_at, workspace_id,
			         CASE WHEN json_valid(metadata)
			              THEN json_extract(metadata,'$.workspace') END AS workspace,
			         CASE WHEN json_valid(metadata)
			               AND json_type(metadata,'$.workspace.orphaned_parent_id') = 'text'
			              THEN json_extract(metadata,'$.workspace.orphaned_parent_id') END AS claim_id
			  FROM tasks
			  WHERE json_valid(metadata)
			    AND json_type(metadata,'$.workspace.orphaned') IS 'true'
			)
			SELECT c.id, c.workspace
			FROM claims c JOIN tasks p ON c.claim_id = p.id AND p.workspace_id = c.workspace_id
			WHERE p.archived_at IS NULL
			ORDER BY c.created_at, c.id
		`
	}

	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []models.StaleOrphanMarker
	for rows.Next() {
		var taskID string
		var workspaceJSON []byte
		if err := rows.Scan(&taskID, &workspaceJSON); err != nil {
			return nil, err
		}
		var workspace map[string]interface{}
		if len(workspaceJSON) > 0 {
			if err := json.Unmarshal(workspaceJSON, &workspace); err != nil {
				return nil, err
			}
		}
		out = append(out, models.StaleOrphanMarker{TaskID: taskID, Workspace: workspace})
	}
	return out, rows.Err()
}

// CountMalformedTaskMetadata counts rows whose metadata is non-empty and
// unparseable — the shapes that would otherwise abort the selections above
// silently. NULL, empty string, and 'null' are excluded: they carry no
// $.workspace.mode and could never have been candidates, so counting them
// would warn on a healthy install. Skipped on Postgres (no json_valid; every
// writer marshals from a Go map or casts, so malformed non-empty text has no
// producer there — same residual as detachTaskQuery).
func (r *Repository) CountMalformedTaskMetadata(ctx context.Context) (int, error) {
	if dialect.IsPostgres(r.db.DriverName()) {
		return 0, nil
	}
	var count int
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT COUNT(*) FROM tasks
		WHERE metadata IS NOT NULL AND metadata <> '' AND metadata <> 'null'
		  AND NOT json_valid(metadata)
	`)).Scan(&count)
	return count, err
}
