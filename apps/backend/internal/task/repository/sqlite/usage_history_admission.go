package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kandev/kandev/internal/task/models"
)

type usageAdmissionScope struct {
	TaskID               string    `db:"task_id"`
	WorkspaceID          string    `db:"workspace_id"`
	OrganizationID       string    `db:"organization_id"`
	TaskCreatedAt        time.Time `db:"task_created_at"`
	SessionIncarnationID string    `db:"session_incarnation_id"`
	ExecutionID          string    `db:"execution_id"`
	ExecutionBindingID   string    `db:"execution_binding_id"`
	ExecutionActorID     string    `db:"execution_actor_id"`
	ExecutionActorStatus string    `db:"execution_actor_status"`
	ExecutionActorKind   string    `db:"execution_actor_kind"`
	ExecutionActorSource string    `db:"execution_actor_source"`
}

// SetUsageHistoryRetainer wires the analytics-owned persistence contract into
// the task repository without making either repository import the other.
const (
	usageAccountingUnknown          = "unknown"
	usageCostSourceProviderReported = "provider_reported"
)

func (r *Repository) SetUsageHistoryRetainer(retainer usagehistory.Retainer) {
	r.usageHistoryRetainer = retainer
	r.usageHistoryLifecycle, _ = retainer.(usagehistory.Lifecycle)
}

func (r *Repository) captureUsageCollectionSessionTx(ctx context.Context, exec taskSessionExecutor, taskID, sessionID string) error {
	writer, ok := r.usageHistoryRetainer.(usagehistory.CollectionSessionWriter)
	if !ok {
		return nil
	}
	tx, ok := exec.(*sqlx.Tx)
	if !ok {
		return fmt.Errorf("usage collection session creation requires a transaction")
	}
	return writer.CaptureUsageCollectionSessionTx(ctx, tx, taskID, sessionID)
}

// ResetWorkspaceUsageHistory clears retained rows for E2E fixtures after
// normal task deletion has exercised its preservation path.
func (r *Repository) ResetWorkspaceUsageHistory(ctx context.Context, workspaceID string) error {
	resetter, ok := r.usageHistoryRetainer.(usagehistory.E2EResetter)
	if !ok {
		return nil
	}
	return resetter.ResetWorkspaceUsageHistory(ctx, workspaceID)
}

func (r *Repository) bindTurnUsageTx(ctx context.Context, tx *sqlx.Tx, turn *models.Turn) error {
	if r.usageHistoryRetainer == nil {
		return nil
	}
	scope, err := r.readUsageAdmissionScopeTx(ctx, tx, turn.TaskID, turn.TaskSessionID)
	if err != nil {
		return err
	}
	binding := turnUsageBinding(ctx, turn, scope)
	if err := r.usageHistoryRetainer.AdmitBindingTx(ctx, tx, binding); err != nil {
		return fmt.Errorf("admit retained turn binding: %w", err)
	}
	turn.UsageBindingID = binding.ID
	return nil
}

func (r *Repository) admitExecutionUsageTx(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID, actorID string,
) (string, error) {
	if r.usageHistoryRetainer == nil || sessionID == "" {
		return "", nil
	}
	var taskID string
	if err := tx.GetContext(ctx, &taskID, tx.Rebind(`SELECT task_id FROM task_sessions WHERE id = ?`), sessionID); err != nil {
		return "", err
	}
	scope, err := r.readUsageAdmissionScopeTx(ctx, tx, taskID, sessionID)
	if err != nil {
		return "", err
	}
	binding := executionUsageBinding(ctx, sessionID, actorID, scope)
	if err := r.usageHistoryRetainer.AdmitBindingTx(ctx, tx, binding); err != nil {
		return "", fmt.Errorf("admit retained execution binding: %w", err)
	}
	return binding.ID, nil
}

func (r *Repository) readUsageAdmissionScopeTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, sessionID string,
) (usageAdmissionScope, error) {
	var scope usageAdmissionScope
	err := tx.GetContext(ctx, &scope, tx.Rebind(`
		SELECT t.id AS task_id, t.workspace_id, COALESCE(w.org_id, '') AS organization_id, t.created_at AS task_created_at,
		       COALESCE(NULLIF(ts.queue_incarnation_id, ''), ts.id) AS session_incarnation_id,
	       COALESCE(er.agent_execution_id, '') AS execution_id,
	       COALESCE(ts.usage_execution_binding_id, '') AS execution_binding_id,
	       COALESCE(ub.actor_id, '') AS execution_actor_id,
	       COALESCE(ub.actor_status, 'unknown') AS execution_actor_status,
	       COALESCE(ub.actor_kind, 'unknown') AS execution_actor_kind,
	       COALESCE(ub.actor_source, 'unknown') AS execution_actor_source
		FROM tasks t
		JOIN workspaces w ON w.id = t.workspace_id
		JOIN task_sessions ts ON ts.id = ? AND ts.task_id = t.id
		LEFT JOIN executors_running er ON er.session_id = ts.id
		LEFT JOIN usage_execution_bindings ub ON ub.binding_id = ts.usage_execution_binding_id
		WHERE t.id = ?
	`), sessionID, taskID)
	if err != nil {
		if err == sql.ErrNoRows {
			return scope, fmt.Errorf("resolve trusted usage admission scope for task %q and session %q: %w", taskID, sessionID, err)
		}
		return scope, fmt.Errorf("resolve trusted usage admission scope: %w", err)
	}
	return scope, nil
}

func turnUsageBinding(ctx context.Context, turn *models.Turn, scope usageAdmissionScope) usagehistory.Binding {
	actorID, actorStatus, actorKind, actorSource := resolveTurnUsageActor(ctx, scope)
	bindingID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("kandev-usage-turn:"+turn.ID)).String()
	return usagehistory.Binding{
		ID: bindingID, Kind: usagehistory.BindingKindTurn, WorkspaceID: scope.WorkspaceID,
		OrganizationID: scope.OrganizationID, TaskID: turn.TaskID, TaskIncarnationID: usageTaskIncarnationID(scope.TaskCreatedAt), SessionID: turn.TaskSessionID,
		SessionIncarnationID: scope.SessionIncarnationID, TurnID: turn.ID,
		ExecutionID: scope.ExecutionID, ParentBindingID: scope.ExecutionBindingID,
		ActorID: actorID, ActorStatus: actorStatus, ActorKind: actorKind,
		ActorSource: actorSource, CapturedAt: turn.CreatedAt,
	}
}

func executionUsageBinding(
	ctx context.Context,
	sessionID, actorID string,
	scope usageAdmissionScope,
) usagehistory.Binding {
	actorID, actorStatus, actorKind, actorSource := resolveUsageActor(ctx, "", actorID, "execution")
	return usagehistory.Binding{
		ID: uuid.NewString(), Kind: usagehistory.BindingKindExecution,
		WorkspaceID: scope.WorkspaceID, OrganizationID: scope.OrganizationID,
		TaskID: scope.TaskID, TaskIncarnationID: usageTaskIncarnationID(scope.TaskCreatedAt), SessionID: sessionID, SessionIncarnationID: scope.SessionIncarnationID,
		ExecutionID: scope.ExecutionID, ActorID: actorID, ActorStatus: actorStatus,
		ActorKind: actorKind, ActorSource: actorSource, CapturedAt: time.Now().UTC(),
	}
}

func usageTaskIncarnationID(createdAt time.Time) string {
	if createdAt.IsZero() {
		return ""
	}
	return createdAt.UTC().Format(time.RFC3339Nano)
}

func resolveUsageActor(
	ctx context.Context,
	_, executorActorID, admissionKind string,
) (actorID, status, kind, source string) {
	if identity, ok := authn.IdentityFromContext(ctx); ok {
		if identity.UserID == "" {
			return "", usagehistory.ActorStatusUnknown, usagehistory.ActorKindUnknown, usageAccountingUnknown
		}
		actorKind := usagehistory.ActorKindUser
		if identity.Synthetic {
			actorKind = usagehistory.ActorKindSynthetic
		}
		return identity.UserID, usagehistory.ActorStatusAttributed, actorKind, admissionKind
	}
	if actor, bound := models.ExecutorActorFromContext(ctx); bound {
		if actor == "" {
			return "", usagehistory.ActorStatusUnknown, usagehistory.ActorKindUnknown, usageAccountingUnknown
		}
		return actor, usagehistory.ActorStatusAttributed, usagehistory.ActorKindUser, "execution_context"
	}
	if executorActorID != "" {
		return executorActorID, usagehistory.ActorStatusAttributed, usagehistory.ActorKindUser, "execution_admission"
	}
	return "", usagehistory.ActorStatusUnknown, usagehistory.ActorKindUnknown, usageAccountingUnknown
}

func resolveTurnUsageActor(ctx context.Context, scope usageAdmissionScope) (actorID, status, kind, source string) {
	if identity, ok := authn.IdentityFromContext(ctx); ok {
		if identity.UserID == "" {
			return "", usagehistory.ActorStatusUnknown, usagehistory.ActorKindUnknown, usageAccountingUnknown
		}
		actorKind := usagehistory.ActorKindUser
		if identity.Synthetic {
			actorKind = usagehistory.ActorKindSynthetic
		}
		return identity.UserID, usagehistory.ActorStatusAttributed, actorKind, "turn_authenticated"
	}
	if actor, bound := models.ExecutorActorFromContext(ctx); bound {
		if actor == "" {
			return "", usagehistory.ActorStatusUnknown, usagehistory.ActorKindUnknown, "explicit_shared_turn"
		}
		return actor, usagehistory.ActorStatusAttributed, usagehistory.ActorKindUser, "turn_executor_context"
	}
	switch scope.ExecutionActorStatus {
	case usagehistory.ActorStatusAttributed:
		if scope.ExecutionActorID == "" {
			return "", usagehistory.ActorStatusUnknown, usagehistory.ActorKindUnknown, "invalid_execution_snapshot"
		}
		return scope.ExecutionActorID, usagehistory.ActorStatusAttributed, scope.ExecutionActorKind, "execution_snapshot"
	case usagehistory.ActorStatusMixed:
		return "", usagehistory.ActorStatusMixed, usagehistory.ActorKindUnknown, scope.ExecutionActorSource
	default:
		return "", usagehistory.ActorStatusUnknown, usagehistory.ActorKindUnknown, scope.ExecutionActorSource
	}
}

func nativeUsageFromTaskEvent(event *models.TaskUsageEvent) usagehistory.NativeUsage {
	costBasis := usageAccountingUnknown
	costPresent := false
	switch event.CostSource {
	case usageCostSourceProviderReported:
		costBasis, costPresent = "reported", true
	case "models_dev_list":
		costBasis, costPresent = "estimated", true
	}
	tokenSemanticsVersion := 0
	if event.UsageSchemaVersion > 0 {
		tokenSemanticsVersion = event.UsageSchemaVersion
	}
	return usagehistory.NativeUsage{
		BindingID: event.UsageBindingID, UsageEventID: event.UsageEventID,
		TaskID: event.TaskID, SessionID: event.SessionID, TurnID: event.TurnID,
		AgentClient: event.AgentType, Model: event.Model, Provider: event.Provider,
		ProviderThreadID: event.ProviderThreadID, ProviderTurnID: event.ProviderTurnID,
		ProviderResponseID: event.ProviderResponseID, NativeScope: event.NativeScope,
		MeasurementSource: event.MeasurementSource, UsageCompleteness: event.UsageCompleteness,
		UsageSchemaVersion: event.UsageSchemaVersion, ContractVersion: event.ContractVersion,
		InputTokens: event.TokensIn, ThoughtTokens: event.TokensThought,
		CacheReadTokens: event.TokensCachedRead, CacheWriteTokens: event.TokensCachedWrite,
		OutputTokens: event.TokensOut, ReasoningOutputTokens: event.ReasoningOutputTokens,
		ReportedCacheWriteTokens: event.ReportedCacheWriteTokens, ReportedTotalTokens: event.ReportedTotalTokens,
		TotalTokens: event.TokensTotal, CostSubcents: event.CostSubcents, CostPresent: costPresent,
		CostBasis: costBasis, CostSource: event.CostSource, Estimated: event.Estimated,
		PricingCatalogVersion: event.PricingCatalogVersion, TokenSemanticsVersion: tokenSemanticsVersion,
		ReasoningInsideOutput: event.ReasoningOutputTokens != nil, CacheReadInsideInput: false,
		OccurredAt: event.OccurredAt, CollectedAt: event.CreatedAt,
	}
}
