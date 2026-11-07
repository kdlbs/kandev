package sqlite

import (
	"context"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/office/models"
)

// Cross-workspace aggregate queries for the read-only multi-workspace
// dashboard overview (GET /api/v1/office/workspaces/aggregate). Each method
// accepts a workspace-id set and processes bounded batches so the query stays
// below SQLite's host-parameter limit.

const maxAggregateWorkspaceIDsPerQuery = 400

// WorkspaceTaskCountRow is a raw row from the cross-workspace task-count query.
type WorkspaceTaskCountRow struct {
	WorkspaceID string `db:"workspace_id"`
	State       string `db:"state"`
	Count       int    `db:"count"`
}

// QueryWorkspaceTaskBreakdowns returns task counts grouped by state for each
// workspace in workspaceIDs. Workspaces with no matching tasks are absent
// from the result map.
func (r *Repository) QueryWorkspaceTaskBreakdowns(ctx context.Context, workspaceIDs []string) (map[string]models.TaskBreakdown, error) {
	out := make(map[string]models.TaskBreakdown, len(workspaceIDs))
	for _, batch := range workspaceIDBatches(workspaceIDs) {
		placeholders, args := placeholdersFor(batch)
		var rows []WorkspaceTaskCountRow
		query := `
			SELECT workspace_id, COALESCE(state,'') as state, COUNT(*) as count
			FROM tasks
			WHERE workspace_id IN (` + strings.Join(placeholders, ",") + `)
			  AND is_ephemeral = 0` + andNotAutomationOrigin + `
			  AND archived_at IS NULL
			GROUP BY workspace_id, state
		`
		err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(query), args...)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			bd := out[row.WorkspaceID]
			bucketTaskBreakdownRow(&bd, row.State, row.Count)
			out[row.WorkspaceID] = bd
		}
	}
	return out, nil
}

// CountPendingApprovalsByWorkspaces returns pending approval counts keyed by
// workspace id, for the supplied workspace set.
func (r *Repository) CountPendingApprovalsByWorkspaces(ctx context.Context, workspaceIDs []string) (map[string]int, error) {
	out := make(map[string]int, len(workspaceIDs))
	for _, batch := range workspaceIDBatches(workspaceIDs) {
		placeholders, args := placeholdersFor(batch)
		var rows []struct {
			WorkspaceID string `db:"workspace_id"`
			Count       int    `db:"count"`
		}
		query := `
			SELECT workspace_id, COUNT(*) as count
			FROM office_approvals
			WHERE workspace_id IN (` + strings.Join(placeholders, ",") + `)
			  AND status = 'pending'
			GROUP BY workspace_id
		`
		err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(query), args...)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			out[row.WorkspaceID] = row.Count
		}
	}
	return out, nil
}

// ListActivityEntriesForWorkspaces returns the newest activity entries across
// the supplied workspace set, up to limit entries total.
func (r *Repository) ListActivityEntriesForWorkspaces(ctx context.Context, workspaceIDs []string, limit int) ([]*models.ActivityEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	entries := make([]*models.ActivityEntry, 0, limit)
	for _, batch := range workspaceIDBatches(workspaceIDs) {
		placeholders, args := placeholdersFor(batch)
		args = append(args, limit)
		var batchEntries []*models.ActivityEntry
		query := `SELECT * FROM office_activity_log
			 WHERE workspace_id IN (` + strings.Join(placeholders, ",") + `)
			 ORDER BY created_at DESC, id DESC LIMIT ?`
		err := r.ro.SelectContext(ctx, &batchEntries, r.ro.Rebind(query), args...)
		if err != nil {
			return nil, err
		}
		entries = append(entries, batchEntries...)
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].CreatedAt.Equal(entries[j].CreatedAt) {
				return entries[i].ID > entries[j].ID
			}
			return entries[i].CreatedAt.After(entries[j].CreatedAt)
		})
		if len(entries) > limit {
			entries = append([]*models.ActivityEntry(nil), entries[:limit]...)
		}
	}
	return entries, nil
}

// workspaceIDBatches keeps every IN query below SQLite's conservative host
// parameter limit while preserving the caller's workspace order.
func workspaceIDBatches(ids []string) [][]string {
	batches := make([][]string, 0, (len(ids)+maxAggregateWorkspaceIDsPerQuery-1)/maxAggregateWorkspaceIDsPerQuery)
	for start := 0; start < len(ids); start += maxAggregateWorkspaceIDsPerQuery {
		end := start + maxAggregateWorkspaceIDsPerQuery
		if end > len(ids) {
			end = len(ids)
		}
		batches = append(batches, ids[start:end])
	}
	return batches
}

// placeholdersFor builds a `?,?,...` list and the matching argument slice for
// a set of ids.
func placeholdersFor(ids []string) ([]string, []interface{}) {
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	return placeholders, args
}

// bucketTaskBreakdownRow folds one state-count row into a TaskBreakdown.
// Shared with BucketTaskBreakdown so the per-workspace and cross-workspace
// breakdowns cannot drift on the state mapping.
func bucketTaskBreakdownRow(bd *models.TaskBreakdown, state string, count int) {
	switch state {
	case "COMPLETED":
		bd.Done += count
	case "IN_PROGRESS", "SCHEDULING":
		bd.InProgress += count
	case "BLOCKED":
		bd.Blocked += count
	default:
		bd.Open += count
	}
}
