package lifecycle

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/worktree"
)

// WorktreePreparer prepares a worktree-based execution environment.
// Steps: validate repository → create/reuse worktree → run setup script (if any).
type WorktreePreparer struct {
	worktreeMgr *worktree.Manager
	logger      *logger.Logger
}

// NewWorktreePreparer creates a new WorktreePreparer.
func NewWorktreePreparer(worktreeMgr *worktree.Manager, log *logger.Logger) *WorktreePreparer {
	return &WorktreePreparer{
		worktreeMgr: worktreeMgr,
		logger:      log.WithFields(zap.String("component", "worktree-preparer")),
	}
}

func (p *WorktreePreparer) Name() string { return "worktree" }

func (p *WorktreePreparer) Prepare(ctx context.Context, req *EnvPrepareRequest, onProgress PrepareProgressCallback) (*EnvPrepareResult, error) {
	start := time.Now()
	var steps []PrepareStep

	totalSteps := 2 // validate + create worktree
	if req.SetupScript != "" {
		totalSteps++
	}

	// Step 1: Validate repository path
	step := beginStep("Validate repository")
	reportProgress(onProgress, step, 0, totalSteps)
	if req.RepositoryPath == "" {
		completeStepError(&step, "no repository path provided")
		steps = append(steps, step)
		return &EnvPrepareResult{Success: false, Steps: steps, ErrorMessage: step.Error, Duration: time.Since(start)}, nil
	}
	completeStepSuccess(&step)
	steps = append(steps, step)
	reportProgress(onProgress, step, 0, totalSteps)

	// Step 2: Create or reuse worktree
	step = beginStep("Create worktree")
	reportProgress(onProgress, step, 1, totalSteps)
	if p.worktreeMgr == nil {
		completeStepError(&step, "worktree manager not available")
		steps = append(steps, step)
		reportProgress(onProgress, step, 1, totalSteps)
		// Fall back: use repository path directly
		return &EnvPrepareResult{
			Success:       true,
			Steps:         steps,
			WorkspacePath: req.RepositoryPath,
			Duration:      time.Since(start),
		}, nil
	}

	completeStepSuccess(&step)
	steps = append(steps, step)
	reportProgress(onProgress, step, 1, totalSteps)

	// Step 3: Run setup script (if provided)
	workspacePath := req.WorkspacePath
	if workspacePath == "" {
		workspacePath = req.RepositoryPath
	}
	if req.SetupScript != "" {
		step = beginStep("Run setup script")
		reportProgress(onProgress, step, 2, totalSteps)
		output, err := runSetupScript(ctx, req.SetupScript, workspacePath, req.Env)
		if err != nil {
			completeStepError(&step, err.Error())
			step.Output = output
			steps = append(steps, step)
			reportProgress(onProgress, step, 2, totalSteps)
			p.logger.Warn("setup script failed", zap.String("task_id", req.TaskID), zap.Error(err))
		} else {
			step.Output = output
			completeStepSuccess(&step)
			steps = append(steps, step)
			reportProgress(onProgress, step, 2, totalSteps)
		}
	}

	return &EnvPrepareResult{
		Success:       true,
		Steps:         steps,
		WorkspacePath: workspacePath,
		Duration:      time.Since(start),
	}, nil
}
