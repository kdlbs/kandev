package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const (
	coordinatorAuditRetention     = 10_000
	coordinatorAuditResultPending = "pending"
)

const coordinatorGrantColumns = `id, coordinator_task_id, principal_id, workspace_id, scope_kind, scope_id, capabilities, note, granted_by_user_id, granted_at, revoked_at, revoked_by_user_id`

func (r *Repository) CreateWorkspaceAgentPrincipal(ctx context.Context, principal *models.WorkspaceAgentPrincipal) error {
	if principal.ID == "" {
		principal.ID = uuid.NewString()
	}
	if principal.CreatedAt.IsZero() {
		principal.CreatedAt = r.nowUTC()
	}
	principal.UpdatedAt = principal.CreatedAt
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO workspace_agent_principals (id, workspace_id, plugin_installation_id, logical_key, backing_task_id, backing_session_id, revoked_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`), principal.ID, principal.WorkspaceID, principal.PluginInstallationID, principal.LogicalKey, principal.BackingTaskID, principal.BackingSessionID, principal.RevokedAt, principal.CreatedAt, principal.UpdatedAt)
	if isPrincipalContextUniqueViolation(err) {
		return repoerrors.ErrWorkspaceAgentPrincipalConflict
	}
	return err
}

func (r *Repository) GetWorkspaceAgentPrincipal(ctx context.Context, id string) (*models.WorkspaceAgentPrincipal, error) {
	p := &models.WorkspaceAgentPrincipal{}
	var revoked sql.NullTime
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`SELECT id, workspace_id, plugin_installation_id, logical_key, backing_task_id, backing_session_id, revoked_at, created_at, updated_at FROM workspace_agent_principals WHERE id = ?`), id).Scan(&p.ID, &p.WorkspaceID, &p.PluginInstallationID, &p.LogicalKey, &p.BackingTaskID, &p.BackingSessionID, &revoked, &p.CreatedAt, &p.UpdatedAt)
	if revoked.Valid {
		p.RevokedAt = &revoked.Time
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repoerrors.ErrWorkspaceAgentPrincipalNotFound
	}
	return p, err
}

func (r *Repository) GetWorkspaceAgentPrincipalByContext(ctx context.Context, workspaceID, pluginInstallationID, logicalKey string) (*models.WorkspaceAgentPrincipal, error) {
	var id string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`SELECT id FROM workspace_agent_principals WHERE workspace_id = ? AND plugin_installation_id = ? AND logical_key = ?`), workspaceID, pluginInstallationID, logicalKey).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repoerrors.ErrWorkspaceAgentPrincipalNotFound
	}
	if err != nil {
		return nil, err
	}
	return r.GetWorkspaceAgentPrincipal(ctx, id)
}

// GetActiveWorkspaceAgentPrincipalForTask resolves the durable subject bound
// to the currently acting task. Revoked bindings intentionally look absent so
// a revocation takes effect on the next authorization check.
func (r *Repository) GetActiveWorkspaceAgentPrincipalForTask(ctx context.Context, workspaceID, taskID string) (*models.WorkspaceAgentPrincipal, error) {
	var id string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`SELECT id FROM workspace_agent_principals WHERE workspace_id = ? AND backing_task_id = ? AND revoked_at IS NULL`), workspaceID, taskID).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r.GetWorkspaceAgentPrincipal(ctx, id)
}

func (r *Repository) RebindWorkspaceAgentPrincipal(ctx context.Context, id, taskID, sessionID string, updatedAt time.Time) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE workspace_agent_principals SET backing_task_id = ?, backing_session_id = ?, updated_at = ? WHERE id = ? AND revoked_at IS NULL`), taskID, sessionID, updatedAt, id)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed == 0 {
		if err != nil {
			return err
		}
		// The row is absent or already revoked; without reading it back the two
		// cases are indistinguishable, which keeps revocation timelines opaque.
		return repoerrors.ErrWorkspaceAgentPrincipalNotFound
	}
	return nil
}

func (r *Repository) RevokeWorkspaceAgentPrincipal(ctx context.Context, id string, revokedAt time.Time) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE workspace_agent_principals SET revoked_at = ?, updated_at = ? WHERE id = ? AND revoked_at IS NULL`), revokedAt, revokedAt, id)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed == 0 {
		if err != nil {
			return err
		}
		return repoerrors.ErrWorkspaceAgentPrincipalNotFound
	}
	return nil
}

func (r *Repository) CreateCoordinatorGrant(ctx context.Context, grant *models.CoordinatorGrant) error {
	if grant.ID == "" {
		grant.ID = uuid.NewString()
	}
	if grant.GrantedAt.IsZero() {
		grant.GrantedAt = r.nowUTC()
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_coordinator_grants (
			id, coordinator_task_id, principal_id, workspace_id, scope_kind, scope_id, capabilities,
			note, granted_by_user_id, granted_at, revoked_at, revoked_by_user_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), grant.ID, grant.CoordinatorTaskID, grant.PrincipalID, grant.WorkspaceID, grant.ScopeKind, grant.ScopeID,
		grant.Capabilities, grant.Note, grant.GrantedByUserID, grant.GrantedAt, grant.RevokedAt, grant.RevokedByUserID)
	if isPrincipalGrantScopeUniqueViolation(err) {
		return repoerrors.ErrCoordinatorGrantConflict
	}
	return err
}

func (r *Repository) ListCoordinatorGrants(ctx context.Context, workspaceID, coordinatorTaskID string, includeRevoked bool) ([]*models.CoordinatorGrant, error) {
	query := `SELECT ` + coordinatorGrantColumns + ` FROM task_coordinator_grants WHERE workspace_id = ?`
	args := []interface{}{workspaceID}
	if coordinatorTaskID != "" {
		query += ` AND coordinator_task_id = ?`
		args = append(args, coordinatorTaskID)
	}
	if !includeRevoked {
		query += ` AND revoked_at IS NULL`
	}
	query += ` ORDER BY granted_at DESC, id DESC`
	return r.listCoordinatorGrants(ctx, query, args...)
}

func (r *Repository) ListActiveCoordinatorGrants(ctx context.Context, coordinatorTaskID, workspaceID string) ([]*models.CoordinatorGrant, error) {
	return r.listCoordinatorGrants(ctx, `
		SELECT `+coordinatorGrantColumns+`
		FROM task_coordinator_grants
		WHERE coordinator_task_id = ? AND workspace_id = ? AND revoked_at IS NULL
		ORDER BY granted_at DESC, id DESC`, coordinatorTaskID, workspaceID)
}

// ListActiveWorkspaceAgentPrincipalGrants is the principal-only authorization
// path. Legacy task-bound rows have an empty principal_id and never match.
func (r *Repository) ListActiveWorkspaceAgentPrincipalGrants(ctx context.Context, principalID, workspaceID string) ([]*models.CoordinatorGrant, error) {
	return r.listCoordinatorGrants(ctx, `SELECT `+coordinatorGrantColumns+` FROM task_coordinator_grants WHERE principal_id = ? AND workspace_id = ? AND revoked_at IS NULL`, principalID, workspaceID)
}

func (r *Repository) RevokeCoordinatorGrant(ctx context.Context, id, revokedByUserID string, revokedAt time.Time) error {
	if revokedAt.IsZero() {
		revokedAt = r.nowUTC()
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_coordinator_grants
		SET revoked_at = ?, revoked_by_user_id = ?
		WHERE id = ? AND revoked_at IS NULL`), revokedAt, revokedByUserID, id)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed == 0 {
		if err != nil {
			return err
		}
		return repoerrors.ErrCoordinatorGrantNotFound
	}
	return nil
}

func (r *Repository) CreateCoordinatorAuditEvent(ctx context.Context, event *models.CoordinatorAuditEvent) error {
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = r.nowUTC()
	}
	if event.Result == "" {
		event.Result = coordinatorAuditResultPending
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_coordinator_audit_events (
			id, occurred_at, actor_task_id, actor_session_id, target_task_id, workspace_id,
			action, capability, decision, deny_reason, grant_id, result, detail
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), event.ID, event.OccurredAt, event.ActorTaskID, event.ActorSessionID, event.TargetTaskID,
		event.WorkspaceID, event.Action, event.Capability, event.Decision, event.DenyReason,
		event.GrantID, event.Result, event.Detail); err != nil {
		return err
	}
	pruneQuery := `
		DELETE FROM task_coordinator_audit_events
		WHERE id IN (
			SELECT id FROM task_coordinator_audit_events
			ORDER BY occurred_at DESC, id DESC
			LIMIT -1 OFFSET ?
		)`
	if dialect.IsPostgres(r.db.DriverName()) {
		pruneQuery = `
			DELETE FROM task_coordinator_audit_events
			WHERE id IN (
				SELECT id FROM task_coordinator_audit_events
				ORDER BY occurred_at DESC, id DESC
				LIMIT ALL OFFSET ?
			)`
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(pruneQuery), coordinatorAuditRetention); err != nil {
		return fmt.Errorf("prune coordinator audit events: %w", err)
	}
	return tx.Commit()
}

func (r *Repository) GetCoordinatorGrant(ctx context.Context, id string) (*models.CoordinatorGrant, error) {
	grant, err := r.getCoordinatorGrant(ctx, `WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repoerrors.ErrCoordinatorGrantNotFound
	}
	return grant, err
}

func (r *Repository) FinishCoordinatorAuditEvent(ctx context.Context, id, result, detail string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE task_coordinator_audit_events SET result = ?, detail = ? WHERE id = ?`), result, detail, id)
	return err
}

func (r *Repository) ListCoordinatorAuditEvents(ctx context.Context, workspaceID, taskID string, limit int) ([]*models.CoordinatorAuditEvent, error) {
	query := `SELECT id, occurred_at, actor_task_id, actor_session_id, target_task_id, workspace_id, action, capability, decision, deny_reason, grant_id, result, detail FROM task_coordinator_audit_events WHERE workspace_id = ?`
	args := []interface{}{workspaceID}
	if taskID != "" {
		query += ` AND (actor_task_id = ? OR target_task_id = ?)`
		args = append(args, taskID, taskID)
	}
	query += ` ORDER BY occurred_at DESC, id DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	events := make([]*models.CoordinatorAuditEvent, 0)
	for rows.Next() {
		event := &models.CoordinatorAuditEvent{}
		if err := rows.Scan(&event.ID, &event.OccurredAt, &event.ActorTaskID, &event.ActorSessionID, &event.TargetTaskID, &event.WorkspaceID, &event.Action, &event.Capability, &event.Decision, &event.DenyReason, &event.GrantID, &event.Result, &event.Detail); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (r *Repository) getCoordinatorGrant(ctx context.Context, where string, args ...interface{}) (*models.CoordinatorGrant, error) {
	grants, err := r.listCoordinatorGrants(ctx, `SELECT `+coordinatorGrantColumns+` FROM task_coordinator_grants `+where, args...)
	if err != nil || len(grants) == 0 {
		if err == nil {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return grants[0], nil
}

func (r *Repository) listCoordinatorGrants(ctx context.Context, query string, args ...interface{}) ([]*models.CoordinatorGrant, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	grants := make([]*models.CoordinatorGrant, 0)
	for rows.Next() {
		grant := &models.CoordinatorGrant{}
		var revokedAt sql.NullTime
		if err := rows.Scan(&grant.ID, &grant.CoordinatorTaskID, &grant.PrincipalID, &grant.WorkspaceID, &grant.ScopeKind, &grant.ScopeID, &grant.Capabilities, &grant.Note, &grant.GrantedByUserID, &grant.GrantedAt, &revokedAt, &grant.RevokedByUserID); err != nil {
			return nil, err
		}
		if revokedAt.Valid {
			grant.RevokedAt = &revokedAt.Time
		}
		grants = append(grants, grant)
	}
	return grants, rows.Err()
}
