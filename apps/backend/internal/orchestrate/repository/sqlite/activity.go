package sqlite

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// CreateActivityEntry creates a new activity log entry.
func (r *Repository) CreateActivityEntry(ctx context.Context, entry *models.ActivityEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	entry.CreatedAt = time.Now().UTC()

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO orchestrate_activity_log (
			id, workspace_id, actor_type, actor_id, action,
			target_type, target_id, details, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), entry.ID, entry.WorkspaceID, entry.ActorType, entry.ActorID,
		entry.Action, entry.TargetType, entry.TargetID, entry.Details, entry.CreatedAt)
	return err
}

// ListActivityEntries returns activity entries for a workspace, most recent first.
func (r *Repository) ListActivityEntries(ctx context.Context, workspaceID string, limit int) ([]*models.ActivityEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	var entries []*models.ActivityEntry
	err := r.ro.SelectContext(ctx, &entries, r.ro.Rebind(
		`SELECT * FROM orchestrate_activity_log WHERE workspace_id = ? ORDER BY created_at DESC LIMIT ?`),
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
		`SELECT * FROM orchestrate_activity_log
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
		`SELECT * FROM orchestrate_activity_log
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
