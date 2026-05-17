package lifecycle

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// SSHPreparer validates prerequisites for SSH-based execution. The heavy
// lifting (connect, agentctl upload, task dir provisioning, agentctl launch)
// happens in SSHExecutor.CreateInstance — this preparer just emits a single
// validation step so the launch UI has consistent progress feedback.
type SSHPreparer struct {
	logger *logger.Logger
}

// NewSSHPreparer creates a new SSHPreparer.
func NewSSHPreparer(log *logger.Logger) *SSHPreparer {
	return &SSHPreparer{logger: log.WithFields(zap.String("component", "ssh-preparer"))}
}

// Name implements EnvironmentPreparer.
func (p *SSHPreparer) Name() string { return "ssh" }

// Prepare implements EnvironmentPreparer. SSH executor configuration is
// validated at launch time (target / fingerprint / arch are checked inside
// CreateInstance). This step is intentionally minimal so the UI still sees
// the preparer running for every launch — preserves a uniform progress shape
// across executor types.
func (p *SSHPreparer) Prepare(_ context.Context, req *EnvPrepareRequest, onProgress PrepareProgressCallback) (*EnvPrepareResult, error) {
	p.logger.Debug("preparing ssh environment",
		zap.String("task", req.TaskID),
		zap.String("session", req.SessionID))

	started := time.Now()
	step := beginStep("Validate SSH executor configuration")
	reportProgress(onProgress, step, 0, 1)
	completeStepSuccess(&step)
	reportProgress(onProgress, step, 0, 1)
	return &EnvPrepareResult{
		Success:        true,
		Steps:          []PrepareStep{step},
		WorkspacePath:  req.WorkspacePath,
		Duration:       time.Since(started),
		WorktreeBranch: nonWorktreeTaskBranch(req),
	}, nil
}
