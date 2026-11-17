package orchestrator

import (
	"context"
	"encoding/json"
	"errors"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
)

func builtinApplicationCommand(app *models.BuiltinMCPApplication, status types.ProviderReplacementStatus, action string) (types.ProviderReplacementRequest, error) {
	var source types.ProviderReplacementSource
	if json.Unmarshal([]byte(app.AttemptSource), &source) != nil || source != status.Source || status.OperationID != app.OperationID {
		return types.ProviderReplacementRequest{}, errors.New("provider replacement ownership changed")
	}
	return types.ProviderReplacementRequest{Action: action, OperationID: app.OperationID, ExpectedVersion: status.Version, Source: source}, nil
}

func startBuiltinMCPApplication(ctx context.Context, repo builtinApplicationRepository, control agentruntime.ProviderMCPControl, app *models.BuiltinMCPApplication, status types.ProviderReplacementStatus) (bool, error) {
	command, err := builtinApplicationCommand(app, status, types.ProviderReplacementActionActivate)
	if err != nil {
		return false, err
	}
	binding, profile, err := p.ParseProviderMCPBinding(app.TargetBindingJSON)
	if err != nil {
		return false, err
	}
	command.Scope = binding.Scope
	command.Profile = *profile
	command.TargetStreamID = command.Source.StreamID
	if status.Phase != types.ProviderReplacementPhaseStarted && status.Phase != types.ProviderReplacementPhaseAccepted {
		status, err = control.Command(ctx, command)
		if err != nil {
			return false, err
		}
	}
	if status.Phase == types.ProviderReplacementPhaseFailed {
		return repo.AdvanceBuiltinMCPApplication(ctx, app, models.BuiltinMCPApplicationFailed, status.FailureCode)
	}
	if status.Phase != types.ProviderReplacementPhaseStarted || status.Scope == nil || *status.Scope != binding.Scope || !status.NativeResumeConfirmed {
		return false, errors.New("provider resume is unconfirmed")
	}
	return repo.AdvanceBuiltinMCPApplication(ctx, app, models.BuiltinMCPApplicationVerifying, "")
}

func acceptBuiltinMCPApplication(ctx context.Context, repo builtinApplicationRepository, app *models.BuiltinMCPApplication, status types.ProviderReplacementStatus) (bool, error) {
	if _, err := builtinApplicationCommand(app, status, types.ProviderReplacementActionAccept); err != nil {
		return false, err
	}
	binding, _, err := p.ParseProviderMCPBinding(app.TargetBindingJSON)
	if err != nil {
		return false, err
	}
	if status.Phase != types.ProviderReplacementPhaseStarted || status.Scope == nil || *status.Scope != binding.Scope || !status.SourceTerminated || !status.NativeResumeConfirmed {
		return false, errors.New("provider catalog evidence is unavailable")
	}
	committed, err := repo.CommitBuiltinProviderBinding(ctx, models.BuiltinMCPAcceptance{SessionID: app.SessionID, OperationID: app.OperationID, RowVersion: app.RowVersion, Binding: binding, Catalog: status.Catalog, NativeSessionID: status.Source.NativeSessionID, NativeResumeConfirmed: status.NativeResumeConfirmed})
	if committed || err != nil {
		return committed, err
	}
	fresh, err := repo.GetBuiltinMCPApplication(ctx, app.SessionID)
	if err != nil || fresh == nil {
		return false, err
	}
	return repo.AdvanceBuiltinMCPApplication(ctx, fresh, models.BuiltinMCPApplicationFailed, "settings_changed")
}

func finishBuiltinMCPApplication(ctx context.Context, repo builtinApplicationRepository, control agentruntime.ProviderMCPControl, app *models.BuiltinMCPApplication, status types.ProviderReplacementStatus) (bool, error) {
	command, err := builtinApplicationCommand(app, status, types.ProviderReplacementActionAccept)
	if err != nil {
		return false, err
	}
	binding, _, err := p.ParseProviderMCPBinding(app.TargetBindingJSON)
	if err != nil {
		return false, err
	}
	command.Scope = binding.Scope
	if err := control.CloseEffects(); err != nil {
		return false, err
	}
	if err := control.AdoptBinding(binding); err != nil {
		return false, err
	}
	status, err = control.Command(ctx, command)
	if err != nil {
		return false, err
	}
	if status.Phase != types.ProviderReplacementPhaseAccepted || status.Scope == nil || *status.Scope != binding.Scope {
		return false, errors.New("provider acceptance is unconfirmed")
	}
	finalized, err := repo.AdvanceBuiltinMCPApplication(ctx, app, models.BuiltinMCPApplicationApplied, "")
	if finalized {
		control.OpenEffects()
	}
	return finalized, err
}
