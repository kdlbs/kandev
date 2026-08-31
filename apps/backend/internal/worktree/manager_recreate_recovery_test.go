package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// archiveDeletesLocalBranch simulates what task archive does to a worktree's
// branch: the local ref is deleted (`git branch -D`), while origin and the
// remote-tracking ref are left alone.
func archiveDeletesLocalBranch(t *testing.T, repoPath, branch string) {
	t.Helper()
	runGit(t, repoPath, "branch", "-D", branch)
}

func newRecreateTestManager(t *testing.T) *Manager {
	t.Helper()
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	return mgr
}

// TestRecreate_FetchesBranchFromOriginWhenLocalDeleted is the
// unarchive-recovery path: archive deleted the local branch and the worktree
// directory, but the branch was pushed. recreate must fetch it back from
// origin and rebuild the worktree at the recorded path.
func TestRecreate_FetchesBranchFromOriginWhenLocalDeleted(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	branchSHA := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "feature/pr-branch"))
	archiveDeletesLocalBranch(t, repoPath, "feature/pr-branch")

	mgr := newRecreateTestManager(t)
	existing := &Worktree{
		ID:             "wt-1",
		SessionID:      "session-1",
		TaskID:         "task-1",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		Path:           filepath.Join(t.TempDir(), "task-1", "repo-1"),
		Branch:         "feature/pr-branch",
		Status:         StatusDeleted,
	}

	wt, err := mgr.recreate(context.Background(), existing, CreateRequest{
		SessionID:      "session-1",
		TaskID:         "task-1",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
	})
	if err != nil {
		t.Fatalf("recreate() should fetch the branch from origin, got: %v", err)
	}
	if wt.Status != StatusActive {
		t.Errorf("status = %q, want %q", wt.Status, StatusActive)
	}
	gotSHA := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))
	if gotSHA != branchSHA {
		t.Errorf("worktree HEAD = %q, want %q (pushed branch tip)", gotSHA, branchSHA)
	}
}

func TestRecreate_RefreshFailureUsesLocalBase(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	worktreePath := filepath.Join(t.TempDir(), "task-required", "repo-1")
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatalf("create worktree placeholder: %v", err)
	}
	branchSHA := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "feature/pr-branch"))
	runGit(t, repoPath, "remote", "set-url", "origin", "https://user:super-secret@127.0.0.1:1/missing.git")

	mgr := newRecreateTestManager(t)
	existing := &Worktree{
		ID:             "wt-required",
		SessionID:      "session-required",
		TaskID:         "task-required",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		Path:           worktreePath,
		Branch:         "feature/pr-branch",
		Status:         StatusDeleted,
	}

	_, err := mgr.recreate(context.Background(), existing, CreateRequest{
		SessionID:          "session-required",
		TaskID:             "task-required",
		RepositoryID:       "repo-1",
		RepositoryPath:     repoPath,
		BaseBranch:         "main",
		PullBeforeWorktree: true,
	})
	if err != nil {
		t.Fatalf("recreate() error = %v", err)
	}
	if _, statErr := os.Stat(worktreePath); statErr != nil {
		t.Fatalf("recreate() did not restore the worktree path: %v", statErr)
	}
	gotSHA := strings.TrimSpace(runGit(t, worktreePath, "rev-parse", "HEAD"))
	if gotSHA != branchSHA {
		t.Fatalf("recreated worktree HEAD = %q, want local branch %q", gotSHA, branchSHA)
	}
}

// TestRecreate_RejectsSymlinkedAncestorBeforeRemoval ensures recreate does not
// delete through an ancestor that changed into a symlink after earlier reuse
// validation.
func TestRecreate_RejectsSymlinkedAncestorBeforeRemoval(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	cfg := newTestConfig(t)
	mgr, err := NewManager(cfg, newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	outside := t.TempDir()
	liveWorktree := filepath.Join(outside, "repo-one")
	if err := os.MkdirAll(liveWorktree, 0o755); err != nil {
		t.Fatalf("mkdir live worktree: %v", err)
	}
	canary := filepath.Join(liveWorktree, "must-survive")
	if err := os.WriteFile(canary, []byte("live task data"), 0o600); err != nil {
		t.Fatalf("write canary: %v", err)
	}
	taskRoot := filepath.Join(cfg.TasksBasePath, "task-one")
	if err := os.Symlink(outside, taskRoot); err != nil {
		t.Fatalf("replace task root with symlink: %v", err)
	}

	_, err = mgr.recreate(context.Background(), &Worktree{
		ID:           "wt-symlink-race",
		TaskID:       "task-one",
		TaskDirName:  "task-one",
		RepositoryID: "repo-one",
		Path:         filepath.Join(taskRoot, "repo-one"),
		Branch:       "feature/pr-branch",
		Status:       StatusActive,
	}, CreateRequest{
		TaskID:         "task-one",
		RepositoryID:   "repo-one",
		RepositoryPath: repoPath,
	})
	if err == nil {
		t.Fatal("recreate() error = nil, want symlinked ancestor rejection")
	}
	if _, statErr := os.Stat(canary); statErr != nil {
		t.Fatalf("recreate() touched live worktree through symlink: %v", statErr)
	}
}

// TestRecreate_BranchGoneEverywhereReturnsUnrecoverable pins the degraded
// path: local branch deleted AND the branch never made it to origin (or was
// deleted there too). recreate must return ErrBranchUnrecoverable so callers
// can fall back to a fresh worktree instead of failing opaquely.
func TestRecreate_BranchGoneEverywhereReturnsUnrecoverable(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	// Delete the branch everywhere: on origin, locally, and prune the
	// remote-tracking ref so no trace remains.
	runGit(t, repoPath, "push", "origin", "--delete", "feature/pr-branch")
	archiveDeletesLocalBranch(t, repoPath, "feature/pr-branch")
	runGit(t, repoPath, "fetch", "--prune", "origin")

	mgr := newRecreateTestManager(t)
	existing := &Worktree{
		ID:             "wt-2",
		SessionID:      "session-2",
		TaskID:         "task-2",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		Path:           filepath.Join(t.TempDir(), "task-2", "repo-1"),
		Branch:         "feature/pr-branch",
		Status:         StatusDeleted,
	}

	_, err := mgr.recreate(context.Background(), existing, CreateRequest{
		SessionID:      "session-2",
		TaskID:         "task-2",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
	})
	if !errors.Is(err, ErrBranchUnrecoverable) {
		t.Fatalf("recreate() err = %v, want ErrBranchUnrecoverable", err)
	}
	var branchErr interface{ BranchName() string }
	if !errors.As(err, &branchErr) {
		t.Fatalf("recreate() err = %v, want a typed branch recovery error", err)
	}
	if got := branchErr.BranchName(); got != existing.Branch {
		t.Fatalf("branch recovery error names %q, want %q", got, existing.Branch)
	}
}

func TestRecreate_DoesNotReplaceAfterTransientFetchFailure(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	runGit(t, repoPath, "remote", "set-url", "origin", "https://127.0.0.1:1/unavailable.git")
	archiveDeletesLocalBranch(t, repoPath, "feature/pr-branch")

	cfg := newTestConfig(t)
	store := newMockStore()
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	existing := &Worktree{
		ID:                "wt-transient-fetch",
		SessionID:         "session-transient-fetch",
		TaskID:            "task-transient-fetch",
		TaskDirName:       "task-transient-fetch",
		TaskEnvironmentID: "env-transient-fetch",
		RepositoryID:      "repo-1",
		RepositoryPath:    repoPath,
		Path:              filepath.Join(cfg.TasksBasePath, "task-transient-fetch", "repo-1"),
		Branch:            "feature/pr-branch",
		Status:            StatusDeleted,
	}
	store.worktrees[existing.ID] = existing

	_, err = mgr.recreate(context.Background(), existing, CreateRequest{
		SessionID:              existing.SessionID,
		TaskID:                 existing.TaskID,
		TaskEnvironmentID:      existing.TaskEnvironmentID,
		RepositoryID:           existing.RepositoryID,
		RepositoryPath:         repoPath,
		TaskDirName:            existing.TaskDirName,
		RepoName:               "repo-1",
		BaseBranch:             "main",
		AllowBranchReplacement: true,
	})
	if err == nil {
		t.Fatal("recreate() succeeded after a transient fetch failure")
	}
	if errors.Is(err, ErrBranchUnrecoverable) {
		t.Fatalf("recreate() misclassified transient fetch failure as branch loss: %v", err)
	}
	if errors.Is(err, ErrInvalidBaseBranch) {
		t.Fatalf("recreate() misclassified transient fetch failure as invalid branch: %v", err)
	}
	if errors.Is(err, ErrRemoteRefMissing) {
		t.Fatalf("recreate() reported confirmed remote ref loss for transient fetch failure: %v", err)
	}
}

func TestRecreate_AllowsExplicitReplacementWithFreshBranchAndPath(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	runGit(t, repoPath, "push", "origin", "--delete", "feature/pr-branch")
	archiveDeletesLocalBranch(t, repoPath, "feature/pr-branch")
	runGit(t, repoPath, "fetch", "--prune", "origin")

	cfg := newTestConfig(t)
	store := newMockStore()
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	existing := &Worktree{
		ID:                "wt-replace",
		SessionID:         "session-replace",
		TaskID:            "task-replace",
		TaskDirName:       "task-replace",
		TaskEnvironmentID: "env-replace",
		RepositoryID:      "repo-1",
		RepositoryPath:    repoPath,
		Path:              filepath.Join(cfg.TasksBasePath, "task-replace", "repo-1-primary"),
		Branch:            "feature/pr-branch",
		BranchSlug:        "primary",
		BaseBranch:        "main",
		Status:            StatusDeleted,
	}
	store.worktrees[existing.ID] = existing

	wt, err := mgr.recreate(context.Background(), existing, CreateRequest{
		SessionID:              existing.SessionID,
		TaskID:                 existing.TaskID,
		TaskEnvironmentID:      existing.TaskEnvironmentID,
		RepositoryID:           existing.RepositoryID,
		RepositoryPath:         repoPath,
		BaseBranch:             "main",
		TaskDirName:            existing.TaskDirName,
		RepoName:               "repo-1",
		BranchIdentitySlug:     existing.BranchSlug,
		PullBeforeWorktree:     true,
		AllowBranchReplacement: true,
	})
	if err != nil {
		t.Fatalf("recreate() explicit replacement: %v", err)
	}
	if wt.ID != existing.ID {
		t.Fatalf("replacement worktree ID = %q, want stable ID %q", wt.ID, existing.ID)
	}
	if wt.Branch == existing.Branch {
		t.Fatalf("replacement branch = %q, want a fresh branch", wt.Branch)
	}
	if wt.Path == existing.Path {
		t.Fatalf("replacement path = %q, want a fresh path", wt.Path)
	}
	if wt.BranchSlug != existing.BranchSlug {
		t.Fatalf("replacement branch slug = %q, want stable identity %q", wt.BranchSlug, existing.BranchSlug)
	}
	if wt.Status != StatusActive || wt.DeletedAt != nil {
		t.Fatalf("replacement status = %q, deleted_at = %v, want active and nil", wt.Status, wt.DeletedAt)
	}
	if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "--abbrev-ref", "HEAD")); got != wt.Branch {
		t.Fatalf("replacement HEAD branch = %q, want %q", got, wt.Branch)
	}
	mainSHA := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "main"))
	if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD")); got != mainSHA {
		t.Fatalf("replacement HEAD = %q, want main SHA %q", got, mainSHA)
	}
	stored := store.worktrees[existing.ID]
	if stored == nil || stored.Branch != wt.Branch || stored.Path != wt.Path {
		t.Fatalf("stored replacement = %#v, want branch %q and path %q", stored, wt.Branch, wt.Path)
	}
}

type replacementUpdateErrorStore struct {
	*mockStore
	err       error
	attempted *Worktree
}

func (s *replacementUpdateErrorStore) UpdateWorktree(_ context.Context, wt *Worktree) error {
	copy := *wt
	s.attempted = &copy
	return s.err
}

func TestRecreate_CleansReplacementWhenPersistenceFails(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	runGit(t, repoPath, "push", "origin", "--delete", "feature/pr-branch")
	archiveDeletesLocalBranch(t, repoPath, "feature/pr-branch")
	runGit(t, repoPath, "fetch", "--prune", "origin")

	cfg := newTestConfig(t)
	storeErr := errors.New("replacement persistence failed")
	store := &replacementUpdateErrorStore{mockStore: newMockStore(), err: storeErr}
	existing := &Worktree{
		ID:                "wt-replace-persist-failure",
		SessionID:         "session-replace-persist-failure",
		TaskID:            "task-replace-persist-failure",
		TaskDirName:       "task-replace-persist-failure",
		TaskEnvironmentID: "env-replace-persist-failure",
		RepositoryID:      "repo-1",
		RepositoryPath:    repoPath,
		Path:              filepath.Join(cfg.TasksBasePath, "task-replace-persist-failure", "repo-1-old"),
		Branch:            "feature/pr-branch",
		BaseBranch:        "main",
		Status:            StatusDeleted,
	}
	store.worktrees[existing.ID] = existing
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	_, err = mgr.recreate(context.Background(), existing, CreateRequest{
		SessionID:              existing.SessionID,
		TaskID:                 existing.TaskID,
		TaskEnvironmentID:      existing.TaskEnvironmentID,
		RepositoryID:           existing.RepositoryID,
		RepositoryPath:         repoPath,
		BaseBranch:             existing.BaseBranch,
		TaskDirName:            existing.TaskDirName,
		RepoName:               "repo-1",
		BranchIdentitySlug:     "primary",
		PullBeforeWorktree:     true,
		AllowBranchReplacement: true,
	})
	if !errors.Is(err, storeErr) {
		t.Fatalf("recreate() error = %v, want %v", err, storeErr)
	}
	if store.attempted == nil {
		t.Fatal("replacement persistence was not attempted")
	}
	if _, statErr := os.Stat(store.attempted.Path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("replacement worktree path still exists after persistence failure: %q (%v)", store.attempted.Path, statErr)
	}
	branchExists, branchErr := mgr.branchExists(context.Background(), repoPath, "refs/heads/"+store.attempted.Branch)
	if branchErr != nil {
		t.Fatalf("verify replacement branch cleanup: %v", branchErr)
	}
	if branchExists {
		t.Fatalf("replacement branch %q still exists after persistence failure", store.attempted.Branch)
	}
	stored := store.worktrees[existing.ID]
	if stored == nil || stored.Path != existing.Path || stored.Branch != existing.Branch || stored.Status != StatusDeleted {
		t.Fatalf("stored worktree = %#v, want the original deleted record %#v", stored, existing)
	}
}

// TestBranchRecoveryStatus covers the three probe outcomes used by the
// unarchive HTTP response: local, remote (only the remote-tracking ref
// remains after archive deleted the local branch), and missing.
func TestBranchRecoveryStatus(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	mgr := newRecreateTestManager(t)
	ctx := context.Background()

	if got := mgr.BranchRecoveryStatus(ctx, repoPath, "feature/pr-branch"); got != BranchStatusLocal {
		t.Errorf("status with local branch = %q, want %q", got, BranchStatusLocal)
	}

	archiveDeletesLocalBranch(t, repoPath, "feature/pr-branch")
	if got := mgr.BranchRecoveryStatus(ctx, repoPath, "feature/pr-branch"); got != BranchStatusRemote {
		t.Errorf("status after local delete = %q, want %q", got, BranchStatusRemote)
	}

	if got := mgr.BranchRecoveryStatus(ctx, repoPath, "feature/never-existed"); got != BranchStatusMissing {
		t.Errorf("status for unknown branch = %q, want %q", got, BranchStatusMissing)
	}
	if got := mgr.BranchRecoveryStatus(ctx, "", "feature/pr-branch"); got != BranchStatusMissing {
		t.Errorf("status with empty repo path = %q, want %q", got, BranchStatusMissing)
	}
}

// TestRecreate_ForkPRFetchesPullHeadRef covers fork-PR tasks: the head
// branch never exists on origin by name, only under refs/pull/<N>/head.
// recreate must forward req.PRNumber so fetchBranchToLocal uses the pull
// refspec instead of failing with ErrBranchUnrecoverable.
func TestRecreate_ForkPRFetchesPullHeadRef(t *testing.T) {
	repoPath, prHeadSHA := initGitRepoWithPullRef(t, 974, "feature/fork-pr")

	mgr := newRecreateTestManager(t)
	existing := &Worktree{
		ID:             "wt-3",
		SessionID:      "session-3",
		TaskID:         "task-3",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		Path:           filepath.Join(t.TempDir(), "task-3", "repo-1"),
		Branch:         "feature/fork-pr",
		Status:         StatusDeleted,
	}

	wt, err := mgr.recreate(context.Background(), existing, CreateRequest{
		SessionID:      "session-3",
		TaskID:         "task-3",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		PRNumber:       974,
	})
	if err != nil {
		t.Fatalf("recreate() should fetch the fork PR head via pull/<N>/head, got: %v", err)
	}
	gotSHA := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))
	if gotSHA != prHeadSHA {
		t.Errorf("worktree HEAD = %q, want %q (PR head)", gotSHA, prHeadSHA)
	}
}

func TestRecreate_ManagedRefreshUsesRefreshedPRHeadWithoutNetwork(t *testing.T) {
	repoPath, wantSHA := initGitRepoWithPullRef(t, 975, "feature/managed-fork-pr")
	runGit(t, repoPath, "fetch", "origin", "pull/975/head:refs/remotes/origin/pr/975")
	runGit(t, repoPath, "remote", "set-url", "origin", "https://127.0.0.1:1/never.git")
	worktreePath := filepath.Join(t.TempDir(), "task-managed-pr", "repo-1")
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatalf("create worktree placeholder: %v", err)
	}

	mgr := newRecreateTestManager(t)
	refreshCalls := 0
	wt, err := mgr.recreate(context.Background(), &Worktree{
		ID: "wt-managed-pr", SessionID: "session-managed-pr", TaskID: "task-managed-pr",
		RepositoryID: "repo-1", RepositoryPath: repoPath, Path: worktreePath,
		Branch: "feature/managed-fork-pr", Status: StatusDeleted,
	}, CreateRequest{
		SessionID: "session-managed-pr", TaskID: "task-managed-pr", RepositoryID: "repo-1",
		RepositoryPath: repoPath, CheckoutBranch: "feature/managed-fork-pr", PRNumber: 975,
		PullBeforeWorktree: true,
		RefreshRepository: func(context.Context) error {
			refreshCalls++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("recreate(): %v", err)
	}
	if refreshCalls != 1 {
		t.Fatalf("refresh calls = %d, want 1", refreshCalls)
	}
	if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD")); got != wantSHA {
		t.Fatalf("worktree HEAD = %q, want refreshed PR head %q", got, wantSHA)
	}
}

func TestRecreate_ManagedRefreshUsesRemoteWhenLocalCheckoutBranchIsBehind(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	wantSHA := advanceRemoteBranch(t, repoPath, "feature/pr-branch")
	runGit(t, repoPath, "remote", "set-url", "origin", "https://127.0.0.1:1/never.git")
	worktreePath := filepath.Join(t.TempDir(), "task-managed-branch", "repo-1")
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatalf("create worktree placeholder: %v", err)
	}

	mgr := newRecreateTestManager(t)
	wt, err := mgr.recreate(context.Background(), &Worktree{
		ID: "wt-managed-branch", SessionID: "session-managed-branch", TaskID: "task-managed-branch",
		RepositoryID: "repo-1", RepositoryPath: repoPath, Path: worktreePath,
		Branch: "feature/pr-branch", Status: StatusDeleted,
	}, CreateRequest{
		SessionID: "session-managed-branch", TaskID: "task-managed-branch", RepositoryID: "repo-1",
		RepositoryPath: repoPath, CheckoutBranch: "feature/pr-branch", PullBeforeWorktree: true,
		RefreshRepository: func(context.Context) error { return nil },
	})
	if err != nil {
		t.Fatalf("recreate(): %v", err)
	}
	if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD")); got != wantSHA {
		t.Fatalf("worktree HEAD = %q, want refreshed branch head %q", got, wantSHA)
	}
}

func TestRecreate_ManagedRefreshUsesRemotePRHeadWhenLocalCheckoutBranchIsBehind(t *testing.T) {
	repoPath, wantSHA := initManagedPRCheckoutBranch(t, 977, "feature/managed-fork-pr-behind")
	runGit(t, repoPath, "remote", "set-url", "origin", "https://127.0.0.1:1/never.git")
	worktreePath := filepath.Join(t.TempDir(), "task-managed-pr-behind", "repo-1")
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatalf("create worktree placeholder: %v", err)
	}

	mgr := newRecreateTestManager(t)
	wt, err := mgr.recreate(context.Background(), &Worktree{
		ID: "wt-managed-pr-behind", SessionID: "session-managed-pr-behind", TaskID: "task-managed-pr-behind",
		RepositoryID: "repo-1", RepositoryPath: repoPath, Path: worktreePath,
		Branch: "feature/managed-fork-pr-behind", Status: StatusDeleted,
	}, CreateRequest{
		SessionID: "session-managed-pr-behind", TaskID: "task-managed-pr-behind", RepositoryID: "repo-1",
		RepositoryPath: repoPath, CheckoutBranch: "feature/managed-fork-pr-behind", PRNumber: 977,
		PullBeforeWorktree: true,
		RefreshRepository:  func(context.Context) error { return nil },
	})
	if err != nil {
		t.Fatalf("recreate(): %v", err)
	}
	if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD")); got != wantSHA {
		t.Fatalf("worktree HEAD = %q, want refreshed PR head %q", got, wantSHA)
	}
}

// TestCreate_RestoresReleasedWorktreeAfterArchive is the whole unarchive
// round trip at the worktree layer. Archiving a task removes the worktree
// directory and releases its reference (status=deleted + deleted_at) while
// deliberately keeping the git branch, so the next launch must rebuild the
// directory from the released record — including reactivating that record.
// Leaving deleted_at set would hide the restored worktree from every lookup
// that filters on `deleted_at IS NULL`, so the session would silently get a
// brand-new worktree instead of its own work back.
func TestCreate_RestoresReleasedWorktreeAfterArchive(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-archived", "session-archived", models.TaskSessionStateCompleted)

	repoPath := initGitRepoWithRemote(t)
	req := CreateRequest{
		TaskID:         "task-archived",
		SessionID:      "session-archived",
		TaskTitle:      "Archived work",
		RepositoryID:   "repository",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		TaskDirName:    "task-archived",
		RepoName:       "repository",
	}
	wt, err := mgr.Create(ctx, req)
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	// Work the user never pushed. It only survives because archive keeps the
	// branch (DestroyWorktree passes removeBranch=false).
	runGit(t, wt.Path, "commit", "--allow-empty", "-m", "unpushed work")
	workSHA := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))

	// Archive: remove the directory, release the reference, keep the branch.
	if err := mgr.RemoveByID(ctx, wt.ID, false); err != nil {
		t.Fatalf("archive worktree: %v", err)
	}
	if _, statErr := os.Stat(wt.Path); !os.IsNotExist(statErr) {
		t.Fatalf("archive should remove the worktree directory, stat error = %v", statErr)
	}

	// Unarchive + resume: the launch carries the stored worktree ID.
	resumeReq := req
	resumeReq.WorktreeID = wt.ID
	restored, err := mgr.Create(ctx, resumeReq)
	if err != nil {
		t.Fatalf("resume after unarchive must recreate the worktree: %v", err)
	}
	if restored.Path != wt.Path {
		t.Fatalf("restored path = %q, want the original %q", restored.Path, wt.Path)
	}
	if restored.Branch != wt.Branch {
		t.Fatalf("restored branch = %q, want the original %q", restored.Branch, wt.Branch)
	}
	if got := strings.TrimSpace(runGit(t, restored.Path, "rev-parse", "HEAD")); got != workSHA {
		t.Fatalf("restored HEAD = %q, want the pre-archive work %q", got, workSHA)
	}

	// The record must be visible again to the session-scoped lookup the next
	// launch uses; a lingering deleted_at would strand it.
	found, err := store.GetWorktreeBySessionAndRepository(ctx, "session-archived", "repository", restored.BranchSlug)
	if err != nil {
		t.Fatalf("look up restored worktree: %v", err)
	}
	if found == nil {
		t.Fatal("restored worktree is still hidden from the session lookup (deleted_at was not cleared)")
	}
	if found.ID != wt.ID {
		t.Fatalf("session lookup returned worktree %q, want the restored %q", found.ID, wt.ID)
	}
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusActive)
}
