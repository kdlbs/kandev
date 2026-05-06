package configloader

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

// initBareRepo creates a bare git repository for use as a remote.
// `-c init.defaultBranch=main` forces the bare repo's HEAD to refs/heads/main
// regardless of the runner's git config — without this override, a vanilla
// git defaults HEAD to refs/heads/master, the test pushes only `main`, and
// any subsequent `git clone <bare>` ends up with an empty working tree
// ("remote HEAD refers to nonexistent ref"), making downstream reads fail.
func initBareRepo(t *testing.T) string {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "remote.git")
	run(t, "", "git", "-c", "init.defaultBranch=main", "init", "--bare", bare)
	return bare
}

// initRepoWithContent creates a git repo with an initial commit containing a kandev.yml.
func initRepoWithContent(t *testing.T, bareURL string) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "git", "init", "-b", "main")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")
	writeFile(t, filepath.Join(dir, "kandev.yml"), "name: test-ws\n")
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-m", "initial")
	if bareURL != "" {
		run(t, dir, "git", "remote", "add", "origin", bareURL)
		run(t, dir, "git", "push", "-u", "origin", "main")
	}
	return dir
}

// run is a test helper that runs a command and fails on error.
func run(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
	return string(out)
}

func newTestGitManager(t *testing.T, basePath string) *GitManager {
	t.Helper()
	loader := NewConfigLoader(basePath)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("NewLogger(): %v", err)
	}
	return NewGitManager(basePath, loader, log)
}

func TestIsGitWorkspace(t *testing.T) {
	base := t.TempDir()
	wsDir := filepath.Join(base, "workspaces", "my-ws")
	mkdirAll(t, wsDir)

	gm := newTestGitManager(t, base)

	if gm.IsGitWorkspace("my-ws") {
		t.Error("should not detect git workspace without .git")
	}

	// Init a git repo in the workspace.
	run(t, wsDir, "git", "init")

	if !gm.IsGitWorkspace("my-ws") {
		t.Error("should detect git workspace with .git")
	}
}

func TestIsGitWorkspace_Nonexistent(t *testing.T) {
	base := t.TempDir()
	gm := newTestGitManager(t, base)

	if gm.IsGitWorkspace("nonexistent") {
		t.Error("nonexistent workspace should not be a git workspace")
	}
}

func TestCloneWorkspace(t *testing.T) {
	bare := initBareRepo(t)
	initRepoWithContent(t, bare)

	base := t.TempDir()
	mkdirAll(t, filepath.Join(base, "workspaces"))
	gm := newTestGitManager(t, base)

	ctx := context.Background()
	if err := gm.CloneWorkspace(ctx, bare, "main", "cloned"); err != nil {
		t.Fatalf("CloneWorkspace(): %v", err)
	}

	// Verify .git exists.
	if !gm.IsGitWorkspace("cloned") {
		t.Error("cloned workspace should have .git")
	}

	// Verify kandev.yml was cloned.
	content, err := os.ReadFile(filepath.Join(base, "workspaces", "cloned", "kandev.yml"))
	if err != nil {
		t.Fatalf("read kandev.yml: %v", err)
	}
	if string(content) != "name: test-ws\n" {
		t.Errorf("unexpected content: %q", content)
	}

	// Verify config loader picked up the workspace.
	ws, err := gm.loader.GetWorkspace("cloned")
	if err != nil {
		t.Fatalf("workspace not loaded after clone: %v", err)
	}
	if ws.Settings.Name != "test-ws" {
		t.Errorf("settings name = %q, want %q", ws.Settings.Name, "test-ws")
	}
}

func TestCloneWorkspace_ExistingRepoPulls(t *testing.T) {
	bare := initBareRepo(t)
	initRepoWithContent(t, bare)

	base := t.TempDir()
	mkdirAll(t, filepath.Join(base, "workspaces"))
	gm := newTestGitManager(t, base)
	ctx := context.Background()

	// First clone.
	if err := gm.CloneWorkspace(ctx, bare, "main", "existing"); err != nil {
		t.Fatalf("first CloneWorkspace(): %v", err)
	}

	// Second clone should pull instead of failing.
	if err := gm.CloneWorkspace(ctx, bare, "main", "existing"); err != nil {
		t.Fatalf("second CloneWorkspace() should pull: %v", err)
	}
}

func TestPullWorkspace(t *testing.T) {
	bare := initBareRepo(t)
	upstream := initRepoWithContent(t, bare)

	base := t.TempDir()
	mkdirAll(t, filepath.Join(base, "workspaces"))
	gm := newTestGitManager(t, base)
	ctx := context.Background()

	// Clone.
	if err := gm.CloneWorkspace(ctx, bare, "main", "pulltest"); err != nil {
		t.Fatalf("CloneWorkspace(): %v", err)
	}

	// Push a new commit from upstream.
	mkdirAll(t, filepath.Join(upstream, "agents"))
	writeFile(t, filepath.Join(upstream, "agents", "new-agent.yml"), "name: new-agent\nrole: worker\n")
	run(t, upstream, "git", "add", "-A")
	run(t, upstream, "git", "commit", "-m", "add agent")
	run(t, upstream, "git", "push")

	// Pull.
	if err := gm.PullWorkspace(ctx, "pulltest"); err != nil {
		t.Fatalf("PullWorkspace(): %v", err)
	}

	// Verify the new file exists.
	agentFile := filepath.Join(base, "workspaces", "pulltest", "agents", "new-agent.yml")
	if _, err := os.Stat(agentFile); err != nil {
		t.Fatalf("new-agent.yml should exist after pull: %v", err)
	}
}

func TestPullWorkspace_NotGitRepo(t *testing.T) {
	base := t.TempDir()
	mkdirAll(t, filepath.Join(base, "workspaces", "plain"))
	gm := newTestGitManager(t, base)

	err := gm.PullWorkspace(context.Background(), "plain")
	if err == nil {
		t.Fatal("expected error for non-git workspace")
	}
}

func TestPushWorkspace(t *testing.T) {
	bare := initBareRepo(t)
	initRepoWithContent(t, bare)

	base := t.TempDir()
	mkdirAll(t, filepath.Join(base, "workspaces"))
	gm := newTestGitManager(t, base)
	ctx := context.Background()

	// Clone.
	if err := gm.CloneWorkspace(ctx, bare, "main", "pushtest"); err != nil {
		t.Fatalf("CloneWorkspace(): %v", err)
	}

	// Configure git user in clone for commits.
	wsPath := filepath.Join(base, "workspaces", "pushtest")
	run(t, wsPath, "git", "config", "user.email", "test@test.com")
	run(t, wsPath, "git", "config", "user.name", "Test")

	// Make a change.
	writeFile(t, filepath.Join(wsPath, "kandev.yml"), "name: pushtest\ndescription: updated\n")

	// Push.
	if err := gm.PushWorkspace(ctx, "pushtest", "update settings"); err != nil {
		t.Fatalf("PushWorkspace(): %v", err)
	}

	// Verify push went through by cloning the bare repo again.
	verifyDir := t.TempDir()
	run(t, "", "git", "clone", bare, verifyDir)
	content, err := os.ReadFile(filepath.Join(verifyDir, "kandev.yml"))
	if err != nil {
		t.Fatalf("read verified kandev.yml: %v", err)
	}
	if got := string(content); got != "name: pushtest\ndescription: updated\n" {
		t.Errorf("pushed content = %q, want updated version", got)
	}
}

func TestPushWorkspace_NothingToCommit(t *testing.T) {
	bare := initBareRepo(t)
	initRepoWithContent(t, bare)

	base := t.TempDir()
	mkdirAll(t, filepath.Join(base, "workspaces"))
	gm := newTestGitManager(t, base)
	ctx := context.Background()

	if err := gm.CloneWorkspace(ctx, bare, "main", "noop"); err != nil {
		t.Fatalf("CloneWorkspace(): %v", err)
	}

	// Push with no changes should be a no-op.
	if err := gm.PushWorkspace(ctx, "noop", "no changes"); err != nil {
		t.Fatalf("PushWorkspace() with no changes should not error: %v", err)
	}
}

func TestPushWorkspace_NotGitRepo(t *testing.T) {
	base := t.TempDir()
	mkdirAll(t, filepath.Join(base, "workspaces", "plain"))
	gm := newTestGitManager(t, base)

	err := gm.PushWorkspace(context.Background(), "plain", "msg")
	if err == nil {
		t.Fatal("expected error for non-git workspace")
	}
}

func TestGetWorkspaceGitStatus(t *testing.T) {
	bare := initBareRepo(t)
	initRepoWithContent(t, bare)

	base := t.TempDir()
	mkdirAll(t, filepath.Join(base, "workspaces"))
	gm := newTestGitManager(t, base)
	ctx := context.Background()

	if err := gm.CloneWorkspace(ctx, bare, "main", "statustest"); err != nil {
		t.Fatalf("CloneWorkspace(): %v", err)
	}

	status, err := gm.GetWorkspaceGitStatus(ctx, "statustest")
	if err != nil {
		t.Fatalf("GetWorkspaceGitStatus(): %v", err)
	}

	if status.Branch != "main" {
		t.Errorf("branch = %q, want %q", status.Branch, "main")
	}
	if status.IsDirty {
		t.Error("workspace should not be dirty after clone")
	}
	if !status.HasRemote {
		t.Error("cloned workspace should have remote")
	}
	if status.CommitCount < 1 {
		t.Errorf("commit_count = %d, want >= 1", status.CommitCount)
	}
}

func TestGetWorkspaceGitStatus_Dirty(t *testing.T) {
	bare := initBareRepo(t)
	initRepoWithContent(t, bare)

	base := t.TempDir()
	mkdirAll(t, filepath.Join(base, "workspaces"))
	gm := newTestGitManager(t, base)
	ctx := context.Background()

	if err := gm.CloneWorkspace(ctx, bare, "main", "dirtytest"); err != nil {
		t.Fatalf("CloneWorkspace(): %v", err)
	}

	// Make an uncommitted change.
	wsPath := filepath.Join(base, "workspaces", "dirtytest")
	writeFile(t, filepath.Join(wsPath, "new-file.txt"), "hello")

	status, err := gm.GetWorkspaceGitStatus(ctx, "dirtytest")
	if err != nil {
		t.Fatalf("GetWorkspaceGitStatus(): %v", err)
	}

	if !status.IsDirty {
		t.Error("workspace should be dirty after uncommitted change")
	}
}

func TestGetWorkspaceGitStatus_NotGitRepo(t *testing.T) {
	base := t.TempDir()
	mkdirAll(t, filepath.Join(base, "workspaces", "plain"))
	gm := newTestGitManager(t, base)

	_, err := gm.GetWorkspaceGitStatus(context.Background(), "plain")
	if err == nil {
		t.Fatal("expected error for non-git workspace")
	}
}
