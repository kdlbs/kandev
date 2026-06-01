package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
)

// AddBranchToTaskRequest carries the parameters for adding a new branch
// (worktree) to an existing task. The (RepositoryID, CheckoutBranch) pair
// must not already exist on the task.
type AddBranchToTaskRequest struct {
	TaskID         string
	RepositoryID   string
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
//   - RepositoryID is optional. When omitted, the service defaults to the
//     task's existing repository (the only one for single-repo tasks; the
//     primary by lowest position otherwise). Multi-repo tasks must pass an
//     explicit RepositoryID — defaulting would be ambiguous.
//   - The (TaskID, RepositoryID, BaseBranch, CheckoutBranch) tuple must be
//     unique on the task — duplicates surface as a typed error so callers
//     can render a user-friendly message instead of a raw DB error.
//   - The repository must belong to the task's workspace; the caller is
//     responsible for that check (service_tasks.resolveRepoInput enforces it
//     during create, this method assumes the same upstream gating).
func (s *Service) AddBranchToTask(ctx context.Context, req AddBranchToTaskRequest) (*models.TaskRepository, error) {
	task, existing, err := s.prepareBranchAdd(ctx, &req)
	if err != nil {
		return nil, err
	}
	repo, err := s.resolveBranchRepo(ctx, task, req.RepositoryID)
	if err != nil {
		return nil, err
	}

	baseBranch := req.BaseBranch
	if baseBranch == "" {
		baseBranch = repo.DefaultBranch
	}
	checkoutBranch := resolveCheckoutBranchOrAutoName(req.CheckoutBranch, existing, req.RepositoryID, baseBranch)

	nextPosition, dupErr := scanForBranchAddDuplicate(existing, req.RepositoryID, baseBranch, checkoutBranch, repo)
	if dupErr != nil {
		return nil, dupErr
	}

	taskRepo := &models.TaskRepository{
		TaskID:         req.TaskID,
		RepositoryID:   req.RepositoryID,
		BaseBranch:     baseBranch,
		CheckoutBranch: checkoutBranch,
		Position:       nextPosition,
		Metadata:       map[string]interface{}{},
	}
	if err := s.taskRepos.CreateTaskRepository(ctx, taskRepo); err != nil {
		s.logger.Error("AddBranchToTask: failed to create task repository",
			zap.String("task_id", req.TaskID),
			zap.String("repository_id", req.RepositoryID),
			zap.String("checkout_branch", req.CheckoutBranch),
			zap.Error(err))
		return nil, fmt.Errorf("create task repository: %w", err)
	}

	// Publish task.updated so the kanban / sidebar re-render with the new row.
	s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)

	s.materializeBranchBestEffort(ctx, req.TaskID, taskRepo.ID)
	return taskRepo, nil
}

// prepareBranchAdd runs the input validation, executor-shield check, and
// repository_id defaulting that AddBranchToTask needs before it can pick a
// branch name and write the row. Returns the loaded task plus the current
// task_repositories list so the caller doesn't re-query.
//
// Mutates req.RepositoryID when it was omitted and a single-repo default
// applies; multi-repo tasks return an error here instead of silently
// guessing the wrong repo.
func (s *Service) prepareBranchAdd(ctx context.Context, req *AddBranchToTaskRequest) (*models.Task, []*models.TaskRepository, error) {
	if req.TaskID == "" {
		return nil, nil, fmt.Errorf("task_id is required")
	}
	task, err := s.tasks.GetTask(ctx, req.TaskID)
	if err != nil {
		return nil, nil, fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		// Wrap the repo-tier sentinel so handlers can classify via errors.Is
		// rather than substring-matching the formatted UUID.
		return nil, nil, fmt.Errorf("%w: %s", taskrepo.ErrTaskNotFound, req.TaskID)
	}
	if err := s.requireWorktreeExecutorForBranchAdd(ctx, req.TaskID); err != nil {
		return nil, nil, err
	}
	existing, err := s.taskRepos.ListTaskRepositories(ctx, req.TaskID)
	if err != nil {
		return nil, nil, fmt.Errorf("list existing branches: %w", err)
	}
	if req.RepositoryID == "" {
		req.RepositoryID, err = s.defaultRepositoryIDForTask(existing)
		if err != nil {
			return nil, nil, err
		}
	}
	return task, existing, nil
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
		return nil, fmt.Errorf("repository %q does not belong to task's workspace", repositoryID)
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

// materializeBranchBestEffort triggers the worktree manager + agentctl
// rescan asynchronously-ish. Failures are logged but don't unwind the DB
// write — the next session launch will pick the worktree up via
// prepareMultiRepo, so a transient rescan failure is recoverable.
func (s *Service) materializeBranchBestEffort(ctx context.Context, taskID, taskRepositoryID string) {
	if s.branchMaterializer == nil {
		return
	}
	if err := s.branchMaterializer.MaterializeBranch(ctx, taskID, taskRepositoryID); err != nil {
		s.logger.Warn("branch materialization failed; worktree will be created on next session launch",
			zap.String("task_id", taskID),
			zap.String("task_repository_id", taskRepositoryID),
			zap.Error(err))
	}
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
