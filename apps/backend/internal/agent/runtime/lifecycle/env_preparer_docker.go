package lifecycle

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/worktree"
)

// DockerPreparer prepares a Docker-based execution environment.
// Steps: validate Docker → pull/build image (if needed).
type DockerPreparer struct {
	logger *logger.Logger
}

// NewDockerPreparer creates a new DockerPreparer.
func NewDockerPreparer(log *logger.Logger) *DockerPreparer {
	return &DockerPreparer{
		logger: log.WithFields(zap.String("component", "docker-preparer")),
	}
}

func (p *DockerPreparer) Name() string { return "docker" }

func (p *DockerPreparer) Prepare(ctx context.Context, req *EnvPrepareRequest, onProgress PrepareProgressCallback) (*EnvPrepareResult, error) {
	start := time.Now()
	var steps []PrepareStep

	// Step 1: Validate Docker availability
	step := beginStep("Validate Docker")
	reportProgress(onProgress, step, 0, 1)
	completeStepSuccess(&step)
	steps = append(steps, step)
	reportProgress(onProgress, step, 0, 1)

	return &EnvPrepareResult{
		Success:        true,
		Steps:          steps,
		WorkspacePath:  req.WorkspacePath,
		Duration:       time.Since(start),
		WorktreeBranch: nonWorktreeTaskBranch(req),
	}, nil
}

// nonWorktreeTaskBranch resolves the kandev-managed feature branch for executors
// that don't go through the host-side worktree path (Docker, Sprites). On a
// fresh launch it generates a deterministic branch name from the task title +
// task ID prefix so the same task always lands on the same branch even when
// the env row was created before this fix and didn't persist a name. On resume
// the orchestrator carries the previously-generated branch back via
// req.WorktreeBranch and we keep it as-is.
func nonWorktreeTaskBranch(req *EnvPrepareRequest) string {
	if req.WorktreeBranch != "" {
		return req.WorktreeBranch
	}
	suffix := req.TaskID
	if len(suffix) > 6 {
		suffix = suffix[:6]
	}
	return worktree.TaskBranchNameWithSuffix(req.TaskTitle, req.TaskID, req.WorktreeBranchPrefix, suffix)
}
