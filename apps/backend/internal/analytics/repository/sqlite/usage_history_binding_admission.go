package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/usagehistory"
)

type usageBindingEvidence struct {
	Kind                 string `db:"binding_kind"`
	ScopeID              string `db:"scope_id"`
	SubjectID            string `db:"subject_id"`
	WorkspaceID          string `db:"workspace_id"`
	TaskID               string `db:"task_id"`
	TaskIncarnationID    string `db:"task_incarnation_id"`
	SessionID            string `db:"session_id"`
	SessionIncarnationID string `db:"session_incarnation_id"`
	TurnID               string `db:"turn_id"`
	ExecutionID          string `db:"execution_id"`
	ActorID              string `db:"actor_id"`
	ActorStatus          string `db:"actor_status"`
	ActorKind            string `db:"actor_kind"`
	ActorSource          string `db:"actor_source"`
	ParentBindingID      string `db:"parent_binding_id"`
}

func (r *Repository) verifyUsageBindingTx(ctx context.Context, tx *sqlx.Tx, binding usagehistory.Binding, scopeID, subjectID string) error {
	var stored usageBindingEvidence
	err := tx.GetContext(ctx, &stored, r.db.Rebind(`SELECT binding_kind, scope_id, subject_id, workspace_id, task_id, task_incarnation_id,
		session_id, session_incarnation_id, turn_id, execution_id, actor_id, actor_status, actor_kind,
		actor_source, parent_binding_id
		FROM usage_execution_bindings WHERE binding_id = ?`), binding.ID)
	if err != nil {
		return fmt.Errorf("read retained usage binding: %w", err)
	}
	expected := usageBindingEvidence{
		Kind: binding.Kind, ScopeID: scopeID, SubjectID: subjectID, WorkspaceID: binding.WorkspaceID,
		TaskID: binding.TaskID, TaskIncarnationID: binding.TaskIncarnationID, SessionID: binding.SessionID,
		SessionIncarnationID: binding.SessionIncarnationID, TurnID: binding.TurnID,
		ExecutionID: binding.ExecutionID, ActorID: binding.ActorID, ActorStatus: binding.ActorStatus,
		ActorKind: binding.ActorKind, ActorSource: binding.ActorSource, ParentBindingID: binding.ParentBindingID,
	}
	if stored != expected {
		return fmt.Errorf("retained usage binding %q conflicts with immutable admission evidence", binding.ID)
	}
	return nil
}

type retainedNativeUsageBinding struct {
	ScopeID              string `db:"scope_id"`
	SubjectID            string `db:"subject_id"`
	WorkspaceID          string `db:"workspace_id"`
	TaskID               string `db:"task_id"`
	TaskIncarnationID    string `db:"task_incarnation_id"`
	SessionID            string `db:"session_id"`
	SessionIncarnationID string `db:"session_incarnation_id"`
	TurnID               string `db:"turn_id"`
	ActorID              string `db:"actor_id"`
	ActorStatus          string `db:"actor_status"`
	ActorKind            string `db:"actor_kind"`
}

func (r *Repository) admitNativeUsageBindingTx(ctx context.Context, tx *sqlx.Tx, native *usagehistory.NativeUsage, scope trustedUsageScope) (retainedNativeUsageBinding, error) {
	if native.BindingID == "" {
		binding, err := r.resolveNativeBindingTx(ctx, tx, *native, scope)
		if err != nil {
			return retainedNativeUsageBinding{}, err
		}
		native.BindingID = binding.ID
		if err := r.AdmitBindingTx(ctx, tx, binding); err != nil {
			return retainedNativeUsageBinding{}, err
		}
	}
	var binding retainedNativeUsageBinding
	if err := tx.GetContext(ctx, &binding, r.db.Rebind(`SELECT scope_id, subject_id, workspace_id, task_id, task_incarnation_id,
		session_id, session_incarnation_id, turn_id, actor_id, actor_status, actor_kind
		FROM usage_execution_bindings WHERE binding_id = ?`), native.BindingID); err != nil {
		return binding, fmt.Errorf("resolve native usage actor binding: %w", err)
	}
	return binding, validateRetainedNativeUsageBinding(*native, scope, binding)
}

func validateRetainedNativeUsageBinding(native usagehistory.NativeUsage, scope trustedUsageScope, binding retainedNativeUsageBinding) error {
	if binding.ScopeID != scope.ScopeID || binding.SubjectID != scope.SubjectID || binding.WorkspaceID != scope.WorkspaceID ||
		binding.TaskID != native.TaskID || binding.TaskIncarnationID != scope.TaskIncarnationID ||
		(binding.SessionID != "" && native.SessionID != "" && binding.SessionID != native.SessionID) ||
		(binding.TurnID != "" && native.TurnID != "" && binding.TurnID != native.TurnID) {
		return fmt.Errorf("native usage event %q does not match binding %q", native.UsageEventID, native.BindingID)
	}
	if native.BindingID == "" {
		return errors.New("native usage binding id is required")
	}
	return nil
}

type admittedNativeTurnBinding struct {
	ID                   string    `db:"id"`
	Kind                 string    `db:"kind"`
	WorkspaceID          string    `db:"workspace_id"`
	OrganizationID       string    `db:"organization_id"`
	TaskID               string    `db:"task_id"`
	TaskIncarnationID    string    `db:"task_incarnation_id"`
	SessionID            string    `db:"session_id"`
	SessionIncarnationID string    `db:"session_incarnation_id"`
	TurnID               string    `db:"turn_id"`
	ExecutionID          string    `db:"execution_id"`
	ParentBindingID      string    `db:"parent_binding_id"`
	ActorID              string    `db:"actor_id"`
	ActorStatus          string    `db:"actor_status"`
	ActorKind            string    `db:"actor_kind"`
	ActorSource          string    `db:"actor_source"`
	CapturedAt           time.Time `db:"captured_at"`
}

func (r *Repository) readNativeTurnBindingTx(ctx context.Context, tx *sqlx.Tx, bindingID string, native usagehistory.NativeUsage, scope trustedUsageScope) (usagehistory.Binding, error) {
	var binding admittedNativeTurnBinding
	if err := tx.GetContext(ctx, &binding, r.db.Rebind(`
		SELECT b.binding_id AS id, b.binding_kind AS kind, b.workspace_id, s.organization_id,
		       b.task_id, b.task_incarnation_id, b.session_id, b.session_incarnation_id, b.turn_id, b.execution_id,
		       b.parent_binding_id, b.actor_id, b.actor_status, b.actor_kind, b.actor_source,
		       b.created_at AS captured_at
		FROM usage_execution_bindings b JOIN usage_scopes s ON s.scope_id = b.scope_id
		WHERE b.binding_id = ?`), bindingID); err != nil {
		return usagehistory.Binding{}, fmt.Errorf("read native turn usage binding %q: %w", bindingID, err)
	}
	if binding.Kind != usagehistory.BindingKindTurn || binding.WorkspaceID != scope.WorkspaceID ||
		binding.TaskID != native.TaskID || binding.TurnID != native.TurnID ||
		(native.SessionID != "" && binding.SessionID != native.SessionID) {
		return usagehistory.Binding{}, fmt.Errorf("native turn %q has a binding that conflicts with trusted scope", native.TurnID)
	}
	return binding.binding(), nil
}

func (binding admittedNativeTurnBinding) binding() usagehistory.Binding {
	return usagehistory.Binding{
		ID: binding.ID, Kind: binding.Kind, WorkspaceID: binding.WorkspaceID,
		OrganizationID: binding.OrganizationID, TaskID: binding.TaskID,
		SessionID: binding.SessionID, TaskIncarnationID: binding.TaskIncarnationID, SessionIncarnationID: binding.SessionIncarnationID,
		TurnID: binding.TurnID, ExecutionID: binding.ExecutionID,
		ParentBindingID: binding.ParentBindingID, ActorID: binding.ActorID,
		ActorStatus: binding.ActorStatus, ActorKind: binding.ActorKind,
		ActorSource: binding.ActorSource, CapturedAt: binding.CapturedAt,
	}
}
