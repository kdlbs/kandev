package sqlite

import (
	"context"
	"encoding/json"
)

// WorkspaceTaskOwner reads persisted task ownership, never event-supplied metadata.
func (r *Repository) WorkspaceTaskOwner(ctx context.Context, taskID string) (string, string, error) {
	var row struct {
		WorkspaceID string `db:"workspace_id"`
		Metadata    string `db:"metadata"`
	}
	err := r.ro.GetContext(ctx, &row, r.ro.Rebind(`SELECT workspace_id, COALESCE(metadata,'{}') AS metadata FROM tasks WHERE id = ? AND archived_at IS NULL`), taskID)
	if err != nil {
		return "", "", err
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(row.Metadata), &metadata); err != nil {
		return "", "", err
	}
	chief, _ := metadata["orchestration_chief_id"].(string)
	return row.WorkspaceID, chief, nil
}
