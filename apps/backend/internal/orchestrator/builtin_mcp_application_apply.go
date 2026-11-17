package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
)

func (s *Service) applyBuiltinMCPApplication(ctx context.Context, repo builtinApplicationRepository, control agentruntime.ProviderMCPControl, app *models.BuiltinMCPApplication, session *models.TaskSession, running *models.ExecutorRunning) error {
	if app.Stage == models.BuiltinMCPApplicationPending {
		var err error
		app, err = s.claimBuiltinMCPApplication(ctx, repo, control, app, session, running)
		if err != nil || app == nil {
			return err
		}
	}
	if app.Stage != models.BuiltinMCPApplicationPreparing {
		s.yieldBuiltinApplicationAdmission(ctx)
	}
	for range 8 {
		progressed, err := s.reconcileBuiltinMCPApplicationAttempt(ctx, repo, control, app, session, running)
		if err != nil || !progressed {
			return err
		}
		app, err = repo.GetBuiltinMCPApplication(ctx, app.SessionID)
		if err != nil || app == nil {
			return err
		}
	}
	return nil
}

func (s *Service) advanceBuiltinMCPApplication(ctx context.Context, repo builtinApplicationRepository, control agentruntime.ProviderMCPControl, app *models.BuiltinMCPApplication, session *models.TaskSession, running *models.ExecutorRunning, status types.ProviderReplacementStatus) (bool, error) {
	switch app.Stage {
	case models.BuiltinMCPApplicationPreparing:
		return s.prepareBuiltinMCPApplication(ctx, repo, control, app, status)
	case models.BuiltinMCPApplicationRetiring:
		return s.retireBuiltinMCPApplication(ctx, repo, control, app, session, running, status)
	case models.BuiltinMCPApplicationStarting:
		return startBuiltinMCPApplication(ctx, repo, control, app, status)
	case models.BuiltinMCPApplicationVerifying:
		return acceptBuiltinMCPApplication(ctx, repo, app, status)
	case models.BuiltinMCPApplicationAccepting:
		return finishBuiltinMCPApplication(ctx, repo, control, app, status)
	}
	return false, nil
}

func (s *Service) prepareBuiltinMCPApplication(ctx context.Context, repo builtinApplicationRepository, control agentruntime.ProviderMCPControl, app *models.BuiltinMCPApplication, status types.ProviderReplacementStatus) (bool, error) {
	if status.Phase == types.ProviderReplacementPhaseMissing {
		if app.RetryOfOperationID != "" {
			previous, err := control.Status(ctx, app.RetryOfOperationID)
			if err != nil {
				return false, err
			}
			status.Source.NativeSessionID = previous.Source.NativeSessionID
		}
		req := types.ProviderReplacementRequest{Action: types.ProviderReplacementActionPrepare, OperationID: app.OperationID, PreviousOperationID: app.RetryOfOperationID, Source: status.Source}
		var err error
		status, err = control.Command(ctx, req)
		if err != nil {
			return false, err
		}
	}
	if status.Phase != types.ProviderReplacementPhasePrepared {
		return false, nil
	}
	raw, err := json.Marshal(status.Source)
	if err != nil {
		return false, err
	}
	app.AttemptSource = string(raw)
	if err := control.CloseEffects(); err != nil {
		return false, err
	}
	advanced, err := repo.AdvanceBuiltinMCPApplication(ctx, app, models.BuiltinMCPApplicationRetiring, "")
	if advanced {
		s.yieldBuiltinApplicationAdmission(ctx)
	}
	if advanced || err != nil {
		return advanced, err
	}
	_, releaseErr := control.Command(ctx, types.ProviderReplacementRequest{Action: types.ProviderReplacementActionRelease, OperationID: app.OperationID, Source: status.Source, ExpectedVersion: status.Version})
	if releaseErr != nil {
		return false, releaseErr
	}
	control.OpenEffects()
	fresh, err := repo.GetBuiltinMCPApplication(ctx, app.SessionID)
	if err != nil || fresh == nil {
		return false, err
	}
	return repo.AdvanceBuiltinMCPApplication(ctx, fresh, models.BuiltinMCPApplicationPending, "")
}

func (s *Service) retireBuiltinMCPApplication(ctx context.Context, repo builtinApplicationRepository, control agentruntime.ProviderMCPControl, app *models.BuiltinMCPApplication, session *models.TaskSession, running *models.ExecutorRunning, status types.ProviderReplacementStatus) (bool, error) {
	if err := control.CloseEffects(); err != nil {
		return false, err
	}
	command, err := builtinApplicationCommand(app, status, types.ProviderReplacementActionRetire)
	if err != nil {
		return false, err
	}
	status, err = control.Command(ctx, command)
	if err != nil {
		return false, err
	}
	if status.Phase != types.ProviderReplacementPhaseRetired || !status.SourceTerminated {
		return false, errors.New("provider termination is unconfirmed")
	}
	profile, err := s.executor.PrepareTaskSessionMCPProfile(ctx, app.TaskID, session, false)
	if err != nil {
		return false, err
	}
	if profile.BuiltinPackages == nil || profile.BuiltinPackages.TaskRevision != app.AttemptRevision || profile.BuiltinPackages.WorkspaceRevision != app.AttemptPolicyRevision {
		return repo.AdvanceBuiltinMCPApplication(ctx, app, models.BuiltinMCPApplicationFailed, "settings_changed")
	}
	profile, err = retainBuiltinRuntimeContext(profile, running)
	if err != nil {
		return false, err
	}
	sealed, err := p.MarshalBuiltinBinding(profile)
	if err != nil {
		return false, err
	}
	digest, err := p.BuiltinBindingDigest(profile)
	if err != nil {
		return false, err
	}
	scope := p.ProviderMCPScope{BindingID: uuid.NewString(), ExecutionID: status.Source.ExecutionID, HelperID: status.Source.HelperID, ProcessGeneration: status.Source.ProcessGeneration + 1, HarnessGeneration: status.Source.HarnessGeneration, ProfileDigest: digest}
	raw, err := json.Marshal(p.ProviderMCPBinding{Scope: scope, Profile: sealed})
	if err != nil {
		return false, err
	}
	app.TargetBindingJSON = string(raw)
	return repo.AdvanceBuiltinMCPApplication(ctx, app, models.BuiltinMCPApplicationStarting, "")
}

func retainBuiltinRuntimeContext(desired p.Context, running *models.ExecutorRunning) (p.Context, error) {
	var current *p.Context
	var err error
	if value, ok := running.Metadata[p.ProviderMCPBindingMetadataKey]; ok {
		binding, _, parseErr := p.ParseProviderMCPBinding(value)
		if parseErr != nil {
			return desired, parseErr
		}
		runtime, present := running.Metadata[p.ProviderMCPRuntimeProfileMetadataKey]
		current, err = p.ProviderRuntimeProfile(binding, runtime, present)
	} else if value, ok := running.Metadata[p.BuiltinBindingMetadataKey]; ok {
		current, err = p.ParseBuiltinBindingMetadata(value)
	}
	if err != nil {
		return desired, err
	}
	if current == nil || current.Surface != desired.Surface {
		return desired, errors.New("provider runtime context is unavailable")
	}
	current.BuiltinPackages = desired.BuiltinPackages.Clone()
	return *current, nil
}

func prepareBuiltinApplicationRetry(ctx context.Context, control agentruntime.ProviderMCPControl, app *models.BuiltinMCPApplication, current types.ProviderReplacementStatus) (types.ProviderReplacementStatus, error) {
	previous, err := control.Status(ctx, app.RetryOfOperationID)
	if err != nil {
		return current, err
	}
	if previous.Phase == types.ProviderReplacementPhaseAccepted {
		current.Source.NativeSessionID = previous.Source.NativeSessionID
		current.Ready = true
		current.Capability = types.ProviderMCPReplacementCapability
		return current, nil
	}
	if previous.Phase == types.ProviderReplacementPhaseReleased {
		if current.Source != previous.Source {
			return current, errors.New("released provider source changed")
		}
		return current, nil
	}
	if previous.Phase != types.ProviderReplacementPhaseFailed {
		previous, err = control.Command(ctx, types.ProviderReplacementRequest{Action: types.ProviderReplacementActionAbandon, OperationID: previous.OperationID, Source: previous.Source, ExpectedVersion: previous.Version})
		if err != nil {
			return current, err
		}
	}
	if previous.Phase != types.ProviderReplacementPhaseFailed {
		return current, errors.New("failed provider candidate is unconfirmed")
	}
	current.Source.NativeSessionID = previous.Source.NativeSessionID
	current.Ready = true
	current.Capability = types.ProviderMCPReplacementCapability
	return current, nil
}

func (s *Service) builtinApplicationReadyStatus(ctx context.Context, repo builtinApplicationRepository, control agentruntime.ProviderMCPControl, app *models.BuiltinMCPApplication, running *models.ExecutorRunning) (types.ProviderReplacementStatus, bool, error) {
	status, err := control.Status(ctx, "")
	if err != nil {
		reason := "helper_unavailable"
		if errors.Is(err, agentruntime.ErrProviderReplacementUnsupported) {
			reason = "helper_update_required"
		}
		return status, false, repo.SetBuiltinMCPApplicationPendingReason(ctx, app.SessionID, app.RowVersion, reason)
	}
	if app.RetryOfOperationID != "" {
		status, err = prepareBuiltinApplicationRetry(ctx, control, app, status)
		if err != nil {
			return status, false, err
		}
	}
	if !status.Ready || status.Capability != types.ProviderMCPReplacementCapability {
		reason := "waiting_for_safe_boundary"
		if status.Capability != types.ProviderMCPReplacementCapability {
			reason = "unsupported_resume"
		}
		return status, false, repo.SetBuiltinMCPApplicationPendingReason(ctx, app.SessionID, app.RowVersion, reason)
	}
	if status.Source.ExecutionID != running.AgentExecutionID || status.Source.TaskID != app.TaskID || status.Source.SessionID != app.SessionID || status.Source.NativeSessionID != running.ResumeToken {
		return status, false, errors.New("provider source does not match the retained conversation")
	}
	return status, true, nil
}

func (s *Service) claimBuiltinMCPApplication(ctx context.Context, repo builtinApplicationRepository, control agentruntime.ProviderMCPControl, app *models.BuiltinMCPApplication, session *models.TaskSession, running *models.ExecutorRunning) (*models.BuiltinMCPApplication, error) {
	status, ready, err := s.builtinApplicationReadyStatus(ctx, repo, control, app, running)
	if err != nil || !ready {
		return nil, err
	}

	desired, err := s.executor.PrepareTaskSessionMCPProfile(ctx, app.TaskID, session, false)
	if err != nil {
		return nil, err
	}
	if desired.BuiltinPackages == nil || desired.BuiltinPackages.WorkspaceRevision != app.PolicyRevision || desired.BuiltinPackages.TaskRevision != app.TargetRevision {
		return nil, repo.SetBuiltinMCPApplicationPendingReason(ctx, app.SessionID, app.RowVersion, "policy_changed")
	}
	if _, err := retainBuiltinRuntimeContext(desired, running); err != nil {
		return nil, err
	}
	if app.RetryOfOperationID == "" {
		unchanged, err := repo.SupersedeBuiltinMCPApplication(ctx, app.SessionID, running.AgentExecutionID, status.Source.ProfileDigest, app.RowVersion)
		if err != nil || unchanged {
			return nil, err
		}
	}
	app, err = repo.ClaimBuiltinMCPApplication(ctx, app.SessionID, running.AgentExecutionID, app.RowVersion)
	if err != nil || app == nil {
		return nil, err
	}

	return app, nil
}

func (s *Service) reconcileBuiltinMCPApplicationAttempt(ctx context.Context, repo builtinApplicationRepository, control agentruntime.ProviderMCPControl, app *models.BuiltinMCPApplication, session *models.TaskSession, running *models.ExecutorRunning) (bool, error) {
	if !builtinApplicationAttemptActive(app) {
		return false, nil
	}

	expired := builtinApplicationDeadlineExpired(app)
	status, err := control.Status(ctx, app.OperationID)
	if expired && !builtinApplicationAcceptanceConfirmed(app, status, err) {
		if app.Stage == models.BuiltinMCPApplicationAccepting {
			_, parkErr := repo.AdvanceBuiltinMCPApplication(ctx, app, models.BuiltinMCPApplicationAccepting, "acceptance_unconfirmed")
			return false, parkErr
		}
		_, failErr := repo.AdvanceBuiltinMCPApplication(ctx, app, models.BuiltinMCPApplicationFailed, "verification_timeout")
		return false, failErr
	}
	if err != nil {
		return false, err
	}
	if status.Phase == types.ProviderReplacementPhaseFailed {
		_, err := repo.AdvanceBuiltinMCPApplication(ctx, app, models.BuiltinMCPApplicationFailed, status.FailureCode)
		return false, err
	}
	return s.advanceBuiltinMCPApplication(ctx, repo, control, app, session, running, status)
}

func builtinApplicationAttemptActive(app *models.BuiltinMCPApplication) bool {
	if app.Stage == models.BuiltinMCPApplicationApplied || app.Stage == models.BuiltinMCPApplicationFailed || app.Stage == models.BuiltinMCPApplicationPending {
		return false
	}
	return app.Stage != models.BuiltinMCPApplicationAccepting || app.Reason != "acceptance_unconfirmed"
}
func builtinApplicationDeadlineExpired(app *models.BuiltinMCPApplication) bool {
	return app.VerificationDeadline != nil && time.Now().After(*app.VerificationDeadline)
}
func builtinApplicationAcceptanceConfirmed(app *models.BuiltinMCPApplication, status types.ProviderReplacementStatus, err error) bool {
	return err == nil && status.Phase == types.ProviderReplacementPhaseAccepted && app.Stage == models.BuiltinMCPApplicationAccepting
}
