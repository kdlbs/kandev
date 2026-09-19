package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Regression coverage for the "failed prepare leaves a permanently bricked
// workspace" defect: when a task_environments row was published ready with
// zero task_environment_repos rows (the write-side bug, fixed separately),
// validateReuseEnvironmentInventory used to treat that as a mismatch and
// refuse every subsequent launch forever. Zero rows recorded means the
// canonical inventory was never captured at all, which is recoverable by
// letting the launch rebuild it — distinct from a non-empty but wrong
// inventory, which must still be refused.
func TestValidateReuseEnvironmentInventory_ZeroRowsIsRecoverable(t *testing.T) {
	repo := newMockRepository()
	repo.taskRepositories["task-repo-1"] = &models.TaskRepository{ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1"}
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{
		TaskID:                 "task-1",
		WorkspaceReuseRequired: true,
		RepositoryID:           "repo-1",
	}
	env := &models.TaskEnvironment{ID: "env-1"}

	if err := e.validateReuseEnvironmentInventory(context.Background(), req, env); err != nil {
		t.Fatalf("validateReuseEnvironmentInventory() with zero recorded rows = %v, want nil (recoverable)", err)
	}
	if req.WorkspaceReuseRequired {
		t.Fatal("zero inventory kept WorkspaceReuseRequired=true, want fresh materialization")
	}
}

// The read-side fix must not weaken the guard's actual purpose: a non-empty
// but mismatched canonical inventory (wrong repository, wrong branch, or a
// row explicitly marked failed/deleted) is still an unsafe reuse and must be
// refused.
func TestValidateReuseEnvironmentInventory_MismatchedRowsStillRefused(t *testing.T) {
	repo := newMockRepository()
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{
		TaskID:                 "task-1",
		WorkspaceReuseRequired: true,
		RepositoryID:           "repo-1",
	}
	env := &models.TaskEnvironment{ID: "env-1"}
	repo.taskEnvironmentRepos[env.ID] = []*models.TaskEnvironmentRepo{
		{RepositoryID: "repo-other", WorktreeID: "worktree-other"},
	}

	err := e.validateReuseEnvironmentInventory(context.Background(), req, env)
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("validateReuseEnvironmentInventory() with mismatched rows = %v, want ErrWorkspaceReuseUnsafe", err)
	}
}

// A local executor publishes a branch-scoped row with an empty worktree ID, and
// may still carry a legacy empty-branch worktree row from an older capture. The
// guard must match exactly the scoped row for the branch and not also match the
// legacy row, or it over-counts and falsely refuses an otherwise-complete
// canonical inventory.
func TestValidateReuseEnvironmentInventory_ScopedBranchPlusLegacyEmptyRowAttaches(t *testing.T) {
	repo := newMockRepository()
	repo.taskRepositories["task-repo-1"] = &models.TaskRepository{ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1"}
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{
		TaskID:                 "task-1",
		WorkspaceReuseRequired: true,
		Repositories: []RepoSpec{
			{RepositoryID: "repo-1", BranchIdentitySlug: "main"},
		},
	}
	env := &models.TaskEnvironment{ID: "env-1"}
	repo.taskEnvironmentRepos[env.ID] = []*models.TaskEnvironmentRepo{
		{RepositoryID: "repo-1", BranchSlug: "main", WorktreeID: ""},
		{RepositoryID: "repo-1", BranchSlug: "", WorktreeID: "worktree-legacy"},
	}

	if err := e.validateReuseEnvironmentInventory(context.Background(), req, env); err != nil {
		t.Fatalf("validateReuseEnvironmentInventory() = %v, want nil", err)
	}
}

// TestPrepareResumeRepositorySettings_GuestSessionReuseValidatesAgainstResolvedBaseBranch
// is a regression found in review round 1: a guest session resuming a shared
// task environment (MaterializationSessionID belongs to a different session,
// so WorkspaceReuseRequired is true) on a clone-URL executor never got
// req.BaseBranch stamped, because applyResumeCloneURL — the only writer of
// req.BaseBranch outside the worktree path — short-circuits whenever
// WorkspaceReuseRequired is true. validateReuseEnvironmentInventory then derives
// the expected branch identity slug from req.BaseBranch (via
// topLevelLaunchRepoSpec/topLevelBranchIdentitySlug) and compared an empty
// fallback against the canonical inventory's real branch slug, refusing every
// resume of a genuinely-matching environment with ErrWorkspaceReuseUnsafe.
func TestPrepareResumeRepositorySettings_GuestSessionReuseValidatesAgainstResolvedBaseBranch(t *testing.T) {
	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{ID: "repo-1", LocalPath: "/tmp/repo", RemoteURL: "https://example.com/repo-1.git"}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1", Position: 0, BaseBranch: "feature-x",
	}
	repo.tasks["task-1"] = &models.Task{ID: "task-1"}
	canonicalRows := []*models.TaskEnvironmentRepo{
		{TaskEnvironmentID: "env-1", RepositoryID: "repo-1", BranchSlug: "feature-x", Status: taskEnvironmentRepoStatusActive},
	}
	env := &models.TaskEnvironment{
		ID: "env-1", TaskID: "task-1", ExecutorType: "local_docker",
		// A different session materialized this environment: this session is a
		// guest reusing it, the population WorkspaceReuseRequired gates on.
		MaterializationSessionID: "sess-owner",
		Repos:                    canonicalRows,
	}
	repo.taskEnvironments[env.ID] = env
	repo.taskEnvironmentRepos[env.ID] = canonicalRows

	task := &v1.Task{ID: "task-1"}
	session := &models.TaskSession{ID: "sess-guest", TaskID: "task-1", TaskEnvironmentID: "env-1"}
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{TaskID: "task-1", SessionID: "sess-guest", ExecutorType: "local_docker"}

	if _, _, _, err := e.prepareResumeRepositorySettings(context.Background(), task, session, req); err != nil {
		t.Fatalf("prepareResumeRepositorySettings() = %v, want nil: a guest session reusing a shared environment whose canonical inventory actually matches must not be refused", err)
	}
	if !req.WorkspaceReuseRequired {
		t.Fatalf("req.WorkspaceReuseRequired = false, want true for a live guest-session reuse")
	}
}

// TestPrepareResumeRepositorySettings_LocalExecutorResumeWithUntrackedBranchMatches
// is a second review-round regression: an ordinary (non-guest) session resume
// on a local/local_pc executor also sets WorkspaceReuseRequired whenever the
// task already has a TaskEnvironmentID, which is the common case on every
// backend restart. applyResumeRepoConfig now stamps req.RepositoryID
// unconditionally on every executor type, so topLevelLaunchRepoSpec started
// returning ok=true for local/local_pc launches that previously never reached
// validateReuseEnvironmentInventory at all. But req.BaseBranch is still never
// stamped for a non-clone-URL, non-worktree executor (LocalPreparer keeps
// whatever branch is already checked out on disk), so the derived branch
// identity slug was empty while the canonical inventory's row carried a real
// branch slug — refusing every ordinary local-executor resume with
// ErrWorkspaceReuseUnsafe. Confirmed against the live code with
// KANDEV_E2E_SKIP_FRESHNESS=1 pnpm e2e:raw tests/session/session-recovery.spec.ts,
// which failed identically before this fix.
func TestPrepareResumeRepositorySettings_LocalExecutorResumeWithUntrackedBranchMatches(t *testing.T) {
	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{ID: "repo-1", LocalPath: "/tmp/repo"}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1", Position: 0,
	}
	repo.tasks["task-1"] = &models.Task{ID: "task-1"}
	canonicalRows := []*models.TaskEnvironmentRepo{
		{TaskEnvironmentID: "env-1", RepositoryID: "repo-1", BranchSlug: "main", Status: taskEnvironmentRepoStatusActive},
	}
	env := &models.TaskEnvironment{
		ID: "env-1", TaskID: "task-1", ExecutorType: "local",
		Repos: canonicalRows,
	}
	repo.taskEnvironments[env.ID] = env
	repo.taskEnvironmentRepos[env.ID] = canonicalRows

	task := &v1.Task{ID: "task-1"}
	session := &models.TaskSession{ID: "sess-1", TaskID: "task-1", TaskEnvironmentID: "env-1"}
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{TaskID: "task-1", SessionID: "sess-1", ExecutorType: "local", WorkspaceReuseRequired: true}

	if _, _, _, err := e.prepareResumeRepositorySettings(context.Background(), task, session, req); err != nil {
		t.Fatalf("prepareResumeRepositorySettings() = %v, want nil: an ordinary local-executor resume whose canonical inventory actually matches on repository identity must not be refused for lacking a tracked branch", err)
	}
}

// TestValidateReuseEnvironmentInventory_NonWorktreeLegacyToleranceRequiresRealMismatch
// is a third regression: a non-worktree (local/local_pc) launch never
// populates TaskEnvironmentRepo.WorktreeID, since that column only has
// meaning on the worktree path. canonicalInventoryMatches used to gate its
// "legacy empty-branch" tolerance on hasBranchScopedEnvironmentRepoRows, which
// requires WorktreeID != "" to recognize a row as carrying real branch
// identity. For a non-worktree launch that check was always false — even when
// a row's BranchSlug held a real value like "main" — so the tolerance branch
// treated every non-worktree canonical inventory as "legacy", let an
// unrelated stray empty-branch-slug row (written by a distinct bug: an
// untracked-branch resume race writing a duplicate row instead of updating
// the existing branch-scoped one in place) also count as a match, and a spec
// requiring branch "main" matched twice instead of once. Confirmed against
// the live code with workflow-session-targeting.spec.ts's "delivers reuse and
// fresh initial targets" case, which failed with the same "canonical
// workspace repository inventory has no matching entry" error (got matches=2)
// before this fix.
func TestValidateReuseEnvironmentInventory_NonWorktreeLegacyToleranceRequiresRealMismatch(t *testing.T) {
	repo := newMockRepository()
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{
		TaskID:                 "task-1",
		WorkspaceReuseRequired: true,
		RepositoryID:           "repo-1",
		BaseBranch:             "main",
	}
	env := &models.TaskEnvironment{ID: "env-1"}
	repo.taskEnvironmentRepos[env.ID] = []*models.TaskEnvironmentRepo{
		{RepositoryID: "repo-1", BranchSlug: "main", Status: taskEnvironmentRepoStatusActive},
		{RepositoryID: "repo-1", BranchSlug: "", Status: taskEnvironmentRepoStatusActive},
	}

	err := e.validateReuseEnvironmentInventory(context.Background(), req, env)
	if err != nil {
		t.Fatalf("validateReuseEnvironmentInventory() = %v, want nil: a spec requiring branch %q must match only the branch-scoped row, not also the stray empty-branch row", err, "main")
	}
}
