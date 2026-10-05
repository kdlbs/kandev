package controller

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

func (c *Controller) SetRuntimeAutoUpdateStore(store *managedruntime.AutoUpdateStore) {
	c.runtimeAutoUpdateStore = store
}

func (c *Controller) automaticUpdateSupported(cap agents.RuntimeUpdateCapability) bool {
	return cap.Managed != nil && c.verifiedManagedActivation() && c.runtimeAutoUpdateStore != nil && c.updateJobStore != nil
}

// SetAgentAutomaticUpdates binds install-wide consent to the trusted runtime.
func (c *Controller) SetAgentAutomaticUpdates(ctx context.Context, name string, enabled bool) error {
	c.runtimeAutoUpdateMu.Lock()
	defer c.runtimeAutoUpdateMu.Unlock()
	return c.setAgentAutomaticUpdatesLocked(ctx, name, enabled)
}

func (c *Controller) setAgentAutomaticUpdatesLocked(ctx context.Context, name string, enabled bool) error {
	ag, found := c.agentRegistry.Get(name)
	if !found {
		return ErrAgentNotFound
	}
	capability := agents.RuntimeUpdateCapabilities(ag)
	if enabled && (!ag.Enabled() || !c.automaticUpdateSupported(capability)) {
		return ErrRuntimeUpdateUnsupported
	}
	policy, err := c.runtimeAutoUpdateStore.Get(ctx, name, capability.RuntimeID)
	if err != nil {
		return err
	}
	if policy.Enabled != enabled {
		policy.AttemptedVersion = ""
	}
	policy.Enabled = enabled
	return c.runtimeAutoUpdateStore.Save(ctx, name, policy)
}

// RunRuntimeUpdatePass shares source discovery with the read-only status API.
func (c *Controller) RunRuntimeUpdatePass(ctx context.Context) error {
	c.runtimeUpdatePassMu.Lock()
	defer c.runtimeUpdatePassMu.Unlock()
	response, err := c.ListAgentUpdateStatuses(ctx)
	if err != nil {
		return err
	}
	for _, status := range response.Statuses {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		c.settleInterruptedAutomaticUpdate(ctx, &status)
		c.publishRuntimeStatus(ctx, status)
		if !status.Available || !status.Enabled || !status.AutoUpdateSupported || status.CheckState != dto.AgentUpdateCheckStateUpdateAvailable {
			continue
		}
		if err := c.enqueueAutomaticUpdate(ctx, status); err != nil {
			c.logger.Debug("automatic runtime update deferred")
		}
	}
	return nil
}

func (c *Controller) enqueueAutomaticUpdate(ctx context.Context, status dto.AgentUpdateStatusDTO) error {
	c.runtimeAutoUpdateMu.Lock()
	var earlyOutcome *managedruntime.UpdateOutcome
	defer func() {
		c.runtimeAutoUpdateMu.Unlock()
		if earlyOutcome != nil {
			status.Available = false
			status.LastOutcome = earlyOutcome
			c.publishRuntimeStatus(ctx, status)
		}
	}()
	cap, err := c.automaticRuntimeCapability(status.AgentName, status.RuntimeID)
	if err != nil {
		return err
	}
	policy, err := c.runtimeAutoUpdateStore.Get(ctx, status.AgentName, cap.RuntimeID)
	if err != nil || !policy.Enabled {
		return err
	}
	if _, active := c.updateJobStore.GetActive(status.AgentName); active {
		return nil
	}
	if policy.AttemptedVersion == status.LatestVersion {
		return nil
	}
	active, effective, _, err := c.runtimeVersions(ctx, status.AgentName, *cap.Managed)
	if err != nil || effective != status.EffectiveVersion {
		return err
	}
	id := uuid.NewString()
	policy.AttemptedVersion = status.LatestVersion
	policy.Outcome = &managedruntime.UpdateOutcome{ID: id, Status: managedruntime.UpdateOutcomeRunning, PreviousVersion: effective, TargetVersion: status.LatestVersion}
	if err := c.runtimeAutoUpdateStore.Save(ctx, status.AgentName, policy); err != nil {
		return err
	}
	if err := c.validateAutomaticRuntimeTarget(ctx, *cap.Managed, status.LatestVersion); err != nil {
		earlyOutcome, err = c.retainEarlyAutomaticFailure(ctx, status.AgentName, policy)
		return err
	}
	spec := *cap.Managed
	spec.NativeBinary = ""
	guard := c.automaticActivationGuard(status.AgentName, cap.RuntimeID, spec, active, effective, id, status.LatestVersion)
	_, err = c.updateJobStore.enqueueAutomatic(ctx, status.AgentName, spec, status.LatestVersion, effective, id, cap.RuntimeID, guard)
	if err != nil {
		var saveErr error
		earlyOutcome, saveErr = c.retainEarlyAutomaticFailure(ctx, status.AgentName, policy)
		if saveErr != nil {
			return saveErr
		}
	}
	return err
}

func (c *Controller) automaticRuntimeCapability(name, runtimeID string) (agents.RuntimeUpdateCapability, error) {
	ag, found := c.agentRegistry.Get(name)
	if !found || !ag.Enabled() {
		return agents.RuntimeUpdateCapability{}, ErrAgentNotFound
	}
	cap := agents.RuntimeUpdateCapabilities(ag)
	if !c.automaticUpdateSupported(cap) || cap.RuntimeID != runtimeID {
		return agents.RuntimeUpdateCapability{}, ErrRuntimeUpdateUnsupported
	}
	return cap, nil
}

func (c *Controller) retainEarlyAutomaticFailure(ctx context.Context, name string, policy managedruntime.AutoUpdatePolicy) (*managedruntime.UpdateOutcome, error) {
	policy.Outcome.Status, policy.Outcome.FinishedAt = managedruntime.UpdateOutcomeFailed, time.Now().UTC()
	if err := c.runtimeAutoUpdateStore.Save(ctx, name, policy); err != nil {
		return nil, err
	}
	return policy.Outcome, nil
}

func (c *Controller) automaticActivationGuard(name, runtimeID string, spec agents.ManagedNPMRuntimeSpec, active, effective, attemptID, target string) func(context.Context, func() error) error {
	return func(ctx context.Context, commit func() error) error {
		c.runtimeAutoUpdateMu.Lock()
		defer c.runtimeAutoUpdateMu.Unlock()
		ag, found := c.agentRegistry.Get(name)
		if !found || !ag.Enabled() || agents.RuntimeUpdateCapabilities(ag).RuntimeID != runtimeID {
			return ErrRuntimeUpdateUnsupported
		}
		policy, err := c.runtimeAutoUpdateStore.Get(ctx, name, runtimeID)
		if err != nil {
			return err
		}
		if !policy.Enabled || policy.AttemptedVersion != target || policy.Outcome == nil || policy.Outcome.ID != attemptID {
			return errors.New("automatic runtime consent withdrawn")
		}
		currentActive, currentEffective, _, err := c.runtimeVersions(ctx, name, spec)
		if err != nil {
			return err
		}
		if currentActive != active || currentEffective != effective {
			return errors.New("runtime selection changed")
		}
		return commit()
	}
}

func (c *Controller) retainAutomaticOutcome(job dto.AgentUpdateJobDTO) {
	if !job.Automatic || c.runtimeAutoUpdateStore == nil {
		return
	}
	c.runtimeAutoUpdateMu.Lock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ag, found := c.agentRegistry.Get(job.AgentName)
	if !found {
		c.runtimeAutoUpdateMu.Unlock()
		return
	}
	policy, err := c.runtimeAutoUpdateStore.Get(ctx, job.AgentName, job.RuntimeID)
	if err != nil || policy.Outcome == nil || policy.Outcome.ID != job.JobID {
		c.runtimeAutoUpdateMu.Unlock()
		return
	}
	policy.Outcome.Status, policy.Outcome.FinishedAt = string(job.Status), time.Now().UTC()
	err = c.runtimeAutoUpdateStore.Save(ctx, job.AgentName, policy)
	c.runtimeAutoUpdateMu.Unlock()
	if err != nil {
		c.logger.Warn("automatic runtime outcome could not be persisted")
		return
	}
	c.publishRuntimeStatus(ctx, dto.AgentUpdateStatusDTO{AgentName: job.AgentName, DisplayName: ag.DisplayName(), RuntimeID: policy.RuntimeID, LastOutcome: policy.Outcome})
}

func (c *Controller) disableAutomaticUpdatesLocked(ctx context.Context, name string) error {
	if c.runtimeAutoUpdateStore == nil {
		return nil
	}
	return c.setAgentAutomaticUpdatesLocked(ctx, name, false)
}

func (c *Controller) settleInterruptedAutomaticUpdate(ctx context.Context, status *dto.AgentUpdateStatusDTO) {
	if c.runtimeAutoUpdateStore == nil || status.LastOutcome == nil || status.LastOutcome.Status != managedruntime.UpdateOutcomeRunning {
		return
	}
	c.runtimeAutoUpdateMu.Lock()
	defer c.runtimeAutoUpdateMu.Unlock()
	if c.updateJobStore != nil {
		if _, active := c.updateJobStore.GetActive(status.AgentName); active {
			return
		}
	}
	policy, err := c.runtimeAutoUpdateStore.Get(ctx, status.AgentName, status.RuntimeID)
	if err != nil || policy.Outcome == nil || policy.Outcome.Status != managedruntime.UpdateOutcomeRunning {
		return
	}
	policy.Outcome.Status, policy.Outcome.FinishedAt = managedruntime.UpdateOutcomeInterrupted, time.Now().UTC()
	if err := c.runtimeAutoUpdateStore.Save(ctx, status.AgentName, policy); err == nil {
		status.LastOutcome = policy.Outcome
	}
}
