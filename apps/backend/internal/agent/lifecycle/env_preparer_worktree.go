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
	// Multi-repo (>=2 explicit specs) takes a dedicated path that creates one
	// worktree per repo and rolls back on partial failure. Single-repo and
	// repo-less requests use the original sequence unchanged.
	if specs := req.Repositories; len(specs) >= 2 {
		return p.prepareMultiRepo(ctx, req, specs, onProgress)
	}

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

	// worktree.Manager.Create already executed the per-repo setup script via
	// its ScriptMessageHandler (recorded as a script_execution message). To
	// avoid running the same script twice we blank RepoSetupScript on a
	// request copy before resolving the prepare script — the placeholder
	// substitutes to empty and the resolved script is skipped via
	// isScriptEffectivelyEmpty.
	scriptReq := *req
	scriptReq.RepoSetupScript = ""
	resolvedScript := resolvePreparerSetupScript(&scriptReq, workspacePath)
	if resolvedScript != "" {
		totalSteps++
		steps = runSetupScriptStep(ctx, &scriptReq, workspacePath, resolvedScript, stepIdx, totalSteps, onProgress, steps, p.logger)
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
		FallbackBaseBranch:   req.DefaultBranch,
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
	if wt.BaseBranchFallbackWarning != "" {
		step.Warning = wt.BaseBranchFallbackWarning
		step.WarningDetail = wt.BaseBranchFallbackDetail
	}
	// wt.FetchWarning is surfaced in the separate "Fetch PR branch" step
	// rendered later in the pipeline. It can only be non-empty when
	// req.CheckoutBranch != "" (it is set inside fetchBranchToLocal), so the
	// two warnings always co-occur and FetchWarning is never silently dropped.
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

// prepareMultiRepo prepares N worktrees, one per spec. On any per-repo
// failure, all already-created worktrees for this preparation are rolled back
// and the request is reported as failed with the originating error.
//
// Per-repo setup scripts (RepoSetupScript) are executed by the worktree
// manager during Create() via its script handler, which streams output to a
// "script_execution" chat message. The env preparer does not run them as a
// separate step — doing so caused the script to run twice per repo. The
// task-level setup script (req.SetupScript) is intentionally NOT run from
// this path — that resolution depends on the task root layout and is handled
// by the caller in a future phase.
//
// On success the legacy single-worktree fields on EnvPrepareResult mirror
// Worktrees[0] so existing downstream code (manager_launch.launchApplyPrepareResult,
// orchestrator persistence) continues to consume the first repo's worktree
// without changes.
func (p *WorktreePreparer) prepareMultiRepo(
	ctx context.Context,
	req *EnvPrepareRequest,
	specs []RepoPrepareSpec,
	onProgress PrepareProgressCallback,
) (*EnvPrepareResult, error) {
	start := time.Now()
	var steps []PrepareStep

	if p.worktreeMgr == nil {
		step := beginStep("Validate repositories")
		completeStepError(&step, "worktree manager not available")
		steps = append(steps, step)
		reportProgress(onProgress, step, 0, 1)
		return &EnvPrepareResult{
			Success:      false,
			Steps:        steps,
			ErrorMessage: step.Error,
			Duration:     time.Since(start),
		}, nil
	}

	// Step budget: per-repo (validate + create + optional sync + optional checkout).
	// Per-repo setup scripts are executed by the worktree manager during
	// Create() (via its script handler, which also creates the chat
	// "script_execution" message). The env preparer no longer re-runs them
	// here — doing so caused the script to run twice per repo and broke
	// non-idempotent scripts (e.g. "mkdir build" on the second run).
	totalSteps := 0
	for _, spec := range specs {
		totalSteps += 2
		if spec.PullBeforeWorktree {
			totalSteps++
		}
		if spec.CheckoutBranch != "" {
			totalSteps++
		}
	}

	stepIdx := 0
	worktrees := make([]RepoWorktreeResult, 0, len(specs))
	createdIDs := make([]string, 0, len(specs))

	for _, spec := range specs {
		wt, newSteps, nextIdx, err := p.prepareOneRepo(ctx, req, spec, stepIdx, totalSteps, onProgress, steps)
		steps = newSteps
		stepIdx = nextIdx
		if err != nil {
			p.rollbackWorktrees(ctx, createdIDs)
			return &EnvPrepareResult{
				Success:      false,
				Steps:        steps,
				ErrorMessage: err.Error(),
				Duration:     time.Since(start),
			}, nil
		}
		createdIDs = append(createdIDs, wt.ID)
		worktrees = append(worktrees, RepoWorktreeResult{
			RepositoryID:   spec.RepositoryID,
			WorktreeID:     wt.ID,
			WorktreeBranch: wt.Branch,
			WorktreePath:   wt.Path,
			MainRepoGitDir: filepath.Join(spec.RepositoryPath, ".git"),
		})
	}

	// Workspace path = task root (parent of any repo subdir). All repos share
	// the same TaskDirName, so any worktree's parent works.
	workspacePath := ""
	if len(worktrees) > 0 {
		workspacePath = filepath.Dir(worktrees[0].WorktreePath)
	}

	res := &EnvPrepareResult{
		Success:       true,
		Steps:         steps,
		WorkspacePath: workspacePath,
		Duration:      time.Since(start),
		Worktrees:     worktrees,
	}
	// Mirror first repo into legacy fields for downstream consumers that
	// haven't been migrated yet.
	if len(worktrees) > 0 {
		res.WorktreeID = worktrees[0].WorktreeID
		res.WorktreeBranch = worktrees[0].WorktreeBranch
		res.MainRepoGitDir = worktrees[0].MainRepoGitDir
	}
	return res, nil
}

// prepareOneRepo runs the validate → create-worktree → fetch-PR sequence for
// a single repo within a multi-repo preparation. The per-repo setup script
// is executed by the worktree manager during Create() and is not re-run
// here. Returns the created worktree, updated steps, the next step index,
// and any error.
func (p *WorktreePreparer) prepareOneRepo(
	ctx context.Context,
	req *EnvPrepareRequest,
	spec RepoPrepareSpec,
	stepIdx, totalSteps int,
	onProgress PrepareProgressCallback,
	steps []PrepareStep,
) (*worktree.Worktree, []PrepareStep, int, error) {
	repoLabel := spec.RepoName
	if repoLabel == "" {
		repoLabel = spec.RepositoryID
	}

	// Validate
	validateStep := beginStep(fmt.Sprintf("Validate %s repository", repoLabel))
	reportProgress(onProgress, validateStep, stepIdx, totalSteps)
	if spec.RepositoryPath == "" {
		completeStepError(&validateStep, "no repository path provided")
		steps = append(steps, validateStep)
		reportProgress(onProgress, validateStep, stepIdx, totalSteps)
		return nil, steps, stepIdx + 1, fmt.Errorf("repo %q: no repository path", repoLabel)
	}
	completeStepSuccess(&validateStep)
	steps = append(steps, validateStep)
	reportProgress(onProgress, validateStep, stepIdx, totalSteps)
	stepIdx++

	// Sync (optional) + Create — reuses single-repo helper by translating spec→req.
	subReq := *req
	subReq.RepositoryID = spec.RepositoryID
	subReq.RepositoryPath = spec.RepositoryPath
	subReq.RepoName = spec.RepoName
	subReq.BaseBranch = spec.BaseBranch
	subReq.DefaultBranch = spec.DefaultBranch
	subReq.CheckoutBranch = spec.CheckoutBranch
	subReq.WorktreeID = spec.WorktreeID
	subReq.WorktreeBranchPrefix = spec.WorktreeBranchPrefix
	subReq.PullBeforeWorktree = spec.PullBeforeWorktree
	// Strip the multi-repo list to avoid re-entering the multi-repo branch.
	subReq.Repositories = nil

	wt, steps, stepIdx, err := p.createWorktreeWithSync(ctx, &subReq, stepIdx, totalSteps, onProgress, steps)
	if err != nil {
		return nil, steps, stepIdx, err
	}

	// PR fetch step (mirrors single-repo path).
	if spec.CheckoutBranch != "" {
		step := beginStep(fmt.Sprintf("Fetch PR branch (%s)", repoLabel))
		step.Command = fmt.Sprintf("git fetch origin %s", spec.CheckoutBranch)
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

	// Per-repo setup script execution is owned by the worktree manager
	// (createInTaskDir → runWorktreeSetupScript), which streams output to a
	// "script_execution" chat message. Running it here as well caused the
	// script to execute twice per repo and broke non-idempotent setups.

	return wt, steps, stepIdx, nil
}

// rollbackWorktrees removes any worktrees created during a failed multi-repo
// preparation. Best effort: failures are logged and do not interrupt teardown.
func (p *WorktreePreparer) rollbackWorktrees(ctx context.Context, ids []string) {
	for _, id := range ids {
		if err := p.worktreeMgr.RemoveByID(ctx, id, false); err != nil {
			p.logger.Warn("rollback: failed to remove worktree",
				zap.String("worktree_id", id), zap.Error(err))
		}
	}
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
