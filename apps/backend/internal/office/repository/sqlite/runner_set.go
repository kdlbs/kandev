package sqlite

import "context"

// ListRunnerSetTaskIDs returns the ordered, capped ids of tasks the given
// agent is currently responsible for within one workspace — the "runner
// set" a taskless run's task scope materializes from
// (docs/specs/office/system-design/taskless-coordinator-authority-01.md#task-scope-derivation)
// — plus the true total before capping, so a caller can detect truncation.
// Ordered by (updated_at DESC, id DESC). The total is read from the same
// query snapshot as the ids via a window function, not a second query, so a
// concurrent board mutation between two separate reads cannot desync them.
//
// Shares CountActionableTasksForAgent's four clauses (runner projection
// equals the agent, actionable state, not archived, not automation-origin)
// plus a workspace_id filter CountActionableTasksForAgent does not have:
// this scope is a write authority, so it is filtered explicitly rather
// than relying on agent profiles happening to be workspace-unique.
func (r *Repository) ListRunnerSetTaskIDs(
	ctx context.Context, agentID, workspaceID string, capAt int,
) ([]string, int, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT t.id, COUNT(*) OVER() AS total FROM tasks t
		WHERE `+RunnerProjection("t")+` = ?
		  AND t.workspace_id = ?
		  AND t.state IN ('TODO', 'IN_PROGRESS')
		  AND t.archived_at IS NULL`+andNotAutomationOriginT+`
		ORDER BY t.updated_at DESC, t.id DESC
		LIMIT ?
	`), agentID, workspaceID, capAt)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	ids := make([]string, 0)
	total := 0
	for rows.Next() {
		var id string
		var rowTotal int
		if err := rows.Scan(&id, &rowTotal); err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
		total = rowTotal
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return ids, total, nil
}
