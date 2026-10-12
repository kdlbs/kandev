package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// UpdateWorkspaceFields writes supplied columns and returns the row observed
// by that statement. A supplied timestamp fences the same atomic write.
func (r *Repository) UpdateWorkspaceFields(ctx context.Context, id string, update models.WorkspaceFieldUpdate, expected *time.Time) (*models.Workspace, error) {
	assignments := []string{"updated_at = ?"}
	args := []any{time.Now().UTC()}
	appendValue := func(column string, value any) {
		assignments = append(assignments, column+" = ?")
		args = append(args, value)
	}
	appendString := func(column string, value *string) {
		if value != nil {
			appendValue(column, *value)
		}
	}
	appendDefault := func(column string, value **string) {
		if value != nil {
			appendValue(column, *value)
		}
	}
	appendString("name", update.Name)
	appendString("description", update.Description)
	appendString("unit_id", update.UnitID)
	appendDefault("default_executor_id", update.DefaultExecutorID)
	appendDefault("default_environment_id", update.DefaultEnvironmentID)
	appendDefault("default_agent_profile_id", update.DefaultAgentProfileID)
	appendDefault("default_config_agent_profile_id", update.DefaultConfigAgentProfileID)
	if update.ACPIdleSuspensionEnabled != nil {
		appendValue("acp_idle_suspension_enabled", *update.ACPIdleSuspensionEnabled)
	}
	if update.ACPIdleTimeoutMinutes != nil {
		appendValue("acp_idle_timeout_minutes", *update.ACPIdleTimeoutMinutes)
	}
	args = append(args, id)
	query := "UPDATE workspaces SET " + strings.Join(assignments, ", ") + " WHERE id = ?"
	if expected != nil {
		query += optimisticUpdatedAtPredicate
		args = append(args, *expected)
	}
	query += " RETURNING " + workspaceSelectColumns
	workspace, err := scanWorkspaceRow(r.db.QueryRowContext(ctx, r.db.Rebind(query), args...))
	if errors.Is(err, sql.ErrNoRows) {
		if expected != nil {
			return nil, repoerrors.ErrTaskVersionConflict
		}
		return nil, workspaceNotFoundError(id)
	}
	return workspace, err
}
