package backendapp

import (
	"context"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/common/logger"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// sshTaskDirReclaimerAdapter adapts the runtime SSH contract to the task
// service contract. The runtime owns connection resolution, host-key pinning,
// and guarded removal; this layer only translates domain-neutral request and
// result shapes.
type sshTaskDirReclaimerAdapter struct {
	reclaimer agentruntime.SSHTaskDirReclaimer
}

func newSSHTaskDirReclaimerAdapter(log *logger.Logger) *sshTaskDirReclaimerAdapter {
	return &sshTaskDirReclaimerAdapter{reclaimer: agentruntime.NewSSHTaskDirReclaimer(log)}
}

func (a *sshTaskDirReclaimerAdapter) ReclaimTaskDir(
	ctx context.Context,
	req taskservice.SSHTaskDirReclaimRequest,
) (taskservice.SSHTaskDirReclaimResult, error) {
	result, err := a.reclaimer.ReclaimTaskDir(ctx, agentruntime.SSHTaskDirReclaimRequest{
		Host:            req.Host,
		Port:            req.Port,
		User:            req.User,
		HostFingerprint: req.HostFingerprint,
		IdentitySource:  req.IdentitySource,
		IdentityFile:    req.IdentityFile,
		ProxyJump:       req.ProxyJump,
		Shell:           req.Shell,
		WorkdirRoot:     req.WorkdirRoot,
		TaskDir:         req.TaskDir,
	})
	if err != nil {
		return taskservice.SSHTaskDirReclaimResult{}, err
	}
	return taskservice.SSHTaskDirReclaimResult{
		Removed:    result.Removed,
		SkipReason: result.SkipReason,
		Detail:     result.Detail,
	}, nil
}
