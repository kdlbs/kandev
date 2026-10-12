package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

// ErrInstallScriptEmpty is returned when the agent has no install script defined.
var ErrInstallScriptEmpty = errors.New("agent has no install script")

// ErrJobStoreUnavailable is returned when SetJobBroadcaster hasn't been called
// — i.e. handlers haven't registered yet, so there's no WS hub to stream to.
var ErrJobStoreUnavailable = errors.New("install job store unavailable")

// EnqueueInstall starts (or returns the existing) async install job for the
// named agent. The script is hard-coded by the agent type, so no user input is
// ever shelled. Clients subscribe to WS notifications (agent.install.started,
// agent.install.output, agent.install.finished) for live progress, or poll
// /agent-install/jobs/:id for a snapshot.
func (c *Controller) EnqueueInstall(name string) (*dto.InstallJobDTO, error) {
	if c.jobStore == nil {
		return nil, ErrJobStoreUnavailable
	}
	ag, ok := c.agentRegistry.Get(name)
	if !ok {
		return nil, ErrAgentNotFound
	}
	script, err := c.installScriptForExecution(context.Background(), ag)
	if err != nil {
		return nil, err
	}
	script = strings.TrimSpace(script)
	if script == "" {
		return nil, ErrInstallScriptEmpty
	}
	job, err := c.jobStore.Enqueue(name, script)
	if err != nil {
		return nil, err
	}
	// Get() takes the store mutex, so it's race-free; calling job.snapshot()
	// directly here would read job.Status without the lock while the spawned
	// goroutine may be writing it.
	if snap, ok := c.jobStore.Get(job.ID); ok {
		return snap, nil
	}
	// Job was evicted between Enqueue and Get (extremely unlikely with the
	// configured retention window). Fall back to the unguarded snapshot.
	snap := job.snapshot()
	return &snap, nil
}

func (c *Controller) installScriptForSettings(ctx context.Context, ag agents.Agent) (string, error) {
	provider, ok := ag.(agents.SelectedRuntimeInstallCommandProvider)
	if !ok {
		return ag.InstallScript(), nil
	}
	command, err := c.selectedRuntimeInstallCommand(ctx, provider)
	if err != nil {
		return "", err
	}
	return buildCommandString(command.Args()), nil
}

func (c *Controller) installScriptForExecution(ctx context.Context, ag agents.Agent) (string, error) {
	provider, ok := ag.(agents.SelectedRuntimeInstallCommandProvider)
	if !ok {
		return ag.InstallScript(), nil
	}
	command, err := c.selectedRuntimeInstallCommand(ctx, provider)
	if err != nil {
		return "", err
	}
	args := command.Args()
	if err := managedruntime.PrepareNPMProjectPrefix(args); err != nil {
		return "", fmt.Errorf("prepare managed npm install prefix: %w", err)
	}
	return buildCommandString(args), nil
}

func (c *Controller) selectedRuntimeInstallCommand(
	ctx context.Context,
	provider agents.SelectedRuntimeInstallCommandProvider,
) (agents.Command, error) {
	reader, ok := c.managedRuntimeSelections.(managedruntime.OpenCodeSelectionReader)
	if !ok {
		return agents.Command{}, errors.New("OpenCode runtime selection is unavailable for installation")
	}
	selection, found, err := reader.GetOpenCodeSelection(ctx)
	if err != nil {
		return agents.Command{}, fmt.Errorf("read OpenCode runtime selection for installation: %w", err)
	}
	if !found {
		return agents.Command{}, errors.New("OpenCode runtime selection has not been initialized")
	}
	command, err := provider.SettingsInstallCommand(selection)
	if err != nil {
		return agents.Command{}, fmt.Errorf("resolve OpenCode install command: %w", err)
	}
	return command, nil
}

// ListInstallJobs returns a snapshot of every active or recently-finished
// install job. Used by the UI on page mount to recover in-flight installs.
func (c *Controller) ListInstallJobs() []dto.InstallJobDTO {
	if c.jobStore == nil {
		return nil
	}
	return c.jobStore.ListAll()
}

// GetInstallJob returns a snapshot of one job by ID, or nil if not found.
func (c *Controller) GetInstallJob(id string) (*dto.InstallJobDTO, bool) {
	if c.jobStore == nil {
		return nil, false
	}
	return c.jobStore.Get(id)
}
