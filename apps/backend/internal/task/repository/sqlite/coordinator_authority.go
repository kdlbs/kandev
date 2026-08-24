package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
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
	if strings.TrimSpace(principal.PluginInstallationID) == "" {
		// A task-bound or installation-less row is legacy ambiguous state. It
		// must never become a durable plugin principal.
		return repoerrors.ErrWorkspaceAgentPrincipalNotFound
	}
	if principal.ID == "" {
		principal.ID = uuid.NewString()
	}
	if principal.CreatedAt.IsZero() {
		principal.CreatedAt = r.nowUTC()
	}
	principal.UpdatedAt = principal.CreatedAt
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO workspace_agent_principals (id, workspace_id, plugin_installation_id, logical_key, backing_task_id, backing_session_id, revoked_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`), principal.ID, principal.WorkspaceID, principal.PluginInstallationID, principal.LogicalKey, principal.BackingTaskID, principal.BackingSessionID, principal.RevokedAt, principal.CreatedAt, principal.UpdatedAt)
	if isPrincipalConflictViolation(err) {
		return repoerrors.ErrWorkspaceAgentPrincipalConflict
	}
	return err
}

func (r *Repository) GetWorkspaceAgentPrincipal(ctx context.Context, id string) (*models.WorkspaceAgentPrincipal, error) {
	p := &models.WorkspaceAgentPrincipal{}
	var revoked sql.NullTime
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`SELECT id, workspace_id, plugin_installation_id, logical_key, backing_task_id, backing_session_id, revoked_at, created_at, updated_at FROM workspace_agent_principals WHERE id = ? AND plugin_installation_id != ''`), id).Scan(&p.ID, &p.WorkspaceID, &p.PluginInstallationID, &p.LogicalKey, &p.BackingTaskID, &p.BackingSessionID, &revoked, &p.CreatedAt, &p.UpdatedAt)
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
	if strings.TrimSpace(pluginInstallationID) == "" {
		return nil, repoerrors.ErrWorkspaceAgentPrincipalNotFound
	}
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`SELECT id FROM workspace_agent_principals WHERE workspace_id = ? AND plugin_installation_id = ? AND plugin_installation_id != '' AND logical_key = ?`), workspaceID, pluginInstallationID, logicalKey).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repoerrors.ErrWorkspaceAgentPrincipalNotFound
	}
	if err != nil {
		return nil, err
	}
	return r.GetWorkspaceAgentPrincipal(ctx, id)
}

func (r *Repository) ListWorkspaceAgentPrincipalsByPluginInstallation(ctx context.Context, pluginInstallationID string) ([]*models.WorkspaceAgentPrincipal, error) {
	if strings.TrimSpace(pluginInstallationID) == "" {
		return []*models.WorkspaceAgentPrincipal{}, nil
	}
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`SELECT id, workspace_id, plugin_installation_id, logical_key, backing_task_id, backing_session_id, revoked_at, created_at, updated_at FROM workspace_agent_principals WHERE plugin_installation_id = ? AND plugin_installation_id != '' ORDER BY created_at, id`), pluginInstallationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]*models.WorkspaceAgentPrincipal, 0)
	for rows.Next() {
		principal := &models.WorkspaceAgentPrincipal{}
		var revoked sql.NullTime
		if err := rows.Scan(&principal.ID, &principal.WorkspaceID, &principal.PluginInstallationID, &principal.LogicalKey, &principal.BackingTaskID, &principal.BackingSessionID, &revoked, &principal.CreatedAt, &principal.UpdatedAt); err != nil {
			return nil, err
		}
		if revoked.Valid {
			principal.RevokedAt = &revoked.Time
		}
		out = append(out, principal)
	}
	return out, rows.Err()
}

// GetActiveWorkspaceAgentPrincipalForTask resolves the durable subject bound
// to the currently acting task. Revoked bindings intentionally look absent so
// a revocation takes effect on the next authorization check.
func (r *Repository) GetActiveWorkspaceAgentPrincipalForTask(ctx context.Context, workspaceID, taskID string) (*models.WorkspaceAgentPrincipal, error) {
	var id string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`SELECT id FROM workspace_agent_principals WHERE workspace_id = ? AND backing_task_id = ? AND plugin_installation_id != '' AND revoked_at IS NULL`), workspaceID, taskID).Scan(&id)
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
	if isPrincipalConflictViolation(err) {
		return repoerrors.ErrWorkspaceAgentPrincipalConflict
	}
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

func (r *Repository) GetCoordinatorGrant(ctx context.Context, id string) (*models.CoordinatorGrant, error) {
	grant, err := r.getCoordinatorGrant(ctx, `WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repoerrors.ErrCoordinatorGrantNotFound
	}
	return grant, err
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

// ListWorkspaceAgentPrincipalGrants looks up grants by durable principal ID.
// Empty principal IDs are legacy task-bound grants and are intentionally never
// visible through this path.
func (r *Repository) ListWorkspaceAgentPrincipalGrants(ctx context.Context, workspaceID, principalID string, includeRevoked bool) ([]*models.CoordinatorGrant, error) {
	if principalID == "" {
		return []*models.CoordinatorGrant{}, nil
	}
	query := `SELECT ` + coordinatorGrantColumns + ` FROM task_coordinator_grants WHERE workspace_id = ? AND principal_id = ? AND principal_id != ''`
	args := []interface{}{workspaceID, principalID}
	if !includeRevoked {
		query += ` AND revoked_at IS NULL`
	}
	query += ` ORDER BY granted_at DESC, id DESC`
	return r.listCoordinatorGrants(ctx, query, args...)
}

// ListActiveWorkspaceAgentPrincipalGrants is the principal-only authorization
// path. Legacy task-bound rows have an empty principal_id and never match.
func (r *Repository) ListActiveWorkspaceAgentPrincipalGrants(ctx context.Context, principalID, workspaceID string) ([]*models.CoordinatorGrant, error) {
	return r.ListWorkspaceAgentPrincipalGrants(ctx, workspaceID, principalID, false)
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
			id, occurred_at, principal_id, actor_task_id, actor_session_id, target_task_id, workspace_id,
			action, capability, decision, deny_reason, grant_id, result, detail
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), event.ID, event.OccurredAt, event.PrincipalID, event.ActorTaskID, event.ActorSessionID, event.TargetTaskID,
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

func (r *Repository) FinishCoordinatorAuditEvent(ctx context.Context, id, result, detail string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE task_coordinator_audit_events SET result = ?, detail = ? WHERE id = ?`), result, detail, id)
	return err
}

func (r *Repository) ListCoordinatorAuditEvents(ctx context.Context, workspaceID, taskID string, limit int) ([]*models.CoordinatorAuditEvent, error) {
	query := `SELECT id, occurred_at, principal_id, actor_task_id, actor_session_id, target_task_id, workspace_id, action, capability, decision, deny_reason, grant_id, result, detail FROM task_coordinator_audit_events WHERE workspace_id = ?`
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
		if err := rows.Scan(&event.ID, &event.OccurredAt, &event.PrincipalID, &event.ActorTaskID, &event.ActorSessionID, &event.TargetTaskID, &event.WorkspaceID, &event.Action, &event.Capability, &event.Decision, &event.DenyReason, &event.GrantID, &event.Result, &event.Detail); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

// ListWorkspaceAgentPrincipalAuditEvents is the durable principal-audit
// query. Actor task/session bindings may change after repair, so querying by
// those replaceable ids would lose history.
func (r *Repository) ListWorkspaceAgentPrincipalAuditEvents(ctx context.Context, workspaceID, principalID string, limit int) ([]*models.CoordinatorAuditEvent, error) {
	return r.ListWorkspaceAgentPrincipalAuditEventsFiltered(ctx, workspaceID, models.WorkspaceAgentAuditFilter{PrincipalID: principalID}, limit)
}

// ListWorkspaceAgentPrincipalAuditEventsFiltered is the server-side filtered
// audit query. Task and session identifiers remain internal; callers above
// this layer receive only the redacted public audit projection.
func (r *Repository) ListWorkspaceAgentPrincipalAuditEventsFiltered(ctx context.Context, workspaceID string, filter models.WorkspaceAgentAuditFilter, limit int) ([]*models.CoordinatorAuditEvent, error) {
	query := `SELECT id, occurred_at, principal_id, actor_task_id, actor_session_id, target_task_id, workspace_id, action, capability, decision, deny_reason, grant_id, result, detail FROM task_coordinator_audit_events WHERE workspace_id = ?`
	args := []interface{}{workspaceID}
	if filter.PrincipalID != "" {
		query += ` AND principal_id = ?`
		args = append(args, filter.PrincipalID)
	}
	if filter.ActorTaskID != "" {
		query += ` AND actor_task_id = ?`
		args = append(args, filter.ActorTaskID)
	}
	if filter.TargetTaskID != "" {
		query += ` AND target_task_id = ?`
		args = append(args, filter.TargetTaskID)
	}
	if filter.Action != "" {
		query += ` AND action = ?`
		args = append(args, filter.Action)
	}
	if filter.Decision != "" {
		query += ` AND decision = ?`
		args = append(args, filter.Decision)
	}
	if filter.Result != "" {
		query += ` AND result = ?`
		args = append(args, filter.Result)
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
		if err := rows.Scan(&event.ID, &event.OccurredAt, &event.PrincipalID, &event.ActorTaskID, &event.ActorSessionID, &event.TargetTaskID, &event.WorkspaceID, &event.Action, &event.Capability, &event.Decision, &event.DenyReason, &event.GrantID, &event.Result, &event.Detail); err != nil {
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
