package lifecycle

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

// CoderPreparer publishes the lifecycle boundary; actual creation and
// readiness happen in CoderWorkspaceManager immediately before agentctl setup.
type CoderPreparer struct{ logger *logger.Logger }

func NewCoderPreparer(log *logger.Logger) *CoderPreparer { return &CoderPreparer{logger: log} }
func (p *CoderPreparer) Name() string                    { return string(SSHIdentitySourceCoder) }
func (p *CoderPreparer) Prepare(_ context.Context, req *EnvPrepareRequest, onProgress PrepareProgressCallback) (*EnvPrepareResult, error) {
	started := time.Now()
	step := beginStep("Ensure Coder workspace is ready")
	reportProgress(onProgress, step, 0, 1)
	completeStepSuccess(&step)
	reportProgress(onProgress, step, 1, 1)
	return &EnvPrepareResult{Success: true, Steps: []PrepareStep{step}, WorkspacePath: req.WorkspacePath, Duration: time.Since(started), WorktreeBranch: nonWorktreeTaskBranch(req)}, nil
}
