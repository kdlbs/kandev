package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
)

type legacyExternalUsageRow struct {
	ID          string `db:"id"`
	WorkspaceID string `db:"workspace_id"`
	TaskID      string `db:"task_id"`
	SessionID   string `db:"session_id"`
	ScopeID     string `db:"scope_id"`
	SubjectID   string `db:"subject_id"`
	ActorID     string `db:"actor_id"`
	ActorStatus string `db:"actor_status"`
	ActorKind   string `db:"actor_kind"`
	BindingID   string `db:"usage_binding_id"`
	CoverageKey string `db:"coverage_key"`
	ObservedAt  string `db:"observed_at"`
	CollectedAt string `db:"collected_at"`
}

func (r *Repository) retainLegacyExternalUsageTx(ctx context.Context, tx *sqlx.Tx, table string, row legacyExternalUsageRow, target usagePreservationTarget) error {
	if row.ScopeID != "" && row.SubjectID != "" {
		return r.retainCapturedLegacyExternalUsageTx(ctx, tx, table, row)
	}
	if row.WorkspaceID == "" {
		return errors.New("legacy external usage has no validated workspace")
	}
	orgID, resolved, err := r.resolveLegacyUsageOrganizationTx(ctx, tx, table, row)
	if err != nil || !resolved {
		return err
	}
	if err := r.ensureUsageScopeWritableTx(ctx, tx, row.WorkspaceID, orgID); err != nil {
		return err
	}
	scopeID := row.ScopeID
	if scopeID == "" {
		scopeID = usageScopeID(row.WorkspaceID, orgID)
	}
	subject, err := r.resolveLegacyUsageSubjectTx(ctx, tx, row, scopeID)
	if err != nil {
		return err
	}
	if err := ensureUsageScopeTx(ctx, tx, r.db, scopeID, row.WorkspaceID, orgID, subject.CreatedAt); err != nil {
		return err
	}
	if err := ensureUsageSubjectTx(ctx, tx, r.db, subject.ID, scopeID, row.TaskID, subject.TaskIncarnationID, subject.SessionID, subject.SessionIncarnationID, subject.CreatedAt); err != nil {
		return err
	}
	row, err = r.resolveLegacyUsageActorTx(ctx, tx, row, scopeID, subject.ID)
	if err != nil {
		return err
	}
	return r.retainLegacyExternalSourceTx(ctx, tx, table, row, scopeID, subject)
}

func (r *Repository) retainLegacyExternalSourceTx(ctx context.Context, tx *sqlx.Tx, table string, row legacyExternalUsageRow, scopeID string, subject legacyUsageSubject) error {
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE `+table+` SET scope_id = ?, subject_id = ?, usage_binding_id = ?, actor_id = ?, actor_status = ?, actor_kind = ?
		WHERE id = ? AND (scope_id = '' OR subject_id = '')`), scopeID, subject.ID, row.BindingID, row.ActorID, row.ActorStatus, row.ActorKind, row.ID); err != nil {
		return err
	}
	if err := r.insertUsageLiveLinkTx(ctx, tx, table, row.ID, scopeID, subject.ID, row.TaskID, subject.SessionID); err != nil {
		return err
	}
	if err := r.indexRetainedSourceRowTx(ctx, tx, table, row.ID); err != nil {
		return err
	}
	partition := row.CoverageKey
	if partition == "" {
		partition = usageLifetime
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_dirty_partitions (scope_id, subject_id, partition_key, updated_at)
		VALUES (?, ?, ?, ?) ON CONFLICT(scope_id, subject_id, partition_key) DO UPDATE SET updated_at = excluded.updated_at`), scopeID, subject.ID, partition, subject.CreatedAt)
	return err
}

func (r *Repository) retainCapturedLegacyExternalUsageTx(ctx context.Context, tx *sqlx.Tx, table string, row legacyExternalUsageRow) error {
	var workspaceID, organizationID string
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT workspace_id, organization_id FROM usage_scopes WHERE scope_id = ?`), row.ScopeID).Scan(&workspaceID, &organizationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return r.recordUsageMigrationGapTx(ctx, tx, table, row.ID, row.TaskID, row.WorkspaceID, "", "captured scope is unavailable")
		}
		return err
	}
	if workspaceID != row.WorkspaceID {
		return errors.New("legacy external usage conflicts with captured workspace ownership")
	}
	if err := r.ensureUsageScopeWritableTx(ctx, tx, workspaceID, organizationID); err != nil {
		return err
	}
	if err := r.indexRetainedSourceRowTx(ctx, tx, table, row.ID); err != nil {
		return err
	}
	return r.ensureLegacyUsageLiveLinkTx(ctx, tx, table, row.ID, row.ScopeID, row.SubjectID, row.TaskID, row.SessionID)
}

func (r *Repository) resolveLegacyUsageOrganizationTx(ctx context.Context, tx *sqlx.Tx, table string, row legacyExternalUsageRow) (string, bool, error) {
	if row.ScopeID == "" {
		var organizationID sql.NullString
		if err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT org_id FROM workspaces WHERE id = ?`), row.WorkspaceID).Scan(&organizationID); err != nil {
			return "", false, fmt.Errorf("resolve legacy workspace ownership: %w", err)
		}
		if organizationID.Valid {
			return organizationID.String, true, nil
		}
		return "", true, nil
	}
	var capturedWorkspace, organizationID string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT workspace_id, organization_id FROM usage_scopes WHERE scope_id = ?`), row.ScopeID).Scan(&capturedWorkspace, &organizationID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, r.recordUsageMigrationGapTx(ctx, tx, table, row.ID, row.TaskID, row.WorkspaceID, "", "captured scope is unavailable")
	}
	if err != nil {
		return "", false, err
	}
	if capturedWorkspace != row.WorkspaceID {
		return organizationID, false, r.recordUsageMigrationGapTx(ctx, tx, table, row.ID, row.TaskID, row.WorkspaceID, organizationID, "source workspace conflicts with captured scope")
	}
	return organizationID, true, nil
}

type legacyUsageSubject struct {
	ID                   string
	TaskIncarnationID    string
	SessionID            string
	SessionIncarnationID string
	CreatedAt            time.Time
}

func (r *Repository) resolveLegacyUsageSubjectTx(ctx context.Context, tx *sqlx.Tx, row legacyExternalUsageRow, scopeID string) (legacyUsageSubject, error) {
	var subject legacyUsageSubject
	var taskCreatedAt time.Time
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT created_at FROM tasks WHERE id = ?`), row.TaskID).Scan(&taskCreatedAt); err != nil {
		return subject, errors.New("legacy external usage has no live task incarnation")
	}
	collectedAt := parseTimeString(row.CollectedAt)
	if !collectedAt.IsZero() && taskCreatedAt.After(collectedAt) {
		return subject, errors.New("legacy external usage predates the current task incarnation")
	}
	var err error
	subject.SessionID, subject.SessionIncarnationID, err = r.resolveLegacyUsageSessionTx(ctx, tx, row, collectedAt)
	if err != nil {
		return subject, err
	}
	subject.TaskIncarnationID = usageTaskIncarnationID(taskCreatedAt)
	subject.ID = row.SubjectID
	if subject.ID == "" {
		subject.ID = usageSubjectID(scopeID, row.TaskID, subject.TaskIncarnationID, subject.SessionID, subject.SessionIncarnationID)
	}
	subject.CreatedAt = collectedAt
	if subject.CreatedAt.IsZero() {
		subject.CreatedAt = taskCreatedAt.UTC()
	}
	return subject, nil
}

func (r *Repository) resolveLegacyUsageSessionTx(ctx context.Context, tx *sqlx.Tx, row legacyExternalUsageRow, collectedAt time.Time) (string, string, error) {
	if row.SessionID == "" {
		return "", "", nil
	}
	var incarnationID string
	var startedAt time.Time
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT COALESCE(NULLIF(queue_incarnation_id, ''), id), started_at FROM task_sessions WHERE id = ? AND task_id = ?`), row.SessionID, row.TaskID).Scan(&incarnationID, &startedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	if !collectedAt.IsZero() && startedAt.After(collectedAt) {
		return "", "", nil
	}
	return row.SessionID, incarnationID, nil
}

func (r *Repository) resolveLegacyUsageActorTx(ctx context.Context, tx *sqlx.Tx, row legacyExternalUsageRow, scopeID, subjectID string) (legacyExternalUsageRow, error) {
	if row.ActorStatus == "" || row.ActorStatus == models.UsageAttributionUnknown || row.ScopeID == "" {
		row.ActorID, row.ActorStatus, row.ActorKind, row.BindingID = "", models.UsageAttributionUnknown, usageUnknown, ""
	}
	if row.ActorStatus != models.UsageAttributionAttributed || row.BindingID == "" {
		return row, nil
	}
	var bindingExists bool
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT EXISTS(SELECT 1 FROM usage_execution_bindings WHERE binding_id = ? AND scope_id = ? AND subject_id = ?)`), row.BindingID, scopeID, subjectID).Scan(&bindingExists); err != nil {
		return row, err
	}
	if !bindingExists {
		row.ActorID, row.ActorStatus, row.ActorKind, row.BindingID = "", models.UsageAttributionUnknown, usageUnknown, ""
	}
	return row, nil
}

func (r *Repository) ensureLegacyUsageLiveLinkTx(ctx context.Context, tx *sqlx.Tx, table, id, scopeID, subjectID, taskID, sessionID string) error {
	var exists bool
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT EXISTS(SELECT 1 FROM usage_live_links WHERE source_table = ? AND observation_id = ?)`), table, id).Scan(&exists); err != nil || exists {
		return err
	}
	var workspaceID string
	var taskCreatedAt, subjectCreatedAt time.Time
	var taskIncarnationID, subjectTaskIncarnationID, subjectSessionID, subjectSessionIncarnationID string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT t.workspace_id, t.created_at, s.created_at, s.task_incarnation_id,
		s.session_id, s.session_incarnation_id
		FROM tasks t JOIN usage_subjects s ON s.task_id = t.id
		WHERE t.id = ? AND s.subject_id = ? AND s.scope_id = ?`), taskID, subjectID, scopeID).
		Scan(&workspaceID, &taskCreatedAt, &subjectCreatedAt, &subjectTaskIncarnationID, &subjectSessionID, &subjectSessionIncarnationID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var scopeWorkspace string
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT workspace_id FROM usage_scopes WHERE scope_id = ?`), scopeID).Scan(&scopeWorkspace); err != nil {
		return err
	}
	taskIncarnationID = usageTaskIncarnationID(taskCreatedAt)
	if workspaceID != scopeWorkspace || taskCreatedAt.After(subjectCreatedAt) ||
		(subjectTaskIncarnationID != "" && subjectTaskIncarnationID != taskIncarnationID) {
		return nil
	}
	liveSessionID, err := r.legacyUsageLiveSessionTx(ctx, tx, taskID, sessionID, subjectSessionID, subjectSessionIncarnationID)
	if err != nil {
		return err
	}
	return r.insertUsageLiveLinkTx(ctx, tx, table, id, scopeID, subjectID, taskID, liveSessionID)
}

func (r *Repository) insertUsageLiveLinkTx(ctx context.Context, tx *sqlx.Tx, table, id, scopeID, subjectID, taskID, sessionID string) error {
	var taskCreatedAt, subjectCreatedAt time.Time
	var workspaceID, scopeWorkspace, taskIncarnationID, subjectSessionID, subjectSessionIncarnationID string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT t.workspace_id, t.created_at, s.created_at, s.task_incarnation_id, u.workspace_id,
		s.session_id, s.session_incarnation_id
		FROM tasks t JOIN usage_subjects s ON s.task_id = t.id JOIN usage_scopes u ON u.scope_id = s.scope_id
		WHERE t.id = ? AND s.subject_id = ? AND s.scope_id = ?`), taskID, subjectID, scopeID).
		Scan(&workspaceID, &taskCreatedAt, &subjectCreatedAt, &taskIncarnationID, &scopeWorkspace, &subjectSessionID, &subjectSessionIncarnationID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if workspaceID != scopeWorkspace || taskCreatedAt.After(subjectCreatedAt) ||
		(taskIncarnationID != "" && taskIncarnationID != usageTaskIncarnationID(taskCreatedAt)) {
		return nil
	}
	sessionID, err = r.legacyUsageLiveSessionTx(ctx, tx, taskID, sessionID, subjectSessionID, subjectSessionIncarnationID)
	if err != nil {
		return err
	}
	var liveSession any
	if sessionID != "" {
		liveSession = sessionID
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_live_links (source_table, observation_id, scope_id, subject_id, live_task_id, live_session_id)
		VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(source_table, observation_id) DO NOTHING`), table, id, scopeID, subjectID, taskID, liveSession)
	return err
}

func (r *Repository) legacyUsageLiveSessionTx(ctx context.Context, tx *sqlx.Tx, taskID, sessionID, subjectSessionID, subjectSessionIncarnationID string) (string, error) {
	if sessionID == "" || subjectSessionID != sessionID || subjectSessionIncarnationID == "" {
		return "", nil
	}
	var liveIncarnation string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT COALESCE(NULLIF(queue_incarnation_id, ''), id)
		FROM task_sessions WHERE id = ? AND task_id = ?`), sessionID, taskID).Scan(&liveIncarnation)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", nil
	case err != nil:
		return "", err
	case liveIncarnation != subjectSessionIncarnationID:
		return "", nil
	default:
		return sessionID, nil
	}
}
