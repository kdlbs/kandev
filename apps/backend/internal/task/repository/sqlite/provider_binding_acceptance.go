package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
)

// CommitBuiltinProviderBinding commits only the verified, still-saved target.
// Provider effects stay closed until the helper acknowledges this acceptance.
func (r *Repository) CommitBuiltinProviderBinding(ctx context.Context, request models.BuiltinMCPAcceptance) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), request.SessionID); err != nil {
		return false, err
	}
	app, metadata, err := r.builtinApplicationOwnerTx(ctx, tx, request.SessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	profile, valid := validateBuiltinProviderAcceptance(app, request)
	if !valid {
		return false, nil
	}
	matches, err := r.builtinSavedTargetMatchesTx(ctx, tx, app)
	if err != nil {
		return false, err
	}
	if !matches {
		return false, nil
	}
	if err := r.recordBuiltinProviderSnapshotTx(ctx, tx, app, request, profile); err != nil {
		return false, err
	}
	metadata[p.ProviderMCPBindingMetadataKey] = request.Binding
	metadata[p.ProviderMCPEffectsClosedMetadataKey] = true
	metadata[p.ProviderMCPOperationMetadataKey] = map[string]any{"operation_id": app.OperationID, "stage": models.BuiltinMCPApplicationAccepting}
	delete(metadata, p.ProviderMCPRuntimeProfileMetadataKey)
	if err := r.writeBuiltinExecutorMetadataTx(ctx, tx, app, metadata); err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE task_builtin_mcp_applications SET stage='accepting',reason='',row_version=row_version+1,updated_at=? WHERE session_id=? AND operation_id=? AND row_version=?`), time.Now().UTC(), app.SessionID, app.OperationID, app.RowVersion)
	if err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (r *Repository) builtinApplicationOwnerTx(ctx context.Context, tx *sqlx.Tx, sessionID string) (*models.BuiltinMCPApplication, map[string]any, error) {
	var app models.BuiltinMCPApplication
	err := tx.GetContext(ctx, &app, tx.Rebind(`SELECT a.* FROM task_builtin_mcp_applications a JOIN task_sessions s ON s.id=a.session_id AND s.task_id=a.task_id
 JOIN tasks t ON t.id=a.task_id AND t.workspace_id=a.workspace_id JOIN executors_running er ON er.session_id=s.id AND er.task_id=t.id AND er.agent_execution_id=a.source_execution_id
 WHERE a.session_id=? AND s.state='WAITING_FOR_INPUT' AND t.archived_at IS NULL AND er.resumable=TRUE AND er.resume_token<>''`), sessionID)
	if err != nil {
		return nil, nil, err
	}
	var raw string
	err = tx.QueryRowContext(ctx, tx.Rebind(`SELECT COALESCE(metadata,'{}') FROM executors_running WHERE session_id=? AND agent_execution_id=?`), sessionID, app.SourceExecutionID).Scan(&raw)
	if err != nil {
		return nil, nil, err
	}
	metadata := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return nil, nil, err
	}
	return &app, metadata, nil
}

func validateBuiltinProviderAcceptance(app *models.BuiltinMCPApplication, request models.BuiltinMCPAcceptance) (*p.Context, bool) {
	if app.Stage != models.BuiltinMCPApplicationVerifying || app.OperationID != request.OperationID || app.RowVersion != request.RowVersion || !request.NativeResumeConfirmed || len(request.Catalog) == 0 {
		return nil, false
	}
	binding, profile, err := p.ParseProviderMCPBinding(request.Binding)
	if err != nil {
		return nil, false
	}
	if !builtinProviderApplicationMatchesTarget(app, binding) || !builtinProviderCatalogComplete(profile.BuiltinPackages, request.Catalog) {
		return nil, false
	}

	if !builtinProviderManifestMatchesApplication(profile.BuiltinPackages, app) {
		return nil, false
	}

	expected, _, err := p.ParseProviderMCPBinding(app.TargetBindingJSON)
	if err != nil || expected != binding {
		return nil, false
	}
	var source types.ProviderReplacementSource
	if json.Unmarshal([]byte(app.AttemptSource), &source) != nil {
		return nil, false
	}
	if !builtinProviderSuccessorMatches(app, source, binding.Scope, request.NativeSessionID) {
		return nil, false
	}
	return profile, true
}

func builtinProviderSuccessorMatches(app *models.BuiltinMCPApplication, source types.ProviderReplacementSource, target p.ProviderMCPScope, nativeID string) bool {
	return source.ExecutionID == app.SourceExecutionID && target.ExecutionID == source.ExecutionID && source.TaskID == app.TaskID && source.SessionID == app.SessionID && source.HelperID == target.HelperID && target.ProcessGeneration == source.ProcessGeneration+1 && target.HarnessGeneration == source.HarnessGeneration && source.NativeSessionID != "" && nativeID == source.NativeSessionID
}

func (r *Repository) builtinSavedTargetMatchesTx(ctx context.Context, tx *sqlx.Tx, app *models.BuiltinMCPApplication) (bool, error) {
	var matches int
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT COUNT(*) FROM task_builtin_mcp_preferences pref WHERE pref.task_id=? AND pref.workspace_id=? AND pref.revision=? AND pref.updated_by=?
 AND COALESCE((SELECT revision FROM workspace_builtin_mcp_policies WHERE workspace_id=?),0)=?`), app.TaskID, app.WorkspaceID, app.TargetRevision, app.RequestedBy, app.WorkspaceID, app.AttemptPolicyRevision).Scan(&matches)
	return matches == 1, err
}

func (r *Repository) recordBuiltinProviderSnapshotTx(ctx context.Context, tx *sqlx.Tx, app *models.BuiltinMCPApplication, request models.BuiltinMCPAcceptance, _ *p.Context) error {
	bindingJSON, err := json.Marshal(request.Binding)
	if err != nil {
		return err
	}
	catalogJSON, err := json.Marshal(request.Catalog)
	if err != nil {
		return err
	}
	var existing string
	err = tx.QueryRowContext(ctx, tx.Rebind(`SELECT binding_json FROM agent_provider_mcp_bindings WHERE binding_id=?`), request.Binding.Scope.BindingID).Scan(&existing)
	if err == nil && existing != string(bindingJSON) {
		return models.ErrExecutionRotated
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO agent_provider_mcp_bindings
 (binding_id,session_id,task_id,workspace_id,execution_id,operation_id,binding_json,catalog_json,target_revision,policy_revision,native_session_id,created_at)
 VALUES (?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(binding_id) DO NOTHING`), request.Binding.Scope.BindingID, app.SessionID, app.TaskID, app.WorkspaceID, app.SourceExecutionID, app.OperationID, string(bindingJSON), string(catalogJSON), app.AttemptRevision, app.AttemptPolicyRevision, request.NativeSessionID, time.Now().UTC())
	return err
}

func (r *Repository) writeBuiltinExecutorMetadataTx(ctx context.Context, tx *sqlx.Tx, app *models.BuiltinMCPApplication, metadata map[string]any) error {
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE executors_running SET metadata=?,updated_at=? WHERE session_id=? AND agent_execution_id=?`), string(raw), time.Now().UTC(), app.SessionID, app.SourceExecutionID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return models.ErrExecutionRotated
	}
	return nil
}

func builtinProviderManifestMatchesApplication(manifest *p.BuiltinManifest, app *models.BuiltinMCPApplication) bool {
	return mcpconfig.BuiltinSelectionHash(manifest.Packages) == app.AttemptSelectionHash && manifest.TaskID == app.TaskID && manifest.WorkspaceID == app.WorkspaceID && manifest.TaskRevision == app.TargetRevision && manifest.WorkspaceRevision == app.AttemptPolicyRevision
}

func builtinProviderApplicationMatchesTarget(app *models.BuiltinMCPApplication, binding p.ProviderMCPBinding) bool {
	return app.TargetBindingID == binding.Scope.BindingID && app.AttemptRevision == app.TargetRevision && app.AttemptSelectionHash == app.SelectionHash && app.AttemptActorID == app.RequestedBy
}
