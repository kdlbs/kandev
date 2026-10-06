package controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"go.uber.org/zap"
	"os"
	"strings"
)

func (c *Controller) PreviewAgentUpdateFamily(
	ctx context.Context,
	name string,
	targetVersion string,
	targetFamily string,
) (*dto.AgentUpdatePreviewDTO, error) {
	if targetFamily == "" {
		return c.PreviewAgentUpdate(ctx, name, targetVersion)
	}
	if targetFamily != string(managedruntime.OpenCodeFamilyV2) || name != agents.OpenCodeACPAgentID {
		return nil, ErrRuntimeMigrationUnsupported
	}
	return c.previewOpenCodeV2Migration(ctx, name, targetVersion)
}

func (c *Controller) previewOpenCodeV2Migration(
	ctx context.Context,
	name string,
	targetVersion string,
) (*dto.AgentUpdatePreviewDTO, error) {
	if c.runtimeUpdater == nil {
		return nil, ErrRuntimeUpdaterUnavailable
	}
	openCode, ok := c.agentRegistry.Get(name)
	if !ok {
		return nil, ErrAgentNotFound
	}
	provider, ok := openCode.(*agents.OpenCodeACP)
	if !ok {
		return nil, ErrRuntimeMigrationUnsupported
	}
	reader, ok := c.managedRuntimeSelections.(managedruntime.OpenCodeSelectionReader)
	if !ok {
		return nil, ErrRuntimeMigrationUnsupported
	}
	selection, found, err := reader.GetOpenCodeSelection(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: read OpenCode selection: %v", ErrRuntimeUpdatePreviewFailed, err)
	}
	if !found || selection.Family != managedruntime.OpenCodeFamilyV1 {
		return nil, ErrRuntimeMigrationUnsupported
	}
	spec, err := provider.ManagedNPMRuntimeForFamily(managedruntime.OpenCodeFamilyV2)
	if err != nil {
		return nil, err
	}
	current := selection.SelectedVersion
	if selection.Source == managedruntime.OpenCodeSourceNative {
		if caps, ok := c.runtimeUpdater.CurrentCapabilities(name); ok {
			current = caps.AgentVersion
		}
	}
	if current == "" {
		current = selection.AppliedDefaultVersion
	}
	catalogue, exactCatalogue, err := c.resolveRuntimeCatalogue(ctx, spec.Package, current, spec.DefaultVersionOrPinned())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRuntimeUpdatePreviewFailed, err)
	}
	target := strings.TrimSpace(targetVersion)
	if target == "" {
		target = spec.DefaultVersionOrPinned()
	}
	target, err = resolvePreviewTarget(catalogue, exactCatalogue, target, spec.DefaultVersionOrPinned(), false)
	if err != nil {
		return nil, err
	}
	command := spec.CacheUpdateCommand(target).Args()
	return &dto.AgentUpdatePreviewDTO{
		UpdateMode: dto.AgentUpdateModePinned, AgentName: name, Package: spec.Package, CurrentVersion: current,
		DefaultVersion: spec.DefaultVersionOrPinned(), ActiveVersion: current,
		EffectiveVersion: current, TargetVersion: target,
		Family: string(selection.Family), Source: string(selection.Source),
		TargetFamily: string(managedruntime.OpenCodeFamilyV2), RuntimeRevision: selection.Revision,
		MigrationAvailable: true, Operation: string(managedruntime.OperationMigrate),
		AvailableVersions: runtimeVersionDTOs(catalogue), Command: command,
		CommandString: buildCommandString(command),
	}, nil
}

func (c *Controller) managedRuntimeState(
	ctx context.Context,
	name string,
	managed agents.ManagedNPMRuntimeAgent,
) (agents.ManagedNPMRuntimeSpec, managedruntime.OpenCodeFamily, managedruntime.OpenCodeSource, uint64, string, bool, error) {
	spec := managed.ManagedNPMRuntime()
	provider, ok := c.agentRegistry.Get(name)
	openCode, isOpenCode := provider.(*agents.OpenCodeACP)
	if !ok || !isOpenCode {
		return spec, "", "", 0, "", false, nil
	}
	reader, ok := c.managedRuntimeSelections.(managedruntime.OpenCodeSelectionReader)
	if !ok {
		return spec, "", "", 0, "", false, nil
	}
	selection, found, err := reader.GetOpenCodeSelection(ctx)
	if err != nil || !found {
		return spec, "", "", 0, "", false, err
	}
	spec, err = openCode.ManagedNPMRuntimeForFamily(selection.Family)
	if err != nil {
		return agents.ManagedNPMRuntimeSpec{}, "", "", 0, "", false, err
	}
	active := selection.SelectedVersion
	if selection.Source == managedruntime.OpenCodeSourceNative && c.runtimeUpdater != nil {
		if caps, ok := c.runtimeUpdater.CurrentCapabilities(name); ok {
			active = caps.AgentVersion
		}
	}
	return spec, selection.Family, selection.Source, selection.Revision, active,
		selection.Family == managedruntime.OpenCodeFamilyV1, nil
}

func (c *Controller) EnqueueOpenCodeMigration(
	ctx context.Context,
	name string,
	targetVersion string,
	expectedRevision uint64,
) (*dto.AgentUpdateJobDTO, error) {
	if name != agents.OpenCodeACPAgentID || expectedRevision == 0 {
		return nil, ErrRuntimeMigrationUnsupported
	}
	return c.enqueueOpenCodeMigration(ctx, name, targetVersion, expectedRevision)
}

func (c *Controller) enqueueOpenCodeMigration(
	ctx context.Context,
	name string,
	targetVersion string,
	expectedRevision uint64,
) (*dto.AgentUpdateJobDTO, error) {
	if c.updateJobStore == nil || c.runtimeUpdater == nil {
		return nil, ErrRuntimeMigrationUnsupported
	}
	if c.openCodeMigrationGuard == nil {
		return nil, ErrRuntimeUpdaterUnavailable
	}
	openCode, selection, err := c.openCodeMigrationSelection(ctx, name, expectedRevision)
	if err != nil {
		return nil, err
	}
	spec, err := openCode.ManagedNPMRuntimeForFamily(managedruntime.OpenCodeFamilyV2)
	if err != nil {
		return nil, err
	}
	targetVersion = strings.TrimSpace(targetVersion)
	if targetVersion == "" {
		targetVersion = spec.DefaultVersionOrPinned()
	}
	if err := c.validateAgentUpdateTarget(ctx, spec, targetVersion); err != nil {
		return nil, err
	}
	return c.enqueueValidatedOpenCodeMigration(name, spec, targetVersion, selection.Revision)
}

func (c *Controller) openCodeMigrationSelection(
	ctx context.Context,
	name string,
	expectedRevision uint64,
) (*agents.OpenCodeACP, managedruntime.OpenCodeSelection, error) {
	provider, ok := c.agentRegistry.Get(name)
	if !ok {
		return nil, managedruntime.OpenCodeSelection{}, ErrAgentNotFound
	}
	openCode, ok := provider.(*agents.OpenCodeACP)
	if !ok {
		return nil, managedruntime.OpenCodeSelection{}, ErrRuntimeMigrationUnsupported
	}
	reader, ok := c.managedRuntimeSelections.(managedruntime.OpenCodeSelectionReader)
	if !ok {
		return nil, managedruntime.OpenCodeSelection{}, ErrRuntimeMigrationUnsupported
	}
	selection, found, err := reader.GetOpenCodeSelection(ctx)
	if err != nil {
		return nil, managedruntime.OpenCodeSelection{}, fmt.Errorf("read OpenCode runtime selection: %w", err)
	}
	if !found || selection.Revision != expectedRevision || selection.Family != managedruntime.OpenCodeFamilyV1 {
		return nil, managedruntime.OpenCodeSelection{}, managedruntime.ErrOpenCodeSelectionRevisionConflict
	}
	return openCode, selection, nil
}

func (c *Controller) enqueueValidatedOpenCodeMigration(
	name string,
	spec agents.ManagedNPMRuntimeSpec,
	targetVersion string,
	expectedRevision uint64,
) (*dto.AgentUpdateJobDTO, error) {
	job, err := c.updateJobStore.EnqueueOpenCodeMigration(name, spec, targetVersion, expectedRevision)
	if err != nil {
		return nil, err
	}
	if snapshot, found := c.updateJobStore.Get(job.ID); found {
		return snapshot, nil
	}
	snapshot := job.snapshot()
	return &snapshot, nil
}

func (c *Controller) hasOpenCodeSelection(ctx context.Context) bool {
	reader, ok := c.managedRuntimeSelections.(managedruntime.OpenCodeSelectionReader)
	if !ok {
		return false
	}
	_, found, err := reader.GetOpenCodeSelection(ctx)
	return err != nil || found
}

func (u *hostRuntimeUpdater) ProbeIsolated(
	ctx context.Context,
	agentName string,
	command agents.Command,
) (caps hostutility.AgentCapabilities, err error) {
	root, err := os.MkdirTemp("", "kandev-opencode-probe-")
	if err != nil {
		return hostutility.AgentCapabilities{}, fmt.Errorf("create isolated OpenCode probe: %w", err)
	}
	defer func() {
		err = isolatedProbeCleanupResult(err, os.RemoveAll(root), func(cleanupErr error) {
			if u.logger != nil {
				u.logger.Warn("could not remove isolated OpenCode probe directory", zap.Error(cleanupErr))
			}
		})
	}()
	return u.host.ProbeIsolatedWithCommand(ctx, agentName, command, root)
}

func isolatedProbeCleanupResult(probeErr, cleanupErr error, warn func(error)) error {
	if cleanupErr == nil {
		return probeErr
	}
	if probeErr == nil {
		if warn != nil {
			warn(cleanupErr)
		}
		return nil
	}
	return errors.Join(probeErr, fmt.Errorf("remove isolated OpenCode probe: %w", cleanupErr))
}

func (c *Controller) selectedOpenCodeVersions(ctx context.Context, packageName string) (active, effective string, found bool, err error) {
	reader, ok := c.managedRuntimeSelections.(managedruntime.OpenCodeSelectionReader)
	if !ok {
		return "", "", false, nil
	}
	selection, found, err := reader.GetOpenCodeSelection(ctx)
	if err != nil || !found || selection.Package != packageName {
		return "", "", false, err
	}
	effective = selection.SelectedVersion
	if effective == "" {
		effective = selection.AppliedDefaultVersion
	}
	return selection.SelectedVersion, effective, true, nil
}
