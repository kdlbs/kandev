package worktree

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateInTaskDir_CheckoutBranchUsesRemoteStartPointAndUpstream(t *testing.T) {
	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	repoPath := initGitRepoWithRemote(t)

	wt1, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-1",
		SessionID:      "session-1",
		TaskTitle:      "PR Review 1",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		CheckoutBranch: "feature/pr-branch",
		TaskDirName:    "task-1",
		RepoName:       "repo-1",
	})
	if err != nil {
		t.Fatalf("Create() first task-dir worktree: %v", err)
	}
	if wt1.Branch != "feature/pr-branch" {
		t.Fatalf("first worktree branch = %q, want %q", wt1.Branch, "feature/pr-branch")
	}

	wt2, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-2",
		SessionID:      "session-2",
		TaskTitle:      "PR Review 2",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		CheckoutBranch: "feature/pr-branch",
		TaskDirName:    "task-2",
		RepoName:       "repo-1",
	})
	if err != nil {
		t.Fatalf("Create() second task-dir worktree: %v", err)
	}
	if !strings.HasPrefix(wt2.Branch, "feature/pr-branch-") {
		t.Fatalf("second worktree branch = %q, want feature/pr-branch-*", wt2.Branch)
	}
	if wt2.FetchWarning != "" {
		t.Fatalf("second worktree fetch warning = %q, want empty", wt2.FetchWarning)
	}

	sha1 := strings.TrimSpace(runGit(t, wt1.Path, "rev-parse", "HEAD"))
	sha2 := strings.TrimSpace(runGit(t, wt2.Path, "rev-parse", "HEAD"))
	if sha1 != sha2 {
		t.Fatalf("worktree SHAs differ: wt1=%q wt2=%q", sha1, sha2)
	}

	const wantUpstream = "origin/feature/pr-branch"
	upstream1 := strings.TrimSpace(runGit(t, wt1.Path, "rev-parse", "--abbrev-ref", "@{upstream}"))
	if upstream1 != wantUpstream {
		t.Fatalf("first worktree upstream = %q, want %q", upstream1, wantUpstream)
	}
	upstream2 := strings.TrimSpace(runGit(t, wt2.Path, "rev-parse", "--abbrev-ref", "@{upstream}"))
	if upstream2 != wantUpstream {
		t.Fatalf("second worktree upstream = %q, want %q", upstream2, wantUpstream)
	}
}

func TestCreateInTaskDir_RemoteBaseRefDoesNotSetUpstream(t *testing.T) {
	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	repoPath := initGitRepoWithRemote(t)

	// Create a task-dir worktree from a remote-tracking ref without CheckoutBranch.
	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-1",
		SessionID:      "session-1",
		TaskTitle:      "New Feature",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		BaseBranch:     "origin/feature/pr-branch",
		TaskDirName:    "task-1",
		RepoName:       "repo-1",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	// The new branch should NOT have upstream tracking set.
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "@{upstream}")
	cmd.Dir = wt.Path
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("expected no upstream for new task branch, but got %q", strings.TrimSpace(string(out)))
	}
}

// Org-prefixed RepoName ("owner/repo") used to nest the worktree one level
// below the task root, breaking agentctl's sibling-based multi-repo detection.
// The worktree directory must sit directly under the task root so two repos
// (clean + org-prefixed) end up as siblings.
func TestCreateInTaskDir_OrgPrefixedRepoNameLandsAsSibling(t *testing.T) {
	cfg := newTestConfig(t)
	store := newMockStore()
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	repoA := initGitRepoWithRemote(t)
	repoB := initGitRepoWithRemote(t)
	const taskDir = "test-task"

	wtA, err := mgr.Create(context.Background(), CreateRequest{
		TaskID: "t-1", SessionID: "s-1", TaskTitle: "T",
		RepositoryID:   "repo-a",
		RepositoryPath: repoA,
		BaseBranch:     "main",
		TaskDirName:    taskDir,
		RepoName:       "arthur",
	})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}

	wtB, err := mgr.Create(context.Background(), CreateRequest{
		TaskID: "t-1", SessionID: "s-1", TaskTitle: "T",
		RepositoryID:   "repo-b",
		RepositoryPath: repoB,
		BaseBranch:     "main",
		TaskDirName:    taskDir,
		RepoName:       "acme/widget-config",
	})
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}

	parentA := filepath.Dir(wtA.Path)
	parentB := filepath.Dir(wtB.Path)
	if parentA != parentB {
		t.Fatalf("worktrees not siblings: A parent %q vs B parent %q", parentA, parentB)
	}
	if base := filepath.Base(wtB.Path); base != "acme-widget-config" {
		t.Errorf("repo B dir = %q, want %q", base, "acme-widget-config")
	}
}
