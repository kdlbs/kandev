package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kdlbs/kandev/pluginsdk"
)

var ErrUsageNotVisible = usagehistory.ErrNotVisible

type retainedTarget struct {
	Sequence             int64   `db:"sequence"`
	BindingID            string  `db:"binding_id"`
	ScopeID              string  `db:"scope_id"`
	SubjectID            string  `db:"subject_id"`
	WorkspaceID          string  `db:"workspace_id"`
	TaskID               string  `db:"task_id"`
	SessionID            string  `db:"session_id"`
	TaskIncarnationID    string  `db:"task_incarnation_id"`
	SessionIncarnationID string  `db:"session_incarnation_id"`
	ExternalSessionID    string  `db:"external_session_id"`
	SessionStartedAt     *string `db:"session_started_at"`
	SessionEndedAt       *string `db:"session_ended_at"`
	CoverageStart        *string `db:"coverage_start"`
	CoverageEnd          *string `db:"coverage_end"`
	ActorID              string  `db:"actor_id"`
	ActorStatus          string  `db:"actor_status"`
	ActorKind            string  `db:"actor_kind"`
	Fingerprint          string  `db:"fingerprint"`
	SourceScopeStatus    string  `db:"source_scope_status"`
	Forked               bool    `db:"-"`
}

func (v retainedTarget) dto() pluginsdk.UsageCollectionTargetV1 {
	kind := usageWorkFallback
	if v.SessionID != "" {
		kind = usageSubjectSession
	}
	if v.SourceScopeStatus != usageSourceEligible {
		v.ExternalSessionID = ""
		v.SessionStartedAt, v.SessionEndedAt, v.CoverageStart, v.CoverageEnd = nil, nil, nil, nil
	}
	return pluginsdk.UsageCollectionTargetV1{SubjectBinding: v.BindingID, TaskID: v.TaskID, SessionID: nonemptyPtr(v.SessionID), SourceScopeStatus: v.SourceScopeStatus,
		ExternalSessionID: nonemptyPtr(v.ExternalSessionID), SubjectKind: kind, ActorAttributionStatus: v.ActorStatus,
		SessionStartedAt: v.SessionStartedAt, SessionEndedAt: v.SessionEndedAt, CoverageStart: v.CoverageStart, CoverageEnd: v.CoverageEnd}
}

func nonemptyPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func collectorSource(id string) (string, error) {
	if strings.TrimSpace(id) != id || id == "" || len(id) > 200 {
		return "", errors.New("collector identity is required")
	}
	return "plugin:" + id, nil
}

func (r *Repository) authorizedUsageScopeTx(ctx context.Context, tx *sqlx.Tx, workspace string) (string, error) {
	if workspace == "" || !usagehistory.WorkspaceAllowed(ctx, workspace) {
		return "", ErrUsageNotVisible
	}
	var org string
	if err := tx.GetContext(ctx, &org, r.db.Rebind(`SELECT COALESCE(org_id, '') FROM workspaces WHERE id = ?`), workspace); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrUsageNotVisible
		}
		return "", err
	}
	if identity, ok := authn.IdentityFromContext(ctx); ok && !identity.Synthetic && (identity.Instance || identity.OrgID != org) {
		return "", ErrUsageNotVisible
	}
	if err := r.ensureUsageScopeWritableTx(ctx, tx, workspace, org); err != nil {
		return "", ErrUsageNotVisible
	}
	return usageScopeID(workspace, org), nil
}

// refreshUsageCollectionTargets captures live metadata in bounded transactions.
// Destructive lifecycle preservation invokes the same path before deleting it.
func (r *Repository) refreshUsageCollectionTargets(ctx context.Context, workspace, taskID, sessionID string) error {
	last := ""
	for {
		tx, err := r.db.BeginTxx(ctx, nil)
		if err != nil {
			return err
		}
		count, next, err := r.captureUsageTargetPageTx(ctx, tx, workspace, taskID, sessionID, last, usageHistoryBatchLimit)
		if err == nil {
			err = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
		if err != nil {
			return err
		}
		if count < usageHistoryBatchLimit {
			return nil
		}
		last = next
	}
}

func (r *Repository) captureUsageTargetPageTx(ctx context.Context, tx *sqlx.Tx, workspace, taskID, sessionID, last string, limit int) (int, string, error) {
	if _, err := r.authorizedUsageScopeTx(ctx, tx, workspace); err != nil {
		return 0, last, err
	}
	query := `SELECT ts.id, ts.task_id FROM task_sessions ts JOIN tasks t ON t.id = ts.task_id WHERE t.workspace_id = ? AND ts.id > ?`
	args := []any{workspace, last}
	if taskID != "" {
		query += ` AND t.id = ?`
		args = append(args, taskID)
	}
	if sessionID != "" {
		query += ` AND ts.id = ?`
		args = append(args, sessionID)
	}
	query += ` ORDER BY ts.id LIMIT ?`
	args = append(args, limit)
	var rows []struct {
		ID     string `db:"id"`
		TaskID string `db:"task_id"`
	}
	if err := tx.SelectContext(ctx, &rows, r.db.Rebind(query), args...); err != nil {
		return 0, last, err
	}
	for _, row := range rows {
		_, err := r.bindExternalUsageScopeTx(ctx, tx, models.SessionUsageMeasurement{WorkspaceID: workspace, TaskID: row.TaskID, SessionID: row.ID, CollectedAt: time.Now().UTC()})
		if err != nil {
			return 0, last, err
		}
		last = row.ID
	}
	return len(rows), last, nil
}

func (r *Repository) captureCollectionTargetTx(ctx context.Context, tx *sqlx.Tx, subjectID string) error {
	var v retainedTarget
	query := `SELECT s.scope_id, s.subject_id, sc.workspace_id, s.task_id, s.session_id, s.task_incarnation_id, s.session_incarnation_id
		FROM usage_subjects s JOIN usage_scopes sc ON sc.scope_id = s.scope_id WHERE s.subject_id = ?`
	if err := tx.GetContext(ctx, &v, r.db.Rebind(query), subjectID); err != nil {
		return err
	}
	if v.SessionID == "" {
		return nil
	}
	if err := r.captureTargetSessionTx(ctx, tx, &v); err != nil {
		return err
	}
	if err := r.captureTargetActorTx(ctx, tx, &v); err != nil {
		return err
	}
	status, err := r.refreshUsageSourceContextTx(ctx, tx, v, false)
	if err != nil {
		return err
	}
	v.SourceScopeStatus = status
	encoded, err := json.Marshal(v)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(encoded)
	v.Fingerprint = hex.EncodeToString(sum[:])
	v.BindingID = uuid.NewSHA1(uuid.NameSpaceURL, []byte("usage-collection-binding:"+v.SubjectID+":"+v.Fingerprint)).String()
	if err := r.invalidateUsageWorkContextTx(ctx, tx, v); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_collection_bindings (binding_id, scope_id, subject_id, workspace_id, task_id, session_id,
		task_incarnation_id, session_incarnation_id, external_session_id, session_started_at, session_ended_at, coverage_start, coverage_end,
		actor_id, actor_status, actor_kind, fingerprint, source_scope_status) VALUES (`+placeholders(18)+`) ON CONFLICT(subject_id, fingerprint) DO NOTHING`),
		v.BindingID, v.ScopeID, v.SubjectID, v.WorkspaceID, v.TaskID, v.SessionID, v.TaskIncarnationID, v.SessionIncarnationID,
		v.ExternalSessionID, v.SessionStartedAt, v.SessionEndedAt, v.CoverageStart, v.CoverageEnd, v.ActorID, v.ActorStatus, v.ActorKind, v.Fingerprint, v.SourceScopeStatus)
	if err != nil {
		return err
	}
	return r.indexCurrentUsageTargetTx(ctx, tx, v)
}

func (r *Repository) captureTargetSessionTx(ctx context.Context, tx *sqlx.Tx, v *retainedTarget) error {
	var start, end sql.NullString
	var raw string
	var taskCreated time.Time
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT CAST(ts.started_at AS TEXT), CAST(ts.completed_at AS TEXT), COALESCE(ts.metadata, '{}'), t.created_at
		FROM task_sessions ts JOIN tasks t ON t.id = ts.task_id WHERE ts.id = ? AND ts.task_id = ?`), v.SessionID, v.TaskID).Scan(&start, &end, &raw, &taskCreated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if usageTaskIncarnationID(taskCreated) != v.TaskIncarnationID {
		return nil
	}
	if start.Valid {
		value := parseTimeString(start.String).UTC().Format(time.RFC3339Nano)
		v.SessionStartedAt = &value
	}
	if end.Valid {
		value := parseTimeString(end.String).UTC().Format(time.RFC3339Nano)
		v.SessionEndedAt = &value
	}
	var content struct {
		ForkSource string `json:"codex_fork_source_session_id"`
		ACP        struct {
			SessionID string `json:"session_id"`
		} `json:"acp"`
	}
	if json.Unmarshal([]byte(raw), &content) == nil {
		v.ExternalSessionID = content.ACP.SessionID
		v.Forked = content.ForkSource != ""
	}
	if v.ExternalSessionID == "" {
		err = tx.GetContext(ctx, &v.ExternalSessionID, r.db.Rebind(`SELECT COALESCE(resume_token, '') FROM executors_running WHERE session_id = ? LIMIT 1`), v.SessionID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}

func (r *Repository) captureTargetActorTx(ctx context.Context, tx *sqlx.Tx, v *retainedTarget) error {
	var known, unknown int
	var actor, kind sql.NullString
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT COUNT(DISTINCT CASE WHEN actor_status = 'attributed' THEN actor_kind || ':' || actor_id END),
		COALESCE(SUM(CASE WHEN actor_status <> 'attributed' THEN 1 ELSE 0 END), 0), MAX(actor_id), MAX(actor_kind)
		FROM usage_execution_bindings WHERE scope_id = ? AND subject_id = ?`), v.ScopeID, v.SubjectID).Scan(&known, &unknown, &actor, &kind)
	if err != nil {
		return err
	}
	v.ActorID, v.ActorStatus, v.ActorKind = "", usageUnknown, usageUnknown
	if known == 1 && unknown == 0 {
		v.ActorID, v.ActorStatus, v.ActorKind = actor.String, "attributed", kind.String
	}
	if known > 1 || (known > 0 && unknown > 0) {
		v.ActorStatus = usageMixedValue
	}
	var first, last sql.NullString
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT CAST(MIN(coverage_start) AS TEXT), CAST(MAX(coverage_end) AS TEXT)
		FROM session_usage_buckets WHERE scope_id = ? AND subject_id = ? AND retired = 0`), v.ScopeID, v.SubjectID).Scan(&first, &last); err != nil {
		return err
	}
	if first.Valid {
		value := parseTimeString(first.String).UTC().Format(time.RFC3339Nano)
		v.CoverageStart = &value
	}
	if last.Valid {
		value := parseTimeString(last.String).UTC().Format(time.RFC3339Nano)
		v.CoverageEnd = &value
	}
	return nil
}

func (r *Repository) ListUsageCollectionTargets(ctx context.Context, collector string, query pluginsdk.UsageCollectionTargetQueryV1) (pluginsdk.UsageCollectionTargetPageV1, error) {
	source, err := collectorSource(collector)
	if err != nil {
		return pluginsdk.UsageCollectionTargetPageV1{}, err
	}
	if query.SnapshotID == "" && query.Cursor != "" {
		return pluginsdk.UsageCollectionTargetPageV1{}, ErrUsageNotVisible
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return pluginsdk.UsageCollectionTargetPageV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	scope, err := r.collectionQueryScopeTx(ctx, tx, query)
	if err != nil {
		return pluginsdk.UsageCollectionTargetPageV1{}, err
	}
	var page pluginsdk.UsageCollectionTargetPageV1
	if query.CurrentTarget != nil {
		page, err = r.listCurrentUsageTargetTx(ctx, tx, source, scope, query)
	} else {
		page, err = r.listUsageTargetSnapshotTx(ctx, tx, source, scope, query)
	}
	if err != nil {
		return page, err
	}
	return page, tx.Commit()
}

func usageTargetPageLimit(limit int32) int {
	if limit < 1 {
		return 50
	}
	return min(int(limit), usageHistoryBatchLimit)
}

func (r *Repository) listUsageTargetSnapshotTx(ctx context.Context, tx *sqlx.Tx, source, scope string, query pluginsdk.UsageCollectionTargetQueryV1) (pluginsdk.UsageCollectionTargetPageV1, error) {
	page := pluginsdk.UsageCollectionTargetPageV1{SnapshotID: query.SnapshotID}
	limit := usageTargetPageLimit(query.Limit)
	if query.SnapshotID == "" {
		ready, err := r.prepareUsageCollectionTargetsTx(ctx, tx, scope, query.WorkspaceID, limit)
		if err != nil {
			return page, err
		}
		if !ready {
			page.Preparing = true
			return page, nil
		}
	}
	if err := r.loadUsageTargetSnapshotTx(ctx, tx, source, scope, query.WorkspaceID, &page); err != nil {
		return page, err
	}
	last, err := decodeUsageTargetCursor(query.Cursor, page.SnapshotID, page.SnapshotUpperBound)
	if err != nil {
		return page, err
	}
	var targets []retainedTarget
	err = tx.SelectContext(ctx, &targets, r.db.Rebind(`SELECT b.* FROM usage_collection_bindings b WHERE b.scope_id = ? AND b.sequence > ? AND b.sequence <= ?
		AND NOT EXISTS (SELECT 1 FROM usage_collection_bindings n WHERE n.subject_id = b.subject_id AND n.sequence > b.sequence AND n.sequence <= ?)
		ORDER BY b.sequence LIMIT ?`), scope, last, page.SnapshotUpperBound, page.SnapshotUpperBound, limit+1)
	if err != nil {
		return page, err
	}
	page.HasMore = len(targets) > limit
	if page.HasMore {
		targets = targets[:limit]
	}
	last, err = r.grantUsageTargetsTx(ctx, tx, source, targets, &page, last)
	if err != nil {
		return page, err
	}
	if page.HasMore {
		page.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(page.SnapshotID + ":" + strconv.FormatInt(last, 10)))
	}
	return page, nil
}

func decodeUsageTargetCursor(cursor, snapshot string, upper int64) (int64, error) {
	if cursor == "" {
		return 0, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, ErrUsageNotVisible
	}
	prefix := snapshot + ":"
	if !strings.HasPrefix(string(decoded), prefix) {
		return 0, ErrUsageNotVisible
	}
	value, err := strconv.ParseInt(strings.TrimPrefix(string(decoded), prefix), 10, 64)
	if err != nil || value < 0 || value > upper {
		return 0, ErrUsageNotVisible
	}
	return value, nil
}

func (r *Repository) resolveCollectionBindingTx(ctx context.Context, tx *sqlx.Tx, source, binding string) (retainedTarget, error) {
	var target retainedTarget
	err := tx.GetContext(ctx, &target, r.db.Rebind(`SELECT b.* FROM usage_collection_bindings b JOIN usage_collection_grants g ON g.binding_id = b.binding_id
		WHERE b.binding_id = ? AND g.source = ?`), binding, source)
	if err != nil {
		return target, ErrUsageNotVisible
	}
	scope, err := r.authorizedUsageScopeTx(ctx, tx, target.WorkspaceID)
	if err != nil || scope != target.ScopeID {
		return target, ErrUsageNotVisible
	}
	return target, nil
}

func (r *Repository) loadUsageTargetSnapshotTx(ctx context.Context, tx *sqlx.Tx, source, scope, workspace string, page *pluginsdk.UsageCollectionTargetPageV1) error {
	var err error
	if page.SnapshotID == "" {
		page.SnapshotID = uuid.NewString()
		if err := tx.GetContext(ctx, &page.SnapshotUpperBound, r.db.Rebind(`SELECT COALESCE(MAX(sequence), 0) FROM usage_collection_bindings WHERE scope_id = ?`), scope); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_collection_snapshots (snapshot_id, source, workspace_id, scope_id, upper_bound) VALUES (?, ?, ?, ?, ?)`), page.SnapshotID, source, workspace, scope, page.SnapshotUpperBound)
		if err == nil {
			_, err = tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_collection_snapshots WHERE source = ? AND scope_id = ? AND snapshot_id <> ?`), source, scope, page.SnapshotID)
		}
	} else {
		err = tx.GetContext(ctx, &page.SnapshotUpperBound, r.db.Rebind(`SELECT upper_bound FROM usage_collection_snapshots WHERE snapshot_id = ? AND source = ? AND workspace_id = ? AND scope_id = ?`), page.SnapshotID, source, workspace, scope)
	}
	if err != nil {
		return ErrUsageNotVisible
	}
	return nil
}

func (r *Repository) grantUsageTargetsTx(ctx context.Context, tx *sqlx.Tx, source string, targets []retainedTarget, page *pluginsdk.UsageCollectionTargetPageV1, last int64) (int64, error) {
	for _, target := range targets {
		page.Targets = append(page.Targets, target.dto())
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_collection_grants (source, binding_id, scope_id) VALUES (?, ?, ?) ON CONFLICT(source, binding_id) DO NOTHING`), source, target.BindingID, target.ScopeID); err != nil {
			return last, err
		}
		last = target.Sequence
	}
	return last, nil
}
