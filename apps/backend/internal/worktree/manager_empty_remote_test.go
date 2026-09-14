package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/gitbootstrap"
	"github.com/kandev/kandev/internal/repoclone"
)

func TestCreateSeedsEmptyRemoteBaselineBeforeAddingWorktree(t *testing.T) {
	cfg := newTestConfig(t)
	mgr, err := NewManager(cfg, newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	repoPath := t.TempDir()
	runGit(t, repoPath, "init", "-b", "main")

	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:            "task-empty",
		SessionID:         "session-empty",
		RepositoryID:      "repo-empty",
		RepositoryPath:    repoPath,
		BaseBranch:        "main",
		TaskDirName:       "task-empty",
		RepoName:          "repo-empty",
		RemoteSyncHandled: true,
		RemoteRefState:    repoclone.RemoteRefStateEmpty,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if wt.BaseBranch != "main" {
		t.Fatalf("worktree BaseBranch = %q, want main", wt.BaseBranch)
	}
	if got := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "refs/heads/main")); got == "" {
		t.Fatal("empty-remote baseline did not create refs/heads/main")
	}
	baseline, present, err := gitbootstrap.Validate(context.Background(), repoPath, "main")
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !present || baseline.Commit == "" {
		t.Fatalf("Validate() = (%+v, %v), want a present baseline", baseline, present)
	}
	if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD")); got != baseline.Commit {
		t.Fatalf("worktree HEAD = %q, want baseline %q", got, baseline.Commit)
	}
}

func TestCreateReevaluatesAnEmptyRemoteRaceAgainstExistingLocalBase(t *testing.T) {
	cfg := newTestConfig(t)
	mgr, err := NewManager(cfg, newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	repoPath := t.TempDir()
	runGit(t, repoPath, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoPath, "add", "README.md")
	runGit(t, repoPath, "-c", "user.name=Test User", "-c", "user.email=test@example.com", "commit", "-m", "existing")

	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-empty-race",
		SessionID:      "session-empty-race",
		RepositoryID:   "repo-empty-race",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		TaskDirName:    "task-empty-race",
		RepoName:       "repo-empty-race",
		RemoteRefState: repoclone.RemoteRefStateEmpty,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD")); got == "" {
		t.Fatal("worktree did not retain the existing local base after the empty-remote race")
	}
	if _, present, err := gitbootstrap.Validate(context.Background(), repoPath, "main"); err != nil {
		t.Fatalf("Validate() error = %v", err)
	} else if present {
		t.Fatal("empty-remote race unexpectedly created a bootstrap marker for an existing local base")
	}
}

func TestCreateReevaluatesAnEmptyRemoteRaceWithRemoteSyncHandled(t *testing.T) {
	cfg := newTestConfig(t)
	mgr, err := NewManager(cfg, newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	repoPath := t.TempDir()
	runGit(t, repoPath, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoPath, "add", "README.md")
	runGit(t, repoPath, "-c", "user.name=Test User", "-c", "user.email=test@example.com", "commit", "-m", "existing")

	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:            "task-empty-race-refreshed",
		SessionID:         "session-empty-race-refreshed",
		RepositoryID:      "repo-empty-race-refreshed",
		RepositoryPath:    repoPath,
		BaseBranch:        "main",
		TaskDirName:       "task-empty-race-refreshed",
		RepoName:          "repo-empty-race-refreshed",
		RemoteSyncHandled: true,
		RemoteRefState:    repoclone.RemoteRefStateEmpty,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD")); got == "" {
		t.Fatal("worktree did not retain the existing local base after the refreshed empty-remote race")
	}
}

func TestCreateRecreatesEmptyRemoteWorktreeFromLocalBaseline(t *testing.T) {
	cfg := newTestConfig(t)
	mgr, err := NewManager(cfg, newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	repoPath := t.TempDir()
	runGit(t, repoPath, "init", "-b", "main")

	req := CreateRequest{
		TaskID:            "task-empty-recreate",
		SessionID:         "session-empty-recreate",
		RepositoryID:      "repo-empty-recreate",
		RepositoryPath:    repoPath,
		BaseBranch:        "main",
		TaskDirName:       "task-empty-recreate",
		RepoName:          "repo-empty-recreate",
		RemoteSyncHandled: true,
		RemoteRefState:    repoclone.RemoteRefStateEmpty,
	}
	first, err := mgr.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("initial Create() error = %v", err)
	}
	if err := os.RemoveAll(first.Path); err != nil {
		t.Fatalf("remove worktree: %v", err)
	}

	recreated, err := mgr.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("recreate Create() error = %v", err)
	}
	if got := strings.TrimSpace(runGit(t, recreated.Path, "rev-parse", "HEAD")); got == "" {
		t.Fatal("recreated empty-remote worktree has no local commit")
	}
}
