package lifecycle

import (
	"context"
	"path/filepath"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/worktree"
)

// WorktreePreparer prepares a worktree-based execution environment.
// Steps: validate repository → create worktree → checkout PR branch (if set) → run setup script (if any).
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
	if req.PullBeforeWorktree {
		totalSteps++
	}
	if req.CheckoutBranch != "" {
		totalSteps++
	}
	// We can't resolve the setup script until we know the workspace path,
	// so we count the script step after worktree creation.
	stepIdx := 0

	// Step 1: Validate repository path
	step := beginStep("Validate repository")
	reportProgress(onProgress, step, stepIdx, totalSteps)
	if req.RepositoryPath == "" {
		completeStepError(&step, "no repository path provided")
		steps = append(steps, step)
		reportProgress(onProgress, step, stepIdx, totalSteps)
		return &EnvPrepareResult{Success: false, Steps: steps, ErrorMessage: step.Error, Duration: time.Since(start)}, nil
	}
	if p.worktreeMgr == nil {
		completeStepError(&step, "worktree manager not available")
		steps = append(steps, step)
		reportProgress(onProgress, step, stepIdx, totalSteps)
		return &EnvPrepareResult{Success: false, Steps: steps, ErrorMessage: step.Error, Duration: time.Since(start)}, nil
	}
	completeStepSuccess(&step)
	steps = append(steps, step)
	reportProgress(onProgress, step, stepIdx, totalSteps)
	stepIdx++

	var syncStep *PrepareStep
	syncStepIndex := -1
	if req.PullBeforeWorktree {
		s := beginStep("Sync base branch")
		reportProgress(onProgress, s, stepIdx, totalSteps)
		syncStep = &s
		syncStepIndex = stepIdx
		stepIdx++
	}

	// Step 2: Create worktree
	step = beginStep("Create worktree")
	reportProgress(onProgress, step, stepIdx, totalSteps)

	createReq := worktree.CreateRequest{
		TaskID:               req.TaskID,
		SessionID:            req.SessionID,
		TaskTitle:            req.TaskTitle,
		RepositoryID:         req.RepositoryID,
		RepositoryPath:       req.RepositoryPath,
		BaseBranch:           req.BaseBranch,
		CheckoutBranch:       req.CheckoutBranch,
		WorktreeBranchPrefix: req.WorktreeBranchPrefix,
		PullBeforeWorktree:   req.PullBeforeWorktree,
		WorktreeID:           req.WorktreeID,
		TaskDirName:          req.TaskDirName,
		RepoName:             req.RepoName,
	}
	if syncStep != nil {
		createReq.OnSyncProgress = func(event worktree.SyncProgressEvent) {
			applySyncProgressEvent(syncStep, event)
			reportProgress(onProgress, *syncStep, syncStepIndex, totalSteps)
		}
	}

	wt, err := p.worktreeMgr.Create(ctx, createReq)
	if syncStep != nil {
		if syncStep.Status == PrepareStepRunning {
			if err != nil {
				completeStepError(syncStep, "sync interrupted by worktree creation failure")
			} else {
				completeStepSuccess(syncStep)
			}
		}
		steps = append(steps, *syncStep)
		reportProgress(onProgress, *syncStep, syncStepIndex, totalSteps)
	}
	if err != nil {
		completeStepError(&step, err.Error())
		steps = append(steps, step)
		reportProgress(onProgress, step, stepIdx, totalSteps)
		p.logger.Error("worktree creation failed",
			zap.String("task_id", req.TaskID),
			zap.Error(err))
		return &EnvPrepareResult{Success: false, Steps: steps, ErrorMessage: err.Error(), Duration: time.Since(start)}, nil
	}

	completeStepSuccess(&step)
	steps = append(steps, step)
	reportProgress(onProgress, step, stepIdx, totalSteps)
	stepIdx++

	workspacePath := wt.Path
	mainRepoGitDir := filepath.Join(req.RepositoryPath, ".git")

	// Step 3 (optional): Fetch PR branch is handled inside worktree.Manager.Create
	// via CreateRequest.CheckoutBranch. If it was set and failed, Create() would have
	// returned an error above. We report a success step here for UI visibility.
	if req.CheckoutBranch != "" {
		step = beginStep("Fetch PR branch")
		reportProgress(onProgress, step, stepIdx, totalSteps)
		if wt.FetchWarning != "" {
			step.Warning = wt.FetchWarning
			step.WarningDetail = wt.FetchWarningDetail
		}
		completeStepSuccess(&step)
		steps = append(steps, step)
		reportProgress(onProgress, step, stepIdx, totalSteps)
		stepIdx++
	}

	// Step N: Run setup script (if provided)
	resolvedScript := resolvePreparerSetupScript(req, workspacePath)
	if resolvedScript != "" {
		totalSteps++
		step = beginStep("Run setup script")
		reportProgress(onProgress, step, stepIdx, totalSteps)
		output, scriptErr := runSetupScript(ctx, resolvedScript, workspacePath, req.Env)
		if scriptErr != nil {
			completeStepError(&step, scriptErr.Error())
			step.Output = output
			steps = append(steps, step)
			reportProgress(onProgress, step, stepIdx, totalSteps)
			p.logger.Warn("setup script failed", zap.String("task_id", req.TaskID), zap.Error(scriptErr))
			// Setup script failure is non-fatal
		} else {
			step.Output = output
			completeStepSuccess(&step)
			steps = append(steps, step)
			reportProgress(onProgress, step, stepIdx, totalSteps)
		}
	}

	return &EnvPrepareResult{
		Success:        true,
		Steps:          steps,
		WorkspacePath:  workspacePath,
		Duration:       time.Since(start),
		WorktreeID:     wt.ID,
		WorktreeBranch: wt.Branch,
		MainRepoGitDir: mainRepoGitDir,
	}, nil
}

func applySyncProgressEvent(step *PrepareStep, event worktree.SyncProgressEvent) {
	if event.StepName != "" {
		step.Name = event.StepName
	}
	step.Output = event.Output
	step.Error = event.Error
	switch event.Status {
	case worktree.SyncProgressRunning:
		step.Status = PrepareStepRunning
		if step.StartedAt == nil {
			now := time.Now()
			step.StartedAt = &now
		}
	case worktree.SyncProgressCompleted:
		now := time.Now()
		step.Status = PrepareStepCompleted
		step.EndedAt = &now
	}
}
