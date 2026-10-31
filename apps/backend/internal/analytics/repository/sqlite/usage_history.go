package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
)

var _ usagehistory.Retainer = (*Repository)(nil)

func (r *Repository) AdmitBindingTx(ctx context.Context, tx *sqlx.Tx, binding usagehistory.Binding) error {
	if tx == nil {
		return errors.New("usage history admission requires a transaction")
	}
	if err := validateBinding(binding); err != nil {
		return err
	}
	createdAt := binding.CapturedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	scopeID := usageScopeID(binding.WorkspaceID, binding.OrganizationID)
	subjectID := usageSubjectID(scopeID, binding.TaskID, binding.TaskIncarnationID, binding.SessionID, binding.SessionIncarnationID)
	if err := r.ensureUsageScopeWritableTx(ctx, tx, binding.WorkspaceID, binding.OrganizationID); err != nil {
		return err
	}
	if err := ensureUsageScopeTx(ctx, tx, r.db, scopeID, binding.WorkspaceID, binding.OrganizationID, createdAt); err != nil {
		return err
	}
	if err := ensureUsageSubjectTx(ctx, tx, r.db, subjectID, scopeID, binding.TaskID, binding.TaskIncarnationID, binding.SessionID, binding.SessionIncarnationID, createdAt); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO usage_execution_bindings (
			binding_id, binding_kind, scope_id, subject_id, workspace_id, task_id, task_incarnation_id,
			session_id, session_incarnation_id, turn_id, execution_id, parent_binding_id,
			actor_id, actor_status, actor_kind, actor_source, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(binding_id) DO NOTHING
	`), binding.ID, binding.Kind, scopeID, subjectID, binding.WorkspaceID, binding.TaskID, binding.TaskIncarnationID,
		binding.SessionID, binding.SessionIncarnationID, binding.TurnID, binding.ExecutionID,
		binding.ParentBindingID, binding.ActorID, binding.ActorStatus, binding.ActorKind,
		binding.ActorSource, createdAt)
	if err != nil {
		return fmt.Errorf("insert retained usage binding: %w", err)
	}
	return r.verifyUsageBindingTx(ctx, tx, binding, scopeID, subjectID)
}

func (r *Repository) RetainNativeUsageTx(ctx context.Context, tx *sqlx.Tx, native usagehistory.NativeUsage) error {
	if tx == nil {
		return errors.New("native usage retention requires a transaction")
	}
	if native.UsageEventID == "" || native.TaskID == "" {
		return errors.New("native usage event id and task id are required")
	}
	var scope trustedUsageScope
	var err error
	if native.BindingID == "" {
		native.BindingID, err = r.savedNativeBindingTx(ctx, tx, native)
		if err != nil {
			return err
		}
	}
	if native.BindingID == "" {
		native.BindingID, err = r.adoptUnboundNativeTx(ctx, tx, native)
		if errors.Is(err, errUsageScopeUnproven) {
			return r.recordUnprovenNativeScopeTx(ctx, tx, native)
		}
		if err != nil {
			return err
		}
	}
	if native.BindingID != "" {
		scope, err = r.resolveNativeBindingScopeTx(ctx, tx, native)
	} else {
		scope, err = r.resolveNativeScopeTx(ctx, tx, native)
	}
	if err != nil {
		if errors.Is(err, errUsageScopeUnproven) {
			return r.recordUnprovenNativeScopeTx(ctx, tx, native)
		}
		return err
	}
	return r.persistScopedNativeUsageTx(ctx, tx, native, scope)
}

func (r *Repository) persistScopedNativeUsageTx(ctx context.Context, tx *sqlx.Tx, native usagehistory.NativeUsage, scope trustedUsageScope) error {
	binding, err := r.admitNativeUsageBindingTx(ctx, tx, &native, scope)
	if err != nil {
		return err
	}
	measurement, err := normalizeRetainedNativeUsage(native, scope, binding)
	if err != nil {
		return err
	}
	if err := r.freezeNativeTranscriptTx(ctx, tx, &measurement); err != nil {
		return err
	}
	if err := r.registerNativeWorkIdentitiesTx(ctx, tx, native, measurement); err != nil {
		return err
	}
	measurement.PayloadDigest = nativeUsageDigest(measurement)
	if err := r.insertRetainedNativeUsageTx(ctx, tx, measurement); err != nil {
		return err
	}
	if err := r.insertUsageLiveLinkTx(ctx, tx, usageBucketsTable, measurement.ID, measurement.ScopeID,
		measurement.SubjectID, measurement.TaskID, measurement.SessionID); err != nil {
		return fmt.Errorf("link retained native usage event: %w", err)
	}
	return nil
}

func (r *Repository) resolveNativeBindingScopeTx(ctx context.Context, tx *sqlx.Tx, native usagehistory.NativeUsage) (trustedUsageScope, error) {
	var scope trustedUsageScope
	var binding struct {
		ScopeID              string `db:"scope_id"`
		SubjectID            string `db:"subject_id"`
		WorkspaceID          string `db:"workspace_id"`
		OrganizationID       string `db:"organization_id"`
		TaskID               string `db:"task_id"`
		TaskIncarnationID    string `db:"task_incarnation_id"`
		SessionID            string `db:"session_id"`
		SessionIncarnationID string `db:"session_incarnation_id"`
		TurnID               string `db:"turn_id"`
	}
	err := tx.GetContext(ctx, &binding, r.db.Rebind(`SELECT b.scope_id, b.subject_id, b.workspace_id, s.organization_id,
		b.task_id, b.task_incarnation_id, b.session_id, b.session_incarnation_id, b.turn_id
		FROM usage_execution_bindings b JOIN usage_scopes s ON s.scope_id = b.scope_id
		JOIN usage_subjects u ON u.subject_id = b.subject_id AND u.scope_id = b.scope_id
			AND u.task_id = b.task_id AND u.task_incarnation_id = b.task_incarnation_id
			AND u.session_id = b.session_id AND u.session_incarnation_id = b.session_incarnation_id
		WHERE b.binding_id = ?`), native.BindingID)
	if err != nil {
		return scope, fmt.Errorf("resolve retained native binding %q: %w", native.BindingID, err)
	}
	if binding.TaskID != native.TaskID || (native.SessionID != "" && binding.SessionID != native.SessionID) ||
		(native.TurnID != "" && binding.TurnID != native.TurnID) {
		return scope, fmt.Errorf("native usage event %q does not match retained binding %q", native.UsageEventID, native.BindingID)
	}
	scope = trustedUsageScope{
		WorkspaceID: binding.WorkspaceID, OrganizationID: binding.OrganizationID,
		ScopeID: binding.ScopeID, SubjectID: binding.SubjectID,
		TaskIncarnationID: binding.TaskIncarnationID, SessionID: binding.SessionID,
		SessionIncarnationID: binding.SessionIncarnationID,
	}
	if err := r.ensureUsageScopeWritableTx(ctx, tx, scope.WorkspaceID, scope.OrganizationID); err != nil {
		return trustedUsageScope{}, err
	}
	return scope, nil
}

type trustedUsageScope struct {
	WorkspaceID          string
	OrganizationID       string
	ScopeID              string
	SubjectID            string
	TaskIncarnationID    string
	SessionID            string
	SessionIncarnationID string
}

func (r *Repository) resolveNativeScopeTx(ctx context.Context, tx *sqlx.Tx, native usagehistory.NativeUsage) (trustedUsageScope, error) {
	var scope trustedUsageScope
	var orgID sql.NullString
	var sessionID, incarnationID string
	var taskCreatedAt time.Time
	if native.SessionID != "" {
		err := tx.QueryRowxContext(ctx, r.db.Rebind(`
			SELECT t.workspace_id, w.org_id, ts.id, COALESCE(NULLIF(ts.queue_incarnation_id, ''), ts.id), t.created_at
			FROM tasks t JOIN workspaces w ON w.id = t.workspace_id
			JOIN task_sessions ts ON ts.id = ? AND ts.task_id = t.id
			WHERE t.id = ?`), native.SessionID, native.TaskID).Scan(&scope.WorkspaceID, &orgID, &sessionID, &incarnationID, &taskCreatedAt)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return scope, fmt.Errorf("resolve native usage session scope: %w", err)
		}
		if err == nil {
			scope.SessionID, scope.SessionIncarnationID = sessionID, incarnationID
		}
	}
	if scope.WorkspaceID == "" {
		err := tx.QueryRowxContext(ctx, r.db.Rebind(`
			SELECT t.workspace_id, w.org_id, t.created_at FROM tasks t JOIN workspaces w ON w.id = t.workspace_id WHERE t.id = ?`),
			native.TaskID).Scan(&scope.WorkspaceID, &orgID, &taskCreatedAt)
		if err != nil {
			return scope, fmt.Errorf("resolve native usage task scope: %w", err)
		}
	}
	if orgID.Valid {
		scope.OrganizationID = orgID.String
	}
	scope.ScopeID = usageScopeID(scope.WorkspaceID, scope.OrganizationID)
	scope.TaskIncarnationID = usageTaskIncarnationID(taskCreatedAt)
	scope.SubjectID = usageSubjectID(scope.ScopeID, native.TaskID, scope.TaskIncarnationID, scope.SessionID, scope.SessionIncarnationID)
	if err := r.proveUnboundNativeScopeTx(ctx, tx, native, scope); err != nil {
		return scope, err
	}
	createdAt := native.OccurredAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	if err := ensureUsageScopeTx(ctx, tx, r.db, scope.ScopeID, scope.WorkspaceID, scope.OrganizationID, createdAt); err != nil {
		return scope, err
	}
	if err := ensureUsageSubjectTx(ctx, tx, r.db, scope.SubjectID, scope.ScopeID, native.TaskID, scope.TaskIncarnationID, scope.SessionID, scope.SessionIncarnationID, createdAt); err != nil {
		return scope, err
	}
	return scope, nil
}

func (r *Repository) resolveNativeBindingTx(ctx context.Context, tx *sqlx.Tx, native usagehistory.NativeUsage, scope trustedUsageScope) (usagehistory.Binding, error) {
	if native.TurnID == "" {
		return unknownNativeUsageBinding(native, scope), nil
	}
	var bindingID string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT usage_binding_id FROM task_session_turns WHERE id = ? AND task_id = ? AND task_session_id = ?`),
		native.TurnID, native.TaskID, native.SessionID).Scan(&bindingID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return usagehistory.Binding{}, err
	}
	if err != nil || bindingID == "" {
		return unknownNativeUsageBinding(native, scope), nil
	}
	return r.readNativeTurnBindingTx(ctx, tx, bindingID, native, scope)
}

func unknownNativeUsageBinding(native usagehistory.NativeUsage, scope trustedUsageScope) usagehistory.Binding {
	turnID := native.TurnID
	if scope.SessionID == "" {
		turnID = ""
	}
	seedParts := []string{native.TaskID, scope.SessionID, turnID}
	if turnID == "" {
		seedParts = append(seedParts, native.UsageEventID)
	}
	seed := strings.Join(seedParts, "\x00")
	bindingID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("kandev-usage-unknown:"+seed)).String()
	return usagehistory.Binding{
		ID: bindingID, Kind: usagehistory.BindingKindTurn, WorkspaceID: scope.WorkspaceID,
		OrganizationID: scope.OrganizationID, TaskID: native.TaskID, TaskIncarnationID: scope.TaskIncarnationID, SessionID: scope.SessionID,
		SessionIncarnationID: scope.SessionIncarnationID, TurnID: turnID,
		ActorStatus: usagehistory.ActorStatusUnknown, ActorKind: usagehistory.ActorKindUnknown,
		ActorSource: "unknown_native_admission", CapturedAt: native.OccurredAt,
	}
}

func validateBinding(binding usagehistory.Binding) error {
	if binding.ID == "" || binding.WorkspaceID == "" || binding.TaskID == "" || binding.Kind == "" {
		return errors.New("usage binding id, kind, workspace, and task are required")
	}
	if binding.Kind != usagehistory.BindingKindExecution && binding.Kind != usagehistory.BindingKindTurn {
		return fmt.Errorf("unsupported usage binding kind %q", binding.Kind)
	}
	switch binding.ActorStatus {
	case usagehistory.ActorStatusAttributed:
		if binding.ActorID == "" || (binding.ActorKind != usagehistory.ActorKindUser && binding.ActorKind != usagehistory.ActorKindSynthetic && binding.ActorKind != usagehistory.ActorKindService) {
			return errors.New("attributed usage binding requires a supported actor id and kind")
		}
	case usagehistory.ActorStatusUnknown, usagehistory.ActorStatusMixed:
		if binding.ActorID != "" {
			return errors.New("unknown or mixed usage binding cannot claim a single actor id")
		}
	default:
		return fmt.Errorf("unsupported usage actor status %q", binding.ActorStatus)
	}
	return nil
}

func ensureUsageScopeTx(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, id, workspaceID, organizationID string, createdAt time.Time) error {
	_, err := tx.ExecContext(ctx, db.Rebind(`INSERT INTO usage_scopes (scope_id, workspace_id, organization_id, created_at)
		VALUES (?, ?, ?, ?) ON CONFLICT(scope_id) DO NOTHING`), id, workspaceID, organizationID, createdAt)
	if err != nil {
		return fmt.Errorf("insert retained usage scope: %w", err)
	}
	var storedWorkspace, storedOrganization string
	if err := tx.QueryRowxContext(ctx, db.Rebind(`SELECT workspace_id, organization_id FROM usage_scopes WHERE scope_id = ?`), id).Scan(&storedWorkspace, &storedOrganization); err != nil {
		return fmt.Errorf("read retained usage scope: %w", err)
	}
	if storedWorkspace != workspaceID || storedOrganization != organizationID {
		return fmt.Errorf("retained usage scope %q conflicts with trusted workspace ownership", id)
	}
	return nil
}

func ensureUsageSubjectTx(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, id, scopeID, taskID, taskIncarnationID, sessionID, incarnationID string, createdAt time.Time) error {
	_, err := tx.ExecContext(ctx, db.Rebind(`INSERT INTO usage_subjects (subject_id, scope_id, task_id, task_incarnation_id, session_id, session_incarnation_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(subject_id) DO NOTHING`), id, scopeID, taskID, taskIncarnationID, sessionID, incarnationID, createdAt)
	if err != nil {
		return fmt.Errorf("insert retained usage subject: %w", err)
	}
	var storedScope, storedTask, storedTaskIncarnation, storedSession, storedIncarnation string
	if err := tx.QueryRowxContext(ctx, db.Rebind(`SELECT scope_id, task_id, task_incarnation_id, session_id, session_incarnation_id FROM usage_subjects WHERE subject_id = ?`), id).
		Scan(&storedScope, &storedTask, &storedTaskIncarnation, &storedSession, &storedIncarnation); err != nil {
		return fmt.Errorf("read retained usage subject: %w", err)
	}
	if storedScope != scopeID || storedTask != taskID || storedTaskIncarnation != taskIncarnationID || storedSession != sessionID || storedIncarnation != incarnationID {
		return fmt.Errorf("retained usage subject %q conflicts with trusted identity", id)
	}
	return nil
}

func usageScopeID(workspaceID, organizationID string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("kandev-usage-scope\x00"+workspaceID+"\x00"+organizationID)).String()
}

func usageTaskIncarnationID(createdAt time.Time) string {
	if createdAt.IsZero() {
		return ""
	}
	return createdAt.UTC().Format(time.RFC3339Nano)
}

func usageSubjectID(scopeID, taskID, taskIncarnationID, sessionID, incarnationID string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("kandev-usage-subject\x00"+scopeID+"\x00"+taskID+"\x00"+taskIncarnationID+"\x00"+sessionID+"\x00"+incarnationID)).String()
}

func int64Pointer(value int64) *int64 { return &value }

func nativeValuesNonNegative(values ...*int64) bool {
	for _, value := range values {
		if value != nil && *value < 0 {
			return false
		}
	}
	return true
}

func tokenValuesFitTotal(input int64, output, cacheRead, cacheWrite, reasoning, reasoningOutput *int64, total int64) error {
	for _, value := range []*int64{output, cacheRead, cacheWrite, reasoning, reasoningOutput} {
		if value == nil {
			continue
		}
		if *value > total || total < 0 {
			return errors.New("native token categories exceed total_tokens")
		}
	}
	if input > total {
		return errors.New("native input tokens exceed total_tokens")
	}
	return nil
}

func nativeUsageDigest(item models.SessionUsageMeasurement) string {
	values := []string{item.TaskID, item.SessionID, item.UsageBindingID, item.Model, item.Provider,
		item.AgentClient, item.SourceVersion, item.ObservedAt.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano),
		item.NativeScope, item.MeasurementSource, item.UsageCompleteness,
		item.PricingCatalogVersion, strconv.Itoa(item.UsageSchemaVersion), strconv.Itoa(item.TokenSemanticsVersion),
		strconv.FormatInt(valueOrZero(item.InputTokens), 10), strconv.FormatInt(valueOrZero(item.TotalTokens), 10),
		strconv.FormatInt(valueOrZero(item.CostSubcents), 10), item.CostBasis,
		strconv.FormatBool(item.ReasoningInsideOutput), strconv.FormatBool(item.CacheReadInsideInput)}
	for _, value := range []*int64{item.OutputTokens, item.CacheReadTokens, item.CacheWriteTokens,
		item.ReasoningTokens, item.ReasoningOutputTokens, item.SourceTotalTokens, item.ReportedCacheWriteTokens} {
		if value == nil {
			values = append(values, "null")
		} else {
			values = append(values, strconv.FormatInt(*value, 10))
		}
	}
	digest := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return hex.EncodeToString(digest[:])
}

func valueOrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
