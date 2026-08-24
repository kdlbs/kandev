package backendapp

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// sshTaskDirReclaimerAdapter implements taskservice.SSHTaskDirReclaimer by
// opening a short-lived SSH connection for one reclamation attempt. It is a
// separate connection from the executor's: by the time a terminal cleanup
// reaches this phase every session has been stopped and its client closed.
type sshTaskDirReclaimerAdapter struct {
	logger *logger.Logger
}

func (a *sshTaskDirReclaimerAdapter) ReclaimTaskDir(
	ctx context.Context,
	req taskservice.SSHTaskDirReclaimRequest,
) (taskservice.SSHTaskDirReclaimResult, error) {
	// A launch cannot happen without a trusted host key, so an empty pinned
	// fingerprint here means the recorded connection is not one Kandev
	// established. Dialling anyway would accept whatever key answered and then
	// delete a directory on it.
	if req.HostFingerprint == "" {
		return taskservice.SSHTaskDirReclaimResult{}, errors.New(
			"ssh reclaim: no pinned host fingerprint recorded for " + req.Host)
	}
	target, err := lifecycle.ResolveSSHTarget(lifecycle.SSHConnConfig{
		Host:              req.Host,
		Port:              req.Port,
		User:              req.User,
		IdentitySource:    lifecycle.SSHIdentitySource(req.IdentitySource),
		IdentityFile:      req.IdentityFile,
		ProxyJump:         req.ProxyJump,
		PinnedFingerprint: req.HostFingerprint,
	})
	if err != nil {
		return taskservice.SSHTaskDirReclaimResult{}, fmt.Errorf("resolve ssh target %s: %w", req.Host, err)
	}
	client, err := lifecycle.DialSSH(ctx, target)
	if err != nil {
		return taskservice.SSHTaskDirReclaimResult{}, fmt.Errorf("dial ssh %s: %w", req.Host, err)
	}
	defer func() { _ = client.Close() }()

	reclaimer := lifecycle.NewSSHTaskDirReclaimer(client, req.Shell, a.logger)
	outcome, verdict, err := reclaimer.Reclaim(ctx, req.WorkdirRoot, req.TaskDir)
	if err != nil {
		return taskservice.SSHTaskDirReclaimResult{}, err
	}
	return taskservice.SSHTaskDirReclaimResult{
		Removed:    outcome == lifecycle.SSHReclaimOutcomeRemoved,
		SkipReason: string(verdict.Reason),
		Detail:     verdict.Detail,
	}, nil
}
