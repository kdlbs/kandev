package controller

import (
	"context"
	"github.com/Masterminds/semver/v3"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"strings"
)

func (c *Controller) previewHarnessUpdate(
	ctx context.Context,
	ag agents.Agent,
	spec agents.HarnessUpdateSpec,
	targetVersion string,
	useDefault bool,
) (*dto.AgentUpdatePreviewDTO, error) {
	if strings.TrimSpace(spec.Package) == "" || spec.UpdateCommand.IsEmpty() {
		return nil, ErrRuntimeUpdateUnsupported
	}
	if targetVersion != "" || useDefault {
		return nil, ErrRuntimeUpdateTargetInvalid
	}
	latest, _ := c.resolveHarnessLatest(ctx, spec.Package)
	current := ""
	if caps, found := c.runtimeUpdater.CurrentCapabilities(ag.ID()); found {
		current = caps.AgentVersion
	}
	command := spec.UpdateCommand.Args()
	return &dto.AgentUpdatePreviewDTO{
		UpdateMode: dto.AgentUpdateModeSelfUpdate,
		AgentName:  ag.ID(), Package: spec.Package, CurrentVersion: current,
		EffectiveVersion: current, StableLatestVersion: latest,
		Operation:         harnessUpdateOperation(current),
		AvailableVersions: []dto.AgentUpdateVersionDTO{},
		Command:           command, CommandString: buildCommandString(command),
	}, nil
}

func harnessUpdateOperation(current string) string {
	if current == "" {
		return string(managedruntime.OperationRepair)
	}
	if _, err := semver.NewVersion(current); err != nil {
		return string(managedruntime.OperationRepair)
	}
	// The installed harness selects its own release channel. The stable npm
	// version is a reference only; the trusted updater decides whether work is
	// required for the active channel.
	return string(managedruntime.OperationUpdate)
}

func (c *Controller) resolveHarnessLatest(ctx context.Context, pkg string) (string, error) {
	resolver, ok := c.runtimeUpdater.(interface {
		ResolveHarnessLatest(context.Context, string) (string, error)
	})
	if !ok {
		return "", ErrRuntimeUpdaterUnavailable
	}
	return validateRuntimeUpdateLatest(resolver.ResolveHarnessLatest(ctx, pkg))
}

func (c *Controller) enqueueHarnessUpdate(
	ag agents.Agent, spec agents.HarnessUpdateSpec,
	targetVersion string, useDefault bool,
) (*dto.AgentUpdateJobDTO, error) {
	if strings.TrimSpace(spec.Package) == "" || spec.UpdateCommand.IsEmpty() {
		return nil, ErrRuntimeUpdateUnsupported
	}
	if targetVersion != "" || useDefault {
		return nil, ErrRuntimeUpdateTargetInvalid
	}
	if active, found := c.updateJobStore.GetActive(ag.ID()); found {
		return active, nil
	}
	job, err := c.updateJobStore.EnqueueHarness(ag.ID(), spec, ag.Runtime().Cmd)
	if err != nil {
		return nil, err
	}
	if snapshot, found := c.updateJobStore.Get(job.ID); found {
		return snapshot, nil
	}
	snapshot := job.snapshot()
	return &snapshot, nil
}
