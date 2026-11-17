package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
)

// GetBuiltinMCPApplication excludes intent from a task's previous workspace.
func (r *Repository) GetBuiltinMCPApplication(ctx context.Context, sessionID string) (*models.BuiltinMCPApplication, error) {
	var application models.BuiltinMCPApplication
	err := r.ro.GetContext(ctx, &application, r.ro.Rebind(`SELECT a.* FROM task_builtin_mcp_applications a
 JOIN task_sessions s ON s.id = a.session_id AND s.task_id = a.task_id
 JOIN tasks t ON t.id = a.task_id AND t.workspace_id = a.workspace_id
 WHERE a.session_id = ?`), sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &application, nil
}
