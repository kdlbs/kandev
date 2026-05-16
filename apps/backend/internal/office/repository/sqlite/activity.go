package sqlite

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/office/models"
)

// CreateActivityEntry creates a new activity log entry. RunID and
// SessionID are optional; when set they let the run detail page join
// activity rows back to the originating run (the "Tasks Touched"
// surface) without a separate join table.
func (r *Repository) CreateActivityEntry(ctx context.Context, entry *models.ActivityEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_activity_log (
			id, workspace_id, actor_type, actor_id, action,
			target_type, target_id, details, run_id, session_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), entry.ID, entry.WorkspaceID, entry.ActorType, entry.ActorID,
		entry.Action, entry.TargetType, entry.TargetID, entry.Details,
		entry.RunID, entry.SessionID, entry.CreatedAt)
	return err
}

// ListTasksTouchedByRun returns the distinct task ids the agent
// acted on during a given run, sourced from the activity log via the
// run_id column. Used by the run detail page's Tasks Touched table.
// The query intentionally restricts to entity_type='task'; the
// run-detail handler is expected to union the run's primary task
// (from the run payload) with this set, since a run can produce no
// activity rows for its assigned task if the agent only commented.
func (r *Repository) ListTasksTouchedByRun(
	ctx context.Context, runID string,
) ([]string, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT DISTINCT target_id
		FROM office_activity_log
		WHERE run_id = ? AND target_type = 'task' AND target_id != ''
	`), runID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ListActivityEntries returns activity entries for a workspace, most recent first.
func (r *Repository) ListActivityEntries(ctx context.Context, workspaceID string, limit int) ([]*models.ActivityEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	var entries []*models.ActivityEntry
	err := r.ro.SelectContext(ctx, &entries, r.ro.Rebind(
		`SELECT * FROM office_activity_log WHERE workspace_id = ? ORDER BY created_at DESC LIMIT ?`),
		workspaceID, limit)
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []*models.ActivityEntry{}
	}
	return entries, nil
}

// ListActivityEntriesByAction returns activity entries matching a specific action prefix.
func (r *Repository) ListActivityEntriesByAction(ctx context.Context, workspaceID, action string, limit int) ([]*models.ActivityEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	var entries []*models.ActivityEntry
	err := r.ro.SelectContext(ctx, &entries, r.ro.Rebind(
		`SELECT * FROM office_activity_log
		 WHERE workspace_id = ? AND action = ?
		 ORDER BY created_at DESC LIMIT ?`),
		workspaceID, action, limit)
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []*models.ActivityEntry{}
	}
	return entries, nil
}

// ListActivityEntriesByType returns activity entries filtered by target_type or actor_type.
func (r *Repository) ListActivityEntriesByType(ctx context.Context, workspaceID, filterType string, limit int) ([]*models.ActivityEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	var entries []*models.ActivityEntry
	err := r.ro.SelectContext(ctx, &entries, r.ro.Rebind(
		`SELECT * FROM office_activity_log
		 WHERE workspace_id = ? AND (target_type = ? OR actor_type = ? OR action LIKE ?)
		 ORDER BY created_at DESC LIMIT ?`),
		workspaceID, filterType, filterType, filterType+".%", limit)
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []*models.ActivityEntry{}
	}
	return entries, nil
}

// ListActivityEntriesByTarget returns activity entries for one target in a workspace.
func (r *Repository) ListActivityEntriesByTarget(ctx context.Context, workspaceID, targetID string, limit int) ([]*models.ActivityEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	var entries []*models.ActivityEntry
	err := r.ro.SelectContext(ctx, &entries, r.ro.Rebind(
		`SELECT * FROM office_activity_log
		 WHERE workspace_id = ? AND target_id = ?
		 ORDER BY created_at DESC LIMIT ?`),
		workspaceID, targetID, limit)
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []*models.ActivityEntry{}
	}
	return entries, nil
}
