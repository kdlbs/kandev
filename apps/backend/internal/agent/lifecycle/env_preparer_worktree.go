package lifecycle

import (
	"context"
	"fmt"
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
	steps, ok := p.validateWorktreeRequest(req, stepIdx, totalSteps, onProgress, steps)
	if !ok {
		return &EnvPrepareResult{Success: false, Steps: steps, ErrorMessage: steps[len(steps)-1].Error, Duration: time.Since(start)}, nil
	}
	stepIdx++

	// Steps 2 (and optional sync): Create worktree (with optional pre-sync).
	wt, steps, stepIdx, err := p.createWorktreeWithSync(ctx, req, stepIdx, totalSteps, onProgress, steps)
	if err != nil {
		return &EnvPrepareResult{Success: false, Steps: steps, ErrorMessage: err.Error(), Duration: time.Since(start)}, nil
	}

	workspacePath := wt.Path
	mainRepoGitDir := filepath.Join(req.RepositoryPath, ".git")

	// Step 3 (optional): Fetch PR branch is handled inside worktree.Manager.Create
	// via CreateRequest.CheckoutBranch. If it was set and failed, Create() would have
	// returned an error above. We report a success step here for UI visibility.
	if req.CheckoutBranch != "" {
		step := beginStep("Fetch PR branch")
		step.Command = fmt.Sprintf("git fetch origin %s", req.CheckoutBranch)
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
		steps = runSetupScriptStep(ctx, req, workspacePath, resolvedScript, stepIdx, totalSteps, onProgress, steps, p.logger)
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

// validateWorktreeRequest runs the "Validate repository" step.
// Returns the updated steps and ok=true on success, ok=false when a failure step was appended.
func (p *WorktreePreparer) validateWorktreeRequest(
	req *EnvPrepareRequest,
	stepIdx, totalSteps int,
	onProgress PrepareProgressCallback,
	steps []PrepareStep,
) ([]PrepareStep, bool) {
	step := beginStep("Validate repository")
	reportProgress(onProgress, step, stepIdx, totalSteps)
	if req.RepositoryPath == "" {
		completeStepError(&step, "no repository path provided")
		steps = append(steps, step)
		reportProgress(onProgress, step, stepIdx, totalSteps)
		return steps, false
	}
	if p.worktreeMgr == nil {
		completeStepError(&step, "worktree manager not available")
		steps = append(steps, step)
		reportProgress(onProgress, step, stepIdx, totalSteps)
		return steps, false
	}
	completeStepSuccess(&step)
	steps = append(steps, step)
	reportProgress(onProgress, step, stepIdx, totalSteps)
	return steps, true
}

// createWorktreeWithSync creates the worktree (with optional sync-base-branch step).
// Returns the created worktree, updated steps, the next step index after creation, and any error.
func (p *WorktreePreparer) createWorktreeWithSync(
	ctx context.Context,
	req *EnvPrepareRequest,
	stepIdx, totalSteps int,
	onProgress PrepareProgressCallback,
	steps []PrepareStep,
) (*worktree.Worktree, []PrepareStep, int, error) {
	var syncStep *PrepareStep
	syncStepIndex := -1
	if req.PullBeforeWorktree {
		s := beginStep("Sync base branch")
		if req.BaseBranch != "" {
			s.Command = fmt.Sprintf("git fetch origin %s && git pull origin %s", req.BaseBranch, req.BaseBranch)
		} else {
			s.Command = "git fetch origin && git pull"
		}
		reportProgress(onProgress, s, stepIdx, totalSteps)
		syncStep = &s
		syncStepIndex = stepIdx
		stepIdx++
	}

	step := beginStep("Create worktree")
	step.Command = "git worktree add"
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
	steps = finalizeSyncStep(syncStep, syncStepIndex, totalSteps, onProgress, steps, err)
	if err != nil {
		completeStepError(&step, err.Error())
		steps = append(steps, step)
		reportProgress(onProgress, step, stepIdx, totalSteps)
		p.logger.Error("worktree creation failed", zap.String("task_id", req.TaskID), zap.Error(err))
		return nil, steps, stepIdx, err
	}

	completeStepSuccess(&step)
	steps = append(steps, step)
	reportProgress(onProgress, step, stepIdx, totalSteps)
	return wt, steps, stepIdx + 1, nil
}

// finalizeSyncStep closes out the optional sync step after worktree.Create returns.
func finalizeSyncStep(
	syncStep *PrepareStep,
	syncStepIndex, totalSteps int,
	onProgress PrepareProgressCallback,
	steps []PrepareStep,
	createErr error,
) []PrepareStep {
	if syncStep == nil {
		return steps
	}
	if syncStep.Status == PrepareStepRunning {
		if createErr != nil {
			completeStepError(syncStep, "sync interrupted by worktree creation failure")
		} else {
			completeStepSuccess(syncStep)
		}
	}
	steps = append(steps, *syncStep)
	reportProgress(onProgress, *syncStep, syncStepIndex, totalSteps)
	return steps
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
