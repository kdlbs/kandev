package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

func TestCreate_ReuseRequiredRejectsMissingCanonicalWorktree(t *testing.T) {
	mgr := newRecreateTestManager(t)

	_, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-1",
		SessionID:      "session-2",
		RepositoryID:   "repository-1",
		RepositoryPath: t.TempDir(),
		BaseBranch:     "main",
		WorktreeID:     "canonical-worktree",
		ReuseRequired:  true,
	})
	if !errors.Is(err, ErrReuseWorktreeUnavailable) {
		t.Fatalf("Create() error = %v, want ErrReuseWorktreeUnavailable", err)
	}
}

// TestCreate_RecreatesPersistedWorktreeWhenTasksBaseWasRemoved verifies that
// normal reuse treats a removed managed tasks root as a missing worktree. The
// persisted task-root identity still identifies the original safe slot, so the
// lifecycle may recreate it rather than treating the absent validation anchor
// as a symlink-safety failure.
func TestCreate_RecreatesPersistedWorktreeWhenTasksBaseWasRemoved(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	cfg := newTestConfig(t)
	taskDirName := "task-one_recovery"
	worktreePath := filepath.Join(cfg.TasksBasePath, taskDirName, "repo-one")
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		t.Fatalf("mkdir task directory: %v", err)
	}
	runGit(t, repoPath, "worktree", "add", "-b", "recover-missing-tasks-base", worktreePath, "main")

	store := newMockStore()
	store.worktrees["canonical-worktree"] = &Worktree{
		ID:                "canonical-worktree",
		TaskID:            "task-one",
		TaskDirName:       taskDirName,
		SessionID:         "session-one",
		TaskEnvironmentID: "environment-one",
		RepositoryID:      "repository-one",
		Path:              worktreePath,
		Branch:            "recover-missing-tasks-base",
		Status:            StatusActive,
	}
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if err := os.RemoveAll(cfg.TasksBasePath); err != nil {
		t.Fatalf("remove tasks base: %v", err)
	}

	got, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:            "task-one",
		SessionID:         "session-one",
		TaskEnvironmentID: "environment-one",
		RepositoryID:      "repository-one",
		RepositoryPath:    repoPath,
		BaseBranch:        "main",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.Path != worktreePath {
		t.Fatalf("Create() path = %q, want %q", got.Path, worktreePath)
	}
	if !mgr.IsValid(worktreePath) {
		t.Fatalf("Create() did not recreate a valid worktree at %q", worktreePath)
	}
	owner, found, err := storageworkspaces.ReadOwnershipMarker(filepath.Dir(worktreePath))
	if err != nil {
		t.Fatalf("read recreated ownership marker: %v", err)
	}
	if !found || owner.TaskDirName != taskDirName || owner.TaskID != "task-one" {
		t.Fatalf("recreated ownership marker = %#v, found=%v", owner, found)
	}
}

// TestCreate_ReuseRequiredRejectsRemovedTasksBase confirms attach-only reuse
// remains fail-closed: it must not recreate a missing canonical checkout.
func TestCreate_ReuseRequiredRejectsRemovedTasksBase(t *testing.T) {
	cfg := newTestConfig(t)
	worktreePath := filepath.Join(cfg.TasksBasePath, "task-one_recovery", "repo-one")
	store := newMockStore()
	store.worktrees["canonical-worktree"] = &Worktree{
		ID:                "canonical-worktree",
		TaskID:            "task-one",
		TaskDirName:       "task-one_recovery",
		TaskEnvironmentID: "environment-one",
		RepositoryID:      "repository-one",
		Path:              worktreePath,
		Branch:            "recover-missing-tasks-base",
		Status:            StatusActive,
	}
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if err := os.RemoveAll(cfg.TasksBasePath); err != nil {
		t.Fatalf("remove tasks base: %v", err)
	}

	_, err = mgr.Create(context.Background(), CreateRequest{
		TaskID:            "task-one",
		TaskEnvironmentID: "environment-one",
		RepositoryID:      "repository-one",
		RepositoryPath:    t.TempDir(),
		BaseBranch:        "main",
		WorktreeID:        "canonical-worktree",
		ReuseRequired:     true,
	})
	if !errors.Is(err, ErrReuseWorktreeUnavailable) {
		t.Fatalf("Create() error = %v, want ErrReuseWorktreeUnavailable", err)
	}
}

func TestCreate_ReuseRequiredReturnsCanonicalWorktreeWithoutChangingGitState(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	worktreePath := filepath.Join(t.TempDir(), "canonical")
	runGit(t, repoPath, "worktree", "add", "-b", "reuse-required", worktreePath, "main")
	marker := filepath.Join(worktreePath, "session-a-uncommitted-marker")
	if err := os.WriteFile(marker, []byte("shared workspace state\n"), 0o600); err != nil {
		t.Fatalf("write uncommitted marker: %v", err)
	}
	before := strings.TrimSpace(runGit(t, repoPath, "worktree", "list", "--porcelain"))
	beforeBranch := strings.TrimSpace(runGit(t, worktreePath, "branch", "--show-current"))
	beforeHead := strings.TrimSpace(runGit(t, worktreePath, "rev-parse", "HEAD"))
	beforeStatus := strings.TrimSpace(runGit(t, worktreePath, "status", "--porcelain"))

	store := newMockStore()
	store.worktrees["canonical-worktree"] = &Worktree{
		ID:                "canonical-worktree",
		TaskID:            "task-1",
		TaskEnvironmentID: "environment-1",
		RepositoryID:      "repository-1",
		Path:              worktreePath,
		Branch:            "reuse-required",
		Status:            StatusActive,
	}
	mgr, err := NewManager(newTestConfig(t), store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	got, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:            "task-1",
		SessionID:         "session-2",
		TaskEnvironmentID: "environment-1",
		RepositoryID:      "repository-1",
		RepositoryPath:    repoPath,
		BaseBranch:        "main",
		WorktreeID:        "canonical-worktree",
		ReuseRequired:     true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.ID != "canonical-worktree" || got.Path != worktreePath {
		t.Fatalf("Create() = %#v, want canonical worktree", got)
	}
	_, err = mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-1",
		SessionID:      "session-3",
		RepositoryID:   "repository-1",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		WorktreeID:     "canonical-worktree",
		ReuseRequired:  true,
	})
	if !errors.Is(err, ErrReuseWorktreeUnavailable) {
		t.Fatalf("Create() without TaskEnvironmentID error = %v, want ErrReuseWorktreeUnavailable", err)
	}
	after := strings.TrimSpace(runGit(t, repoPath, "worktree", "list", "--porcelain"))
	if after != before {
		t.Fatalf("git worktree list changed during attach-only reuse\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if branch := strings.TrimSpace(runGit(t, worktreePath, "branch", "--show-current")); branch != beforeBranch {
		t.Fatalf("branch changed during attach-only reuse: before=%q after=%q", beforeBranch, branch)
	}
	if head := strings.TrimSpace(runGit(t, worktreePath, "rev-parse", "HEAD")); head != beforeHead {
		t.Fatalf("HEAD changed during attach-only reuse: before=%q after=%q", beforeHead, head)
	}
	if status := strings.TrimSpace(runGit(t, worktreePath, "status", "--porcelain")); status != beforeStatus {
		t.Fatalf("status changed during attach-only reuse: before=%q after=%q", beforeStatus, status)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("attach-only reuse lost uncommitted marker: %v", err)
	}
}

func TestCreate_ReuseRequiredRejectsCanonicalWorktreeFromAnotherBranch(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	worktreePath := filepath.Join(t.TempDir(), "canonical")
	runGit(t, repoPath, "worktree", "add", "-b", "feature/one", worktreePath, "main")
	store := newMockStore()
	store.worktrees["canonical-worktree"] = &Worktree{
		ID:                "canonical-worktree",
		TaskID:            "task-1",
		TaskEnvironmentID: "environment-1",
		RepositoryID:      "repository-1",
		Path:              worktreePath,
		Branch:            "feature/one",
		BranchSlug:        "feature-one",
		Status:            StatusActive,
	}
	mgr, err := NewManager(newTestConfig(t), store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	_, err = mgr.Create(context.Background(), CreateRequest{
		TaskID:             "task-1",
		SessionID:          "session-2",
		TaskEnvironmentID:  "environment-1",
		RepositoryID:       "repository-1",
		RepositoryPath:     repoPath,
		BaseBranch:         "main",
		WorktreeID:         "canonical-worktree",
		BranchIdentitySlug: "feature-two",
		ReuseRequired:      true,
	})
	if !errors.Is(err, ErrReuseWorktreeUnavailable) {
		t.Fatalf("Create() error = %v, want ErrReuseWorktreeUnavailable", err)
	}
}

func TestCreate_ReuseRequiredAllowsAuthorizedEnvironmentFromAnotherTask(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	cfg := newTestConfig(t)
	tasksBase := cfg.TasksBasePath

	ownerTaskDir := filepath.Join(tasksBase, "owner-task_dir")
	if err := os.MkdirAll(ownerTaskDir, 0755); err != nil {
		t.Fatalf("mkdir owner task dir: %v", err)
	}
	if err := storageworkspaces.WriteOwnershipMarker(ownerTaskDir, storageworkspaces.OwnershipMarker{
		TaskID:        "owner-task",
		TaskDirName:   "owner-task_dir",
		LayoutVersion: storageworkspaces.LayoutVersionSemantic,
	}); err != nil {
		t.Fatalf("write owner ownership marker: %v", err)
	}

	worktreePath := filepath.Join(ownerTaskDir, "repo-one")
	runGit(t, repoPath, "worktree", "add", "-b", "shared-environment", worktreePath, "main")
	store := newMockStore()
	store.worktrees["canonical-worktree"] = &Worktree{
		ID:                "canonical-worktree",
		TaskID:            "owner-task",
		TaskEnvironmentID: "shared-environment",
		RepositoryID:      "repository-1",
		Path:              worktreePath,
		Branch:            "shared-environment",
		Status:            StatusActive,
	}
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	got, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:            "inherited-task",
		SessionID:         "session-2",
		TaskEnvironmentID: "shared-environment",
		RepositoryID:      "repository-1",
		RepositoryPath:    repoPath,
		BaseBranch:        "main",
		WorktreeID:        "canonical-worktree",
		ReuseRequired:     true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v, want inherited task to attach: %v", err, err)
	}
	if got.ID != "canonical-worktree" {
		t.Fatalf("Create() worktree ID = %q, want canonical-worktree", got.ID)
	}
}

// TestCreate_ReuseRequiredRejectsStaleWorktreePathOwnedByAnotherTask
// confirms that the attach-only ReuseRequired path also validates the
// worktree path's ownership marker, rejecting a record whose on-disk
// path belongs to another live task.
func TestCreate_ReuseRequiredRejectsStaleWorktreePathOwnedByAnotherTask(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)

	cfg := newTestConfig(t)
	tasksBase := cfg.TasksBasePath

	// Create the live task's directory and ownership marker.
	liveTaskDir := filepath.Join(tasksBase, "live-task_reuse")
	if err := os.MkdirAll(liveTaskDir, 0755); err != nil {
		t.Fatalf("mkdir live task dir: %v", err)
	}
	if err := storageworkspaces.WriteOwnershipMarker(liveTaskDir, storageworkspaces.OwnershipMarker{
		TaskID:        "task-live-reuse",
		TaskDirName:   "live-task_reuse",
		LayoutVersion: storageworkspaces.LayoutVersionSemantic,
	}); err != nil {
		t.Fatalf("write live ownership marker: %v", err)
	}

	// The worktree lives inside the live task's directory, as it would in
	// production: <tasksBase>/<taskDirName>/<repoName>/
	liveWorktreePath := filepath.Join(liveTaskDir, "my-repo")
	if err := os.MkdirAll(liveWorktreePath, 0755); err != nil {
		t.Fatalf("mkdir live worktree: %v", err)
	}
	runGit(t, repoPath, "worktree", "add", "-b", "feature/reuse-stale", liveWorktreePath, "main")

	store := newMockStore()
	store.worktrees["canonical-reuse"] = &Worktree{
		ID:                "canonical-reuse",
		TaskID:            "task-stale-reuse",
		TaskEnvironmentID: "environment-1",
		RepositoryID:      "repository-1",
		Path:              liveWorktreePath,
		Branch:            "feature/reuse-stale",
		Status:            StatusActive,
	}
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	_, err = mgr.Create(context.Background(), CreateRequest{
		TaskID:            "task-attach-reuse",
		SessionID:         "session-attach",
		TaskEnvironmentID: "environment-1",
		RepositoryID:      "repository-1",
		RepositoryPath:    repoPath,
		BaseBranch:        "main",
		WorktreeID:        "canonical-reuse",
		ReuseRequired:     true,
	})
	if !errors.Is(err, ErrWorktreePathOwnedByAnotherTask) {
		t.Fatalf("Create() error = %v, want ErrWorktreePathOwnedByAnotherTask", err)
	}
}

// TestCreate_ReuseRequiredRejectsSymlinkedWorktreePath confirms that the
// attach-only ReuseRequired path rejects a persisted worktree path whose
// existing components beneath tasksBase contain a symlink. A stale record
// whose final repo directory has been symlinked to another task's checkout
// must not follow the symlink via os.Stat/os.ReadFile inside IsValid.
func TestCreate_ReuseRequiredRejectsSymlinkedWorktreePath(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	cfg := newTestConfig(t)
	tasksBase := cfg.TasksBasePath

	// Create two task directories under tasksBase.
	liveTaskDir := filepath.Join(tasksBase, "live-task_sym")
	if err := os.MkdirAll(liveTaskDir, 0755); err != nil {
		t.Fatalf("mkdir live task dir: %v", err)
	}
	if err := storageworkspaces.WriteOwnershipMarker(liveTaskDir, storageworkspaces.OwnershipMarker{
		TaskID:        "task-live-sym",
		TaskDirName:   "live-task_sym",
		LayoutVersion: storageworkspaces.LayoutVersionSemantic,
	}); err != nil {
		t.Fatalf("write live ownership marker: %v", err)
	}
	liveWorktreePath := filepath.Join(liveTaskDir, "my-repo")
	runGit(t, repoPath, "worktree", "add", "-b", "feature/live", liveWorktreePath, "main")

	// Create a stale task directory whose "repo" entry is a symlink to the
	// live checkout. A clean redo that creates the stale worktree before the
	// live one and then replaces the final component with a symlink would be
	// more realistic, but a direct symlink is deterministic and exercises the
	// same code path: the record's TaskID !== the live marker's TaskID.
	staleTaskDir := filepath.Join(tasksBase, "stale-task_sym")
	if err := os.MkdirAll(staleTaskDir, 0755); err != nil {
		t.Fatalf("mkdir stale task dir: %v", err)
	}
	if err := storageworkspaces.WriteOwnershipMarker(staleTaskDir, storageworkspaces.OwnershipMarker{
		TaskID:        "task-stale-sym",
		TaskDirName:   "stale-task_sym",
		LayoutVersion: storageworkspaces.LayoutVersionSemantic,
	}); err != nil {
		t.Fatalf("write stale ownership marker: %v", err)
	}
	// The symlink — the stale record's persisted path points to a symlink
	// leading into the live task's checkout.
	staleWorktreePath := filepath.Join(staleTaskDir, "my-repo")
	if err := os.Symlink(liveWorktreePath, staleWorktreePath); err != nil {
		t.Fatalf("symlink stale -> live: %v", err)
	}

	store := newMockStore()
	store.worktrees["canonical-sym"] = &Worktree{
		ID:                "canonical-sym",
		TaskID:            "task-stale-sym",
		TaskEnvironmentID: "environment-1",
		RepositoryID:      "repository-1",
		Path:              staleWorktreePath,
		Branch:            "feature/live",
		Status:            StatusActive,
	}
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	_, err = mgr.Create(context.Background(), CreateRequest{
		TaskID:            "task-attach-sym",
		SessionID:         "session-attach-sym",
		TaskEnvironmentID: "environment-1",
		RepositoryID:      "repository-1",
		RepositoryPath:    repoPath,
		BaseBranch:        "main",
		WorktreeID:        "canonical-sym",
		ReuseRequired:     true,
	})
	if !errors.Is(err, ErrWorktreePathOwnedByAnotherTask) && !strings.Contains(err.Error(), "unsafe worktree path") {
		t.Fatalf("Create() error = %v, want ErrWorktreePathOwnedByAnotherTask or unsafe path error", err)
	}
}

// TestCreate_TryReuseExistingRejectsSymlinkedWorktreePath confirms that the
// normal reuse path (tryReuseExisting) rejects a persisted worktree whose
// path beneath tasksBase contains a symlink. Uses the session-based lookup
// branch.
func TestCreate_TryReuseExistingRejectsSymlinkedWorktreePath(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	cfg := newTestConfig(t)
	tasksBase := cfg.TasksBasePath

	liveTaskDir := filepath.Join(tasksBase, "live-task_sym2")
	if err := os.MkdirAll(liveTaskDir, 0755); err != nil {
		t.Fatalf("mkdir live task dir: %v", err)
	}
	if err := storageworkspaces.WriteOwnershipMarker(liveTaskDir, storageworkspaces.OwnershipMarker{
		TaskID:        "task-live-sym2",
		TaskDirName:   "live-task_sym2",
		LayoutVersion: storageworkspaces.LayoutVersionSemantic,
	}); err != nil {
		t.Fatalf("write live ownership marker: %v", err)
	}
	liveWorktreePath := filepath.Join(liveTaskDir, "my-repo")
	runGit(t, repoPath, "worktree", "add", "-b", "feature/live2", liveWorktreePath, "main")

	// Stale task: the final "repo" component is a symlink to the live checkout.
	staleTaskDir := filepath.Join(tasksBase, "stale-task_sym2")
	if err := os.MkdirAll(staleTaskDir, 0755); err != nil {
		t.Fatalf("mkdir stale task dir: %v", err)
	}
	if err := storageworkspaces.WriteOwnershipMarker(staleTaskDir, storageworkspaces.OwnershipMarker{
		TaskID:        "task-stale-sym2",
		TaskDirName:   "stale-task_sym2",
		LayoutVersion: storageworkspaces.LayoutVersionSemantic,
	}); err != nil {
		t.Fatalf("write stale ownership marker: %v", err)
	}
	staleWorktreePath := filepath.Join(staleTaskDir, "my-repo")
	if err := os.Symlink(liveWorktreePath, staleWorktreePath); err != nil {
		t.Fatalf("symlink stale2 -> live2: %v", err)
	}

	store := newMockStore()
	store.worktrees["wt-sym2"] = &Worktree{
		ID:           "wt-sym2",
		TaskID:       "task-stale-sym2",
		SessionID:    "session-reuse-sym2",
		RepositoryID: "repository-1",
		Path:         staleWorktreePath,
		Branch:       "feature/live2",
		Status:       StatusActive,
	}
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	_, err = mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-attach-sym2",
		SessionID:      "session-reuse-sym2",
		RepositoryID:   "repository-1",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		WorktreeID:     "wt-sym2",
	})
	if !errors.Is(err, ErrWorktreePathOwnedByAnotherTask) && !strings.Contains(err.Error(), "unsafe worktree path") {
		t.Fatalf("Create() error = %v, want ErrWorktreePathOwnedByAnotherTask or unsafe path error", err)
	}
}

func TestCreate_ReuseRequiredAllowsTransferredEnvironmentOwner(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	cfg := newTestConfig(t)
	ownerTaskDir := filepath.Join(cfg.TasksBasePath, "owner-task_transfer")
	if err := os.MkdirAll(ownerTaskDir, 0755); err != nil {
		t.Fatalf("mkdir owner task dir: %v", err)
	}
	if err := storageworkspaces.WriteOwnershipMarker(ownerTaskDir, storageworkspaces.OwnershipMarker{
		TaskID:        "task-owner-transfer",
		TaskDirName:   "owner-task_transfer",
		LayoutVersion: storageworkspaces.LayoutVersionSemantic,
	}); err != nil {
		t.Fatalf("write owner ownership marker: %v", err)
	}
	worktreePath := filepath.Join(ownerTaskDir, "repo")
	runGit(t, repoPath, "worktree", "add", "-b", "feature/transferred", worktreePath, "main")

	store := newMockStore()
	store.worktrees["transferred-worktree"] = &Worktree{
		ID:                "transferred-worktree",
		TaskID:            "task-borrower-transfer",
		TaskDirName:       "owner-task_transfer",
		TaskEnvironmentID: "environment-transfer",
		RepositoryID:      "repository-transfer",
		Path:              worktreePath,
		Branch:            "feature/transferred",
		Status:            StatusActive,
	}
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	got, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:            "task-borrower-transfer",
		SessionID:         "session-borrower-transfer",
		TaskEnvironmentID: "environment-transfer",
		RepositoryID:      "repository-transfer",
		RepositoryPath:    repoPath,
		BaseBranch:        "main",
		WorktreeID:        "transferred-worktree",
		ReuseRequired:     true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v, want transferred environment reuse", err)
	}
	if got.ID != "transferred-worktree" || got.Path != worktreePath {
		t.Fatalf("Create() = %#v, want transferred worktree", got)
	}
}
