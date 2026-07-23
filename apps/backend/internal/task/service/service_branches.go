package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
)

// AddBranchToTaskRequest carries the parameters for adding a new branch
// (worktree) to an existing task. The (RepositoryID, CheckoutBranch) pair
// must not already exist on the task.
//
// RepositoryID, LocalPath, and GitHubURL are alternative ways to identify the
// target repository — mirrors the create_task input shape. When RepositoryID
// is empty and either LocalPath or GitHubURL is set, the service resolves to
// (or creates) the matching repository in the task's workspace. Pure
// RepositoryID is the fast path; the other two are agent-ergonomic
// alternatives so callers don't have to look up the UUID first.
type AddBranchToTaskRequest struct {
	TaskID         string
	RepositoryID   string
	LocalPath      string
	GitHubURL      string
	BaseBranch     string
	CheckoutBranch string
}

// AddBranchToTask appends a new task_repositories row to an existing task,
// effectively adding a branch (or a second branch of an already-attached repo)
// without recreating the task. The row's position is set to the next free
// slot after the highest existing position.
//
// Returns the persisted TaskRepository on success.
//
// Constraints:
//   - TaskID is required.
//   - RepositoryID, LocalPath, and GitHubURL are alternative ways to identify
//     the target repository. All three are optional: when none is supplied
//     the service defaults to the task's existing repository (the only one
//     for single-repo tasks; the primary by lowest position otherwise).
//     Multi-repo tasks must pass one explicitly — defaulting would be
//     ambiguous. LocalPath and GitHubURL are resolved (find-or-create)
//     through the same workspace-scoped path used by create_task.
//   - The (TaskID, RepositoryID, BaseBranch, CheckoutBranch) tuple must be
//     unique on the task — duplicates surface as a typed error so callers
//     can render a user-friendly message instead of a raw DB error.
//   - The repository must belong to the task's workspace. The resolution
//     path (ResolveRepositoryRef → resolveRepoInput) enforces this for the
//     LocalPath / GitHubURL flows; for the RepositoryID fast path the
//     workspace-membership check happens in resolveBranchRepo below.
func (s *Service) AddBranchToTask(ctx context.Context, req AddBranchToTaskRequest) (*models.TaskRepository, error) {
	unlock := s.lockWorkspaceSources(req.TaskID)
	defer unlock()
	task, existing, cleanupOrphanRepo, err := s.prepareBranchAdd(ctx, &req)
	if err != nil {
		return nil, err
	}
	if err := s.workspaceSourcesIdle(ctx, task.ID); err != nil {
		return nil, err
	}
	repo, err := s.resolveBranchRepo(ctx, task, req.RepositoryID)
	if err != nil {
		cleanupOrphanRepo()
		return nil, err
	}

	baseBranch := req.BaseBranch
	if baseBranch == "" {
		baseBranch = repo.DefaultBranch
	}
	// Gate before any task_repositories write: with no resolvable base branch
	// the materialize step can only fail, leaving an orphan row + (worse) an
	// orphan provider repository_url Repository row. Fail loud so the MCP
	// caller can supply base_branch explicitly or pre-register the repo.
	if baseBranch == "" {
		cleanupOrphanRepo()
		return nil, fmt.Errorf("cannot resolve base_branch for repository %q: pass base_branch explicitly", repoLabelOrID(repo, req.RepositoryID))
	}
	checkoutBranch := resolveCheckoutBranchOrAutoName(req.CheckoutBranch, existing, req.RepositoryID, baseBranch)
	if err := validateWorkspaceSourceBranches(baseBranch, checkoutBranch); err != nil {
		cleanupOrphanRepo()
		return nil, err
	}

	_, dupErr := scanForBranchAddDuplicate(existing, req.RepositoryID, baseBranch, checkoutBranch, repo)
	if dupErr != nil {
		cleanupOrphanRepo()
		return nil, dupErr
	}

	taskRepo := &models.TaskRepository{
		RepositoryID:   req.RepositoryID,
		BaseBranch:     baseBranch,
		CheckoutBranch: checkoutBranch,
		Metadata:       map[string]interface{}{},
	}
	batch := &models.WorkspaceSourceBatch{TaskID: task.ID, Sources: []models.WorkspaceSource{{Repository: taskRepo}}}
	if err := s.rejectRuntimeNameCollisions(ctx, append(existing, taskRepo), batch); err != nil {
		cleanupOrphanRepo()
		return nil, err
	}
	materialize := s.materializeLegacyBranch
	if s.workspaceSourceMaterializer != nil {
		materialize = s.materializeWorkspaceSources
	}
	_, err = s.commitWorkspaceSourceBatch(ctx, task, batch, func(context.Context) { cleanupOrphanRepo() }, materialize)
	if err != nil {
		return nil, err
	}
	return taskRepo, nil
}

// materializeLegacyBranch adapts the legacy one-row worktree capability to
// the batch commit path. The existing materializer owns its live/pre-launch
// distinction, preserving add_branch's historical no-op-before-launch rule.
func (s *Service) materializeLegacyBranch(ctx context.Context, taskID string, batch *models.WorkspaceSourceBatch) (*WorkspaceSourceMaterializationResult, error) {
	for _, source := range batch.Sources {
		if source.Repository != nil {
			if err := s.materializeBranch(ctx, taskID, source.Repository.ID); err != nil {
				return nil, err
			}
		}
	}
	return &WorkspaceSourceMaterializationResult{}, nil
}

// prepareBranchAdd runs the input validation, executor-shield check, and
// repository_id defaulting that AddBranchToTask needs before it can pick a
// branch name and write the row. Returns the loaded task plus the current
// task_repositories list so the caller doesn't re-query.
//
// Mutates req.RepositoryID when it was omitted and a single-repo default
// applies; multi-repo tasks return an error here instead of silently
// guessing the wrong repo.
//
// Also returns a cleanupOrphanRepo callback that the caller invokes on any
// error path AFTER prepareBranchAdd returns successfully: when this call
// created a fresh Repository row via the GitHubURL/LocalPath
// find-or-create path, the callback deletes it so a later validation or
// materialize failure does not leave an orphan provider row in the DB. The
// callback is never nil and is safe to call on the happy path (no-op when
// nothing was created here).
func (s *Service) prepareBranchAdd(ctx context.Context, req *AddBranchToTaskRequest) (*models.Task, []*models.TaskRepository, func(), error) {
	noopCleanup := func() {}
	if req.TaskID == "" {
		return nil, nil, noopCleanup, fmt.Errorf("task_id is required")
	}
	task, err := s.tasks.GetTask(ctx, req.TaskID)
	if err != nil {
		return nil, nil, noopCleanup, fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		// Wrap the repo-tier sentinel so handlers can classify via errors.Is
		// rather than substring-matching the formatted UUID.
		return nil, nil, noopCleanup, fmt.Errorf("%w: %s", taskrepo.ErrTaskNotFound, req.TaskID)
	}
	// Executor gate runs BEFORE repository resolution so a rejection on a
	// non-worktree executor can't leak an orphan Repository row into the
	// workspace via FindOrCreateRepository / CreateRepository inside
	// ResolveRepositoryRef. Mirrors the "keeps the DB clean" invariant
	// documented on requireWorktreeExecutorForBranchAdd itself.
	if err := s.requireWorktreeExecutorForBranchAdd(ctx, req.TaskID); err != nil {
		return nil, nil, noopCleanup, err
	}
	cleanupOrphanRepo, err := s.resolveBranchAddRepoRef(ctx, task, req, noopCleanup)
	if err != nil {
		return nil, nil, noopCleanup, err
	}
	existing, err := s.taskRepos.ListTaskRepositories(ctx, req.TaskID)
	if err != nil {
		cleanupOrphanRepo()
		return nil, nil, noopCleanup, fmt.Errorf("list existing branches: %w", err)
	}
	if req.RepositoryID == "" {
		req.RepositoryID, err = s.defaultRepositoryIDForTask(existing)
		if err != nil {
			cleanupOrphanRepo()
			return nil, nil, noopCleanup, err
		}
	}
	return task, existing, cleanupOrphanRepo, nil
}

// resolveBranchRepo loads the Repository for req.RepositoryID and verifies
// it belongs to the task's workspace. Extracted from AddBranchToTask so the
// repository / workspace check stays separated from the row-uniqueness
// logic for the linter's complexity budget.
func (s *Service) resolveBranchRepo(ctx context.Context, task *models.Task, repositoryID string) (*models.Repository, error) {
	repo, err := s.repoEntities.GetRepository(ctx, repositoryID)
	if err != nil {
		return nil, fmt.Errorf("get repository: %w", err)
	}
	if repo == nil || repo.WorkspaceID != task.WorkspaceID {
		return nil, fmt.Errorf("%w: repository %q does not belong to task's workspace", taskrepository.ErrRepositoryNotFound, repositoryID)
	}
	return repo, nil
}

// resolveCheckoutBranchOrAutoName picks the branch name for the new
// task_repositories row. When the caller passed an explicit value it's
// taken verbatim; when omitted AND a row for (repo, base, "") already
// exists, the helper auto-names the new row branch-N so add_branch is
// ergonomic for agents that only express intent ("make a new branch").
func resolveCheckoutBranchOrAutoName(
	requested string,
	existing []*models.TaskRepository,
	repositoryID, baseBranch string,
) string {
	if requested != "" {
		return requested
	}
	if hasRowForRepoBase(existing, repositoryID, baseBranch) {
		return autoBranchName(existing, repositoryID, baseBranch)
	}
	return ""
}

// scanForBranchAddDuplicate walks the existing task_repositories rows once
// to compute the next position AND surface a typed duplicate error when
// the prospective (repo, base, checkout) tuple already exists. The single
// pass keeps the hot AddBranchToTask path compact and the linter happy
// (the inline version pushed the parent over the cyclop budget).
func scanForBranchAddDuplicate(
	existing []*models.TaskRepository,
	repositoryID, baseBranch, checkoutBranch string,
	repo *models.Repository,
) (nextPosition int, dupErr error) {
	// Branches whose names differ only in unsafe characters (e.g. "feat/a" vs
	// "feat-a") sanitize to the same on-disk slug and would collide at
	// `git worktree add`. SanitizeBranchSlug documents that the service layer
	// must reject those duplicates — the exact-string check below would let
	// both rows land in task_repositories.
	newSlug := worktree.SanitizeBranchSlug(checkoutBranch)
	for _, tr := range existing {
		if tr.RepositoryID == repositoryID &&
			tr.BaseBranch == baseBranch &&
			tr.CheckoutBranch == checkoutBranch {
			return 0, branchAddDuplicateError(repo, repositoryID, baseBranch, checkoutBranch)
		}
		// Slug-collision check is NOT scoped by base_branch — worktree path
		// derivation (TaskWorktreePath) uses only repo + slug, so two rows
		// on the same repo with sibling slugs would land on the same on-disk
		// directory regardless of which base_branch they were cut from.
		if tr.RepositoryID == repositoryID &&
			checkoutBranch != "" && tr.CheckoutBranch != "" &&
			tr.CheckoutBranch != checkoutBranch &&
			worktree.SanitizeBranchSlug(tr.CheckoutBranch) == newSlug && newSlug != "" {
			return 0, fmt.Errorf(
				"checkout branch %q conflicts with existing branch %q on this task (both resolve to the same worktree path)",
				checkoutBranch, tr.CheckoutBranch,
			)
		}
		if tr.Position >= nextPosition {
			nextPosition = tr.Position + 1
		}
	}
	return nextPosition, nil
}

// branchAddDuplicateError returns the user-facing duplicate-attachment
// error, preferring the more descriptive "on branch %q" form when a branch
// name is available. Extracted so the scan loop in
// scanForBranchAddDuplicate stays single-purpose.
func branchAddDuplicateError(
	repo *models.Repository, repositoryID, baseBranch, checkoutBranch string,
) error {
	label := repoLabelOrID(repo, repositoryID)
	branchLabel := checkoutBranch
	if branchLabel == "" {
		branchLabel = baseBranch
	}
	if branchLabel == "" {
		return fmt.Errorf("repository %q is already attached to this task", label)
	}
	return fmt.Errorf("repository %q on branch %q is already attached to this task", label, branchLabel)
}

// materializeBranch triggers the worktree manager + agentctl rescan.
//
// Pre-launch tasks (no task_environments row, or no materializer wired) keep
// the legacy best-effort behaviour: failures are logged and the next session
// launch will pick the worktree up via prepareMultiRepo. Returns nil in both
// of those cases so the task_repositories row sticks around for the launcher
// to act on.
//
// Live worktree-executor tasks (already launched) surface the materialize
// failure to the caller so AddBranchToTask can roll back the dangling
// task_repositories row — leaving it in place produced the silent-success
// orphan that the original bug report describes.
func (s *Service) materializeBranch(ctx context.Context, taskID, taskRepositoryID string) error {
	if s.branchMaterializer == nil {
		return nil
	}
	// Snapshot launch state BEFORE MaterializeBranch so a caller-side cancel
	// during materialize can't poison the post-failure check: launch state
	// cannot change during the synchronous materialize call, but the DB
	// query in taskAlreadyLaunched would fail with a context error if we
	// asked after the fact — and that would silently demote a live-task
	// rollback to a best-effort no-op, leaving the orphan row the PR exists
	// to prevent.
	alreadyLaunched := s.taskAlreadyLaunched(ctx, taskID)
	if err := s.branchMaterializer.MaterializeBranch(ctx, taskID, taskRepositoryID); err != nil {
		if !alreadyLaunched {
			s.logger.Warn("branch materialization failed; worktree will be created on next session launch",
				zap.String("task_id", taskID),
				zap.String("task_repository_id", taskRepositoryID),
				zap.Error(err))
			return nil
		}
		s.logger.Error("branch materialization failed on a live task; rolling back",
			zap.String("task_id", taskID),
			zap.String("task_repository_id", taskRepositoryID),
			zap.Error(err))
		return fmt.Errorf("materialize branch: %w", err)
	}
	return nil
}

// taskAlreadyLaunched reports whether the task already has a task_environments
// row whose executor type is fixed — the signal that an executor backend is
// running and won't replay prepareMultiRepo for the new branch on its own.
// Pre-launch tasks (no row, or no executor type yet) return false so a
// materialize failure stays best-effort.
func (s *Service) taskAlreadyLaunched(ctx context.Context, taskID string) bool {
	if s.taskEnvironments == nil {
		return false
	}
	env, err := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil || env == nil {
		return false
	}
	return env.ExecutorType != ""
}

// defaultRepositoryIDForTask resolves the implicit repository to use when
// AddBranchToTask is called without one. Single-repo tasks resolve trivially;
// multi-repo tasks force the caller to disambiguate so we never silently
// pick the wrong repo. Returns an error rather than guessing in ambiguous
// cases so the agent sees a clear "specify repository_id" message.
func (s *Service) defaultRepositoryIDForTask(existing []*models.TaskRepository) (string, error) {
	if len(existing) == 0 {
		return "", fmt.Errorf("repository_id is required: task has no repositories attached yet")
	}
	uniqueRepos := make(map[string]bool, len(existing))
	for _, tr := range existing {
		uniqueRepos[tr.RepositoryID] = true
	}
	if len(uniqueRepos) > 1 {
		return "", fmt.Errorf("repository_id is required: task has %d repositories — pick one explicitly", len(uniqueRepos))
	}
	// Single repo (possibly with multiple branches already attached). Return
	// the primary (lowest-position) row's repo id — they all share the same
	// repository_id by definition of the uniqueness check above.
	primary := existing[0]
	for _, tr := range existing {
		if tr.Position < primary.Position {
			primary = tr
		}
	}
	return primary.RepositoryID, nil
}

// requireWorktreeExecutorForBranchAdd rejects add_branch_to_task calls on
// tasks whose execution environment isn't the worktree executor. Sibling
// worktrees are a git-worktree-specific layout — containerized executors
// (docker, sprites, ssh) bind one workspace path per task and would
// silently ignore the new sibling. Returning early here also keeps the
// DB clean: without the guard, the row lands in task_repositories but
// nothing materializes on disk, leaving an orphan that breaks invariants.
//
// Tasks that haven't launched yet (no TaskEnvironment row) are permitted:
// the executor type isn't fixed yet, and rejecting them would block the
// pre-launch "set up multi-branch first, then start the agent" flow.
func (s *Service) requireWorktreeExecutorForBranchAdd(ctx context.Context, taskID string) error {
	if s.taskEnvironments == nil {
		return nil
	}
	env, err := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("look up task environment: %w", err)
	}
	if env == nil || env.ExecutorType == "" {
		return nil
	}
	if env.ExecutorType != string(models.ExecutorTypeWorktree) {
		return fmt.Errorf(
			"add_branch_to_task is only supported on the worktree executor; this task uses %q",
			env.ExecutorType,
		)
	}
	return nil
}

// hasRowForRepoBase reports whether `existing` already contains a row for
// the (repo, base, "") triple — i.e. an attached worktree on the same base
// branch with no explicit checkout_branch. Used to decide whether an
// otherwise-empty checkout_branch should be auto-named so the new row
// doesn't collide.
func hasRowForRepoBase(existing []*models.TaskRepository, repositoryID, baseBranch string) bool {
	for _, tr := range existing {
		if tr.RepositoryID == repositoryID && tr.BaseBranch == baseBranch && tr.CheckoutBranch == "" {
			return true
		}
	}
	return false
}

// autoBranchName picks a fresh `branch-<random>` name for a new
// task_repositories row when the caller didn't specify checkout_branch.
// Uses a short random suffix instead of a sequential counter so the name
// is unpredictable — predictable names like `branch-2` invite collisions
// when concurrent agents add branches simultaneously and leak ordering
// information into the worktree path on disk.
//
// On the astronomically unlikely event of a collision with an existing
// row for the same (repo, base), retry up to a few times before giving
// up and letting the caller surface the duplicate-name error.
//
// The naming intentionally avoids encoding the base branch name to keep
// the slug-derived directory short and stable across base renames.
func autoBranchName(existing []*models.TaskRepository, repositoryID, baseBranch string) string {
	for range 5 {
		candidate := "branch-" + worktree.SmallSuffix(3)
		if !branchNameExistsForRepoBase(existing, repositoryID, baseBranch, candidate) {
			return candidate
		}
	}
	return "branch-" + worktree.SmallSuffix(3)
}

// branchNameExistsForRepoBase reports whether `existing` already has a row
// for the given (repo, base, checkout) triple — used by autoBranchName to
// reject random candidates that collide with prior rows.
func branchNameExistsForRepoBase(existing []*models.TaskRepository, repositoryID, baseBranch, checkoutBranch string) bool {
	for _, tr := range existing {
		if tr.RepositoryID == repositoryID && tr.BaseBranch == baseBranch && tr.CheckoutBranch == checkoutBranch {
			return true
		}
	}
	return false
}

// repoLabelOrID returns a human-readable repository label for error messages,
// preferring "owner/name" then bare name, falling back to the raw UUID.
func repoLabelOrID(repo *models.Repository, repositoryID string) string {
	if repo == nil {
		return repositoryID
	}
	if repo.ProviderOwner != "" && repo.ProviderName != "" {
		return repo.ProviderOwner + "/" + repo.ProviderName
	}
	if repo.Name != "" {
		return repo.Name
	}
	return repositoryID
}

// resolveBranchAddRepoRef resolves LocalPath / GitHubURL on the add_branch
// request to a concrete RepositoryID up front so the rest of the flow
// (duplicate scan, row insert) operates on a stable UUID. Returns a cleanup
// callback that the caller must invoke on any error path between resolution
// and a successful task_repositories insert — when the call created a fresh
// Repository row, the callback deletes it so workspace state doesn't accrue
// dangling provider rows. Pure no-op when RepositoryID was already set or
// neither alternative identifier was supplied.
func (s *Service) resolveBranchAddRepoRef(
	ctx context.Context, task *models.Task, req *AddBranchToTaskRequest, noopCleanup func(),
) (func(), error) {
	if req.RepositoryID != "" || (req.LocalPath == "" && req.GitHubURL == "") {
		return noopCleanup, nil
	}
	resolvedID, resolvedBranch, createdByUs, resolveErr := s.ResolveRepositoryRef(ctx, task.WorkspaceID, TaskRepositoryInput{
		LocalPath:               req.LocalPath,
		GitHubURL:               req.GitHubURL,
		BaseBranch:              req.BaseBranch,
		ResolveProviderDefaults: true,
	})
	if resolveErr != nil {
		return noopCleanup, fmt.Errorf("resolve repository: %w", resolveErr)
	}
	req.RepositoryID = resolvedID
	if req.BaseBranch == "" {
		req.BaseBranch = resolvedBranch
	}
	// Register cleanup only when this call actually inserted the Repository
	// row. The previous heuristic (workspace pre-list snapshot vs resolvedID)
	// had a TOCTOU race: a concurrent add_branch for the same GitHub URL
	// could create the row between the snapshot and FindOrCreateRepository,
	// then we'd register cleanup against another request's data and delete
	// it on any later failure here. createdByUs is set by FindOrCreateRepository
	// / CreateRepository themselves and can't be spoofed by concurrency.
	if resolvedID == "" || !createdByUs {
		return noopCleanup, nil
	}
	createdID := resolvedID
	cleanup := func() {
		// Route through Service.DeleteRepository so the matching
		// repository.deleted event fires; the create path published
		// repository.created in CreateRepository, and skipping the
		// delete event would leave WS subscribers / frontend caches
		// holding a phantom row.
		//
		// Detach from the captured ctx via WithoutCancel: the rollback
		// often runs precisely because the caller's ctx was cancelled
		// (timeout, client disconnect), and reusing it would also kill
		// the cleanup query and orphan the row we just created.
		rollbackCtx := context.WithoutCancel(ctx)
		if delErr := s.DeleteRepository(rollbackCtx, createdID); delErr != nil {
			s.logger.Warn("AddBranchToTask: failed to roll back created repository",
				zap.String("repository_id", createdID),
				zap.Error(delErr))
		}
	}
	return cleanup, nil
}
