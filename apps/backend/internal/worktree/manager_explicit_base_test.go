package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCreateWorktree_ExplicitMissingBaseFailsClosed reproduces issue #5142
// with real Git: the cache is cloned before the selected branch is published,
// so the selected commit exists only on the remote. An explicit selection must
// fail rather than materialize the repository default.
func TestCreateWorktree_ExplicitMissingBaseFailsClosed(t *testing.T) {
	remotePath, sourcePath := newExplicitBaseRemoteFixture(t)
	cachePath := filepath.Join(t.TempDir(), "cache")
	runGit(t, filepath.Dir(cachePath), "clone", remotePath, cachePath)

	mainSentinel := strings.TrimSpace(runGit(t, cachePath, "rev-parse", "HEAD"))
	runGit(t, sourcePath, "checkout", "-b", "feature/qualified-base")
	if err := os.WriteFile(filepath.Join(sourcePath, "selected.txt"), []byte("selected\n"), 0o644); err != nil {
		t.Fatalf("write selected sentinel: %v", err)
	}
	runGit(t, sourcePath, "add", "selected.txt")
	runGit(t, sourcePath, "commit", "-m", "selected base")
	selectedHead := strings.TrimSpace(runGit(t, sourcePath, "rev-parse", "HEAD"))
	runGit(t, sourcePath, "push", "origin", "feature/qualified-base")

	store := newMockStore()
	mgr, err := NewManager(newTestConfig(t), store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	var createdCallbacks int
	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:             "task-explicit-base",
		SessionID:          "session-explicit-base",
		TaskTitle:          "Explicit base",
		RepositoryID:       "repo-explicit-base",
		RepositoryPath:     cachePath,
		BaseBranch:         "feature/qualified-base",
		FallbackBaseBranch: "main",
		TaskDirName:        "task-explicit-base",
		RepoName:           "repo",
		OnWorktreeCreated: func(*Worktree) {
			createdCallbacks++
		},
	})
	if err == nil {
		t.Fatal("Create() succeeded for an explicit base missing from the cache")
	}
	if !errors.Is(err, ErrInvalidBaseBranch) {
		t.Fatalf("Create() error = %v, want ErrInvalidBaseBranch", err)
	}
	if !strings.Contains(err.Error(), "feature/qualified-base") {
		t.Fatalf("Create() error = %q, want the requested ref", err)
	}
	if wt != nil {
		t.Fatalf("Create() worktree = %#v, want nil", wt)
	}
	if createdCallbacks != 0 {
		t.Fatalf("OnWorktreeCreated calls = %d, want zero", createdCallbacks)
	}
	if len(store.worktrees) != 0 {
		t.Fatalf("persisted worktrees = %d, want zero", len(store.worktrees))
	}
	if got := strings.TrimSpace(runGit(t, cachePath, "rev-parse", "main")); got != mainSentinel {
		t.Fatalf("cache main moved to %q, want %q", got, mainSentinel)
	}
	if list := runGit(t, cachePath, "worktree", "list", "--porcelain"); strings.Count(list, "worktree ") != 1 {
		t.Fatalf("git worktrees = %q, want only the cache checkout", list)
	}
	if selectedHead == mainSentinel {
		t.Fatalf("fixture requires distinct selected and main commits: %s", selectedHead)
	}
}

// TestResolveBaseRefWithFallback_StrictPolicyIgnoresLiveDefault pins the
// live remote-default substitution path: strict policy must stop before
// consulting a discoverable default.
func TestResolveBaseRefWithFallback_StrictPolicyIgnoresLiveDefault(t *testing.T) {
	mgr := newLiveDefaultTestManager(t)
	repoPath := initGitRepoForWorktreeTest(t)
	originRefs := filepath.Join(repoPath, ".git", "refs", "remotes", "origin")
	if err := os.MkdirAll(originRefs, 0o755); err != nil {
		t.Fatalf("mkdir origin refs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(originRefs, "HEAD"), []byte("ref: refs/remotes/origin/main\n"), 0o644); err != nil {
		t.Fatalf("write origin HEAD: %v", err)
	}

	req := CreateRequest{RepositoryPath: repoPath, BaseBranch: "stale-base"}
	_, _, _, err := mgr.resolveBaseRefWithFallback(context.Background(), &req)
	if !errors.Is(err, ErrInvalidBaseBranch) {
		t.Fatalf("resolveBaseRefWithFallback() error = %v, want ErrInvalidBaseBranch", err)
	}
	if req.BaseBranch != "stale-base" {
		t.Fatalf("request base mutated to %q under strict policy", req.BaseBranch)
	}
}

func newExplicitBaseRemoteFixture(t *testing.T) (remotePath, sourcePath string) {
	t.Helper()
	root := t.TempDir()
	remotePath = filepath.Join(root, "origin.git")
	runGit(t, root, "init", "--bare", "-b", "main", remotePath)
	sourcePath = filepath.Join(root, "source")
	runGit(t, root, "clone", remotePath, sourcePath)
	runGit(t, sourcePath, "config", "user.email", "test@example.com")
	runGit(t, sourcePath, "config", "user.name", "Test User")
	runGit(t, sourcePath, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(sourcePath, "main.txt"), []byte("main\n"), 0o644); err != nil {
		t.Fatalf("write main sentinel: %v", err)
	}
	runGit(t, sourcePath, "add", "main.txt")
	runGit(t, sourcePath, "commit", "-m", "main")
	runGit(t, sourcePath, "push", "-u", "origin", "main")
	return remotePath, sourcePath
}

// TestCreateWorktree_StrictPolicySkipsEarlyFallbackHelpers pins the two early
// substitution sites: provider-refreshed base selection and the numbered-PR
// pull helper. A strict selection must reach neither fallback.
func TestCreateWorktree_StrictPolicySkipsEarlyFallbackHelpers(t *testing.T) {
	cases := []struct {
		name       string
		request    func(repoPath string) CreateRequest
		wantDetail string
	}{
		{
			name: "provider refreshed route",
			request: func(repoPath string) CreateRequest {
				return CreateRequest{
					TaskID: "task-strict-refresh", SessionID: "session-strict-refresh",
					RepositoryID: "repo-strict-refresh", RepositoryPath: repoPath,
					BaseBranch: "feature/deleted-parent", FallbackBaseBranch: "main",
					CheckoutBranch: "feature/pr-branch", PRNumber: 42,
					RemoteSyncHandled: true, TaskDirName: "task-strict-refresh", RepoName: "repo",
				}
			},
			wantDetail: "feature/deleted-parent",
		},
		{
			name: "numbered PR pull route",
			request: func(repoPath string) CreateRequest {
				return CreateRequest{
					TaskID: "task-strict-pull", SessionID: "session-strict-pull",
					RepositoryID: "repo-strict-pull", RepositoryPath: repoPath,
					BaseBranch: "feature/deleted-parent", FallbackBaseBranch: "main",
					CheckoutBranch: "feature/pr-branch", PRNumber: 42,
					PullBeforeWorktree: true, TaskDirName: "task-strict-pull", RepoName: "repo",
				}
			},
			wantDetail: "missing_remote_ref",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repoPath, _ := initManagedPRCheckoutBranch(t, 42, "feature/pr-branch")
			store := newMockStore()
			mgr, err := NewManager(newTestConfig(t), store, newTestLogger())
			if err != nil {
				t.Fatalf("NewManager failed: %v", err)
			}
			wt, err := mgr.Create(context.Background(), tc.request(repoPath))
			if err == nil {
				t.Fatalf("Create() succeeded: %#v", wt)
			}
			if !strings.Contains(err.Error(), tc.wantDetail) {
				t.Fatalf("Create() error = %q, want %s", err, tc.wantDetail)
			}
			if errors.Is(err, ErrInvalidBaseBranch) {
				t.Fatalf("Create() error = %v, want required-refresh failure", err)
			}
			if wt != nil || len(store.worktrees) != 0 {
				t.Fatalf("strict fallback created a worktree: %#v", wt)
			}
		})
	}
}

// TestReplaceUnrecoverableWorktree_UnavailableBasePreservesOriginal pins the
// replacement gate: an unavailable explicit base stops before the replacement
// branch or worktree is created, and the original record and checkout remain.
func TestReplaceUnrecoverableWorktree_UnavailableBasePreservesOriginal(t *testing.T) {
	repoPath := initGitRepoWithRemote(t)
	mgr := newManagerForCopyTest(t, nil)
	existingPath := filepath.Join(t.TempDir(), "original-checkout")
	if err := os.MkdirAll(existingPath, 0o755); err != nil {
		t.Fatalf("create original checkout: %v", err)
	}
	if err := os.WriteFile(filepath.Join(existingPath, "sentinel.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	existing := &Worktree{
		ID: "wt-original", SessionID: "session-original", TaskID: "task-original",
		RepositoryID: "repo-original", RepositoryPath: repoPath, Path: existingPath,
		Branch: "feature/pr-branch", BaseBranch: "main", Status: StatusActive,
	}
	mgr.store.(*mockStore).worktrees[existing.ID] = existing

	_, err := mgr.replaceUnrecoverableWorktree(context.Background(), existing, CreateRequest{
		SessionID: existing.SessionID, TaskID: existing.TaskID,
		RepositoryID: existing.RepositoryID, RepositoryPath: repoPath,
		BaseBranch: "missing-base", FallbackBaseBranch: "main",
		TaskDirName: "task-original", RepoName: "repo-original",
		AllowBranchReplacement: true,
	}, &BranchUnrecoverableError{Branch: "feature/pr-branch"})
	if !errors.Is(err, ErrInvalidBaseBranch) {
		t.Fatalf("replace error = %v, want ErrInvalidBaseBranch", err)
	}
	sentinel, readErr := os.ReadFile(filepath.Join(existingPath, "sentinel.txt"))
	if readErr != nil || string(sentinel) != "keep\n" {
		t.Fatalf("original checkout changed: %q err=%v", sentinel, readErr)
	}
	stored := mgr.store.(*mockStore).worktrees[existing.ID]
	if stored == nil || stored.Branch != "feature/pr-branch" || stored.Path != existingPath {
		t.Fatalf("original record changed: %#v", stored)
	}
	if len(mgr.store.(*mockStore).worktrees) != 1 {
		t.Fatalf("replacement record was persisted: %#v", mgr.store.(*mockStore).worktrees)
	}
}
