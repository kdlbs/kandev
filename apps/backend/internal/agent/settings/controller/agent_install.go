package controller

import (
	"errors"
	"strings"

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
	script := strings.TrimSpace(ag.InstallScript())
	if script == "" {
		return nil, ErrInstallScriptEmpty
	}
	job := c.jobStore.Enqueue(name, script)
	snap := job.snapshot()
	return &snap, nil
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
