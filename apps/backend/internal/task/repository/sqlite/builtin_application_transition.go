package sqlite

import (
	"context"
	"encoding/json"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"time"
)

func (r *Repository) stageBuiltinRetirementTx(ctx context.Context, tx *sqlx.Tx, app, request *models.BuiltinMCPApplication, metadata map[string]any) (bool, error) {
	matches, err := r.builtinSavedTargetMatchesTx(ctx, tx, app)
	if err != nil {
		return false, err
	}
	if !matches || app.AttemptRevision != app.TargetRevision || app.AttemptActorID != app.RequestedBy || app.AttemptSelectionHash != app.SelectionHash {
		return false, nil
	}
	var source types.ProviderReplacementSource
	if json.Unmarshal([]byte(request.AttemptSource), &source) != nil || !builtinApplicationSourceMatches(source, app) {
		return false, nil
	}
	app.AttemptSource = request.AttemptSource
	metadata[p.ProviderMCPEffectsClosedMetadataKey] = true
	metadata[p.ProviderMCPOperationMetadataKey] = map[string]any{"operation_id": app.OperationID, "stage": models.BuiltinMCPApplicationRetiring}
	if err := r.writeBuiltinExecutorMetadataTx(ctx, tx, app, metadata); err != nil {
		return false, err
	}

	return true, nil
}

func (r *Repository) stageBuiltinProviderTarget(app, request *models.BuiltinMCPApplication) (bool, error) {
	binding, profile, err := p.ParseProviderMCPBinding(request.TargetBindingJSON)
	if err != nil {
		return false, err
	}
	var source types.ProviderReplacementSource
	if json.Unmarshal([]byte(app.AttemptSource), &source) != nil || !builtinProviderSuccessorMatches(app, source, binding.Scope, source.NativeSessionID) {
		return false, nil
	}
	if profile.BuiltinPackages.TaskID != app.TaskID || profile.BuiltinPackages.WorkspaceID != app.WorkspaceID || profile.BuiltinPackages.TaskRevision != app.AttemptRevision || profile.BuiltinPackages.WorkspaceRevision != app.AttemptPolicyRevision {
		return false, nil
	}
	if mcpconfig.BuiltinSelectionHash(profile.BuiltinPackages.Packages) != app.AttemptSelectionHash {
		return false, nil
	}
	app.TargetBindingID = binding.Scope.BindingID
	app.TargetBindingJSON = request.TargetBindingJSON

	return true, nil
}

func (r *Repository) finalizeBuiltinProviderTx(ctx context.Context, tx *sqlx.Tx, app, request *models.BuiltinMCPApplication, metadata map[string]any) (bool, error) {
	app.RetryOfOperationID = ""
	binding, _, err := p.ParseProviderMCPBinding(metadata[p.ProviderMCPBindingMetadataKey])
	if err != nil || binding.Scope.BindingID != app.TargetBindingID {
		return false, nil
	}
	metadata[p.ProviderMCPEffectsClosedMetadataKey] = false
	delete(metadata, p.ProviderMCPOperationMetadataKey)
	if err := r.writeBuiltinExecutorMetadataTx(ctx, tx, app, metadata); err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE agent_provider_mcp_bindings SET accepted_at=? WHERE binding_id=? AND operation_id=?`), time.Now().UTC(), app.TargetBindingID, app.OperationID)
	if err != nil {
		return false, err
	}

	return true, nil
}

func builtinApplicationSourceMatches(source types.ProviderReplacementSource, app *models.BuiltinMCPApplication) bool {
	return source.ExecutionID == app.SourceExecutionID && source.TaskID == app.TaskID && source.SessionID == app.SessionID && source.NativeSessionID != "" && source.HelperID != "" && source.HarnessGeneration != 0 && source.StreamID != ""
}
