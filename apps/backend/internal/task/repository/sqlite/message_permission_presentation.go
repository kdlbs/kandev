package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// UpdatePermissionPresentation changes the presentation of one exact
// permission request. The full task/session/request/pending identity prevents a
// delayed event about a replaced request from changing a newer row. Optional
// constraints keep a concurrent resolver's claim and terminal outcome
// authoritative.
func (r *Repository) UpdatePermissionPresentation(
	ctx context.Context,
	request models.PermissionPresentationUpdate,
) (*models.Message, bool, error) {
	if request.TaskID == "" || request.SessionID == "" || request.RequestID == "" || request.PendingID == "" {
		return nil, false, errors.New("permission presentation update requires full identity")
	}
	if !isStoredPermissionPresentation(request.Presentation) {
		return nil, false, fmt.Errorf("invalid permission presentation %q", request.Presentation)
	}
	if request.RequirePresentation != "" && !isStoredPermissionPresentation(request.RequirePresentation) {
		return nil, false, fmt.Errorf("invalid required permission presentation %q", request.RequirePresentation)
	}

	driver := r.db.DriverName()
	query := fmt.Sprintf(`
		UPDATE task_session_messages
		SET metadata = %s, updated_at = ?
		WHERE task_id = ? AND task_session_id = ? AND type = 'permission_request'
		  AND %s = ? AND %s = ?
	`, permissionPresentationJSONExpression(driver),
		permissionJSONExtract(driver, "metadata", "request_id"),
		permissionJSONExtract(driver, "metadata", "pending_id"),
	)
	args := []interface{}{
		string(request.Presentation),
		time.Now().UTC(),
		request.TaskID,
		request.SessionID,
		request.RequestID,
		request.PendingID,
	}
	if request.RequirePresentation != "" {
		query += fmt.Sprintf(" AND COALESCE(%s, '') = ?",
			permissionJSONExtract(driver, "metadata", models.PermissionPresentationMetadataKey))
		args = append(args, string(request.RequirePresentation))
	}
	if request.RequireUnclaimed {
		query += fmt.Sprintf(" AND %s IS NULL",
			permissionJSONExtract(driver, "metadata", "permission_resolution"))
	}

	result, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return nil, false, fmt.Errorf("update permission presentation: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, false, fmt.Errorf("update permission presentation rows: %w", err)
	}
	if rows == 0 {
		return nil, false, nil
	}
	message, err := r.getPermissionMessageByIdentity(ctx, request.TaskID, request.SessionID, request.RequestID, request.PendingID)
	if err != nil {
		return nil, false, fmt.Errorf("read updated permission presentation: %w", err)
	}
	return message, true, nil
}

func isStoredPermissionPresentation(value models.PermissionPresentation) bool {
	switch value {
	case models.PermissionPresentationAutomatic,
		models.PermissionPresentationInteractive,
		models.PermissionPresentationUnavailable:
		return true
	default:
		return false
	}
}

func permissionPresentationJSONExpression(driver string) string {
	if dialect.IsPostgres(driver) {
		return `jsonb_set(COALESCE(NULLIF(metadata, ''), '{}')::jsonb, '{permission_presentation}', to_jsonb(?::text), true)::text`
	}
	return `json_set(COALESCE(NULLIF(metadata, ''), '{}'), '$.permission_presentation', ?)`
}
