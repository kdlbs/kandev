package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Label represents a workspace-scoped label in the catalog.
type Label struct {
	ID          string    `json:"id" db:"id"`
	WorkspaceID string    `json:"workspace_id" db:"workspace_id"`
	Name        string    `json:"name" db:"name"`
	Color       string    `json:"color" db:"color"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// defaultColors is the round-robin palette for auto-created labels.
var defaultColors = []string{
	"#ef4444", "#f59e0b", "#10b981", "#3b82f6",
	"#8b5cf6", "#ec4899", "#14b8a6", "#f97316",
}

// nextLabelColor picks a color from the palette based on the current label count.
func nextLabelColor(count int) string {
	return defaultColors[count%len(defaultColors)]
}

// GetOrCreateLabel returns the existing label by name, or creates a new one.
// The color is assigned round-robin based on the workspace label count.
func (r *Repository) GetOrCreateLabel(ctx context.Context, workspaceID, name string) (*Label, error) {
	// Attempt idempotent insert.
	var count int
	if err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT COUNT(*) FROM office_labels WHERE workspace_id = ?`), workspaceID).Scan(&count); err != nil {
		return nil, fmt.Errorf("count labels: %w", err)
	}
	color := nextLabelColor(count)
	now := time.Now().UTC()
	id := uuid.New().String()

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT OR IGNORE INTO office_labels (id, workspace_id, name, color, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`), id, workspaceID, name, color, now, now)
	if err != nil {
		return nil, fmt.Errorf("insert label: %w", err)
	}

	return r.GetLabelByName(ctx, workspaceID, name)
}

// GetLabelByName returns a label by workspace and name.
func (r *Repository) GetLabelByName(ctx context.Context, workspaceID, name string) (*Label, error) {
	var label Label
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT * FROM office_labels WHERE workspace_id = ? AND name = ?`),
		workspaceID, name).StructScan(&label)
	if err != nil {
		return nil, fmt.Errorf("get label by name: %w", err)
	}
	return &label, nil
}

// AddLabelToTask creates a junction row linking a task to a label.
// Uses INSERT OR IGNORE so it is idempotent.
func (r *Repository) AddLabelToTask(ctx context.Context, taskID, labelID string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT OR IGNORE INTO office_task_labels (task_id, label_id, created_at)
		VALUES (?, ?, ?)
	`), taskID, labelID, now)
	return err
}

// RemoveLabelFromTask deletes the junction row for a task/label pair.
func (r *Repository) RemoveLabelFromTask(ctx context.Context, taskID, labelID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM office_task_labels WHERE task_id = ? AND label_id = ?`),
		taskID, labelID)
	return err
}

// ListLabelsForTask returns all labels attached to a task.
func (r *Repository) ListLabelsForTask(ctx context.Context, taskID string) ([]*Label, error) {
	var labels []*Label
	err := r.ro.SelectContext(ctx, &labels, r.ro.Rebind(`
		SELECT l.*
		FROM office_labels l
		JOIN office_task_labels tl ON tl.label_id = l.id
		WHERE tl.task_id = ?
		ORDER BY l.name
	`), taskID)
	if err != nil {
		return nil, fmt.Errorf("list labels for task: %w", err)
	}
	if labels == nil {
		labels = []*Label{}
	}
	return labels, nil
}

// ListLabelsForTasks returns a map of taskID → labels for a batch of task IDs.
func (r *Repository) ListLabelsForTasks(ctx context.Context, taskIDs []string) (map[string][]*Label, error) {
	result := make(map[string][]*Label, len(taskIDs))
	if len(taskIDs) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(taskIDs))
	args := make([]interface{}, len(taskIDs))
	for i, id := range taskIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	type row struct {
		TaskID string `db:"task_id"`
		Label
	}

	var rows []row
	q := fmt.Sprintf(`
		SELECT tl.task_id, l.id, l.workspace_id, l.name, l.color, l.created_at, l.updated_at
		FROM office_labels l
		JOIN office_task_labels tl ON tl.label_id = l.id
		WHERE tl.task_id IN (%s)
		ORDER BY tl.task_id, l.name
	`, strings.Join(placeholders, ","))

	if err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(q), args...); err != nil {
		return nil, fmt.Errorf("batch list labels: %w", err)
	}

	for _, row := range rows {
		lbl := row.Label
		result[row.TaskID] = append(result[row.TaskID], &lbl)
	}
	return result, nil
}

// ListLabelsByWorkspace returns all labels in a workspace catalog.
func (r *Repository) ListLabelsByWorkspace(ctx context.Context, workspaceID string) ([]*Label, error) {
	var labels []*Label
	err := r.ro.SelectContext(ctx, &labels, r.ro.Rebind(
		`SELECT * FROM office_labels WHERE workspace_id = ? ORDER BY name`),
		workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list workspace labels: %w", err)
	}
	if labels == nil {
		labels = []*Label{}
	}
	return labels, nil
}

// UpdateLabel updates the name and color of a label by ID.
func (r *Repository) UpdateLabel(ctx context.Context, id, name, color string) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_labels SET name = ?, color = ?, updated_at = ? WHERE id = ?
	`), name, color, now, id)
	if err != nil {
		return fmt.Errorf("update label: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("label not found: %s", id)
	}
	return nil
}

// DeleteLabel deletes a label from the catalog. The ON DELETE CASCADE on
// office_task_labels removes junction rows automatically.
func (r *Repository) DeleteLabel(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM office_labels WHERE id = ?`), id)
	return err
}
