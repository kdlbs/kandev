package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPerformFreshBranch_CleanWorkingTree(t *testing.T) {
	isolateGitEnvForTest(t)
	root := t.TempDir()
	repoPath := filepath.Join(root, "repo")
	initRealGitRepo(t, repoPath)

	svc := newDiscoveryService(t, root)
	err := svc.PerformFreshBranch(context.Background(), FreshBranchRequest{
		RepoPath:   repoPath,
		BaseBranch: "main",
		NewBranch:  "feature/x",
	})
	if err != nil {
		t.Fatalf("PerformFreshBranch error: %v", err)
	}
	if got := readCurrentBranch(t, repoPath); got != "feature/x" {
		t.Fatalf("expected current branch feature/x, got %q", got)
	}
}

func TestPerformFreshBranch_DirtyWithoutConfirm(t *testing.T) {
	isolateGitEnvForTest(t)
	root := t.TempDir()
	repoPath := filepath.Join(root, "repo")
	initRealGitRepo(t, repoPath)
	writeDirty(t, repoPath, "untracked.txt", "hi")

	svc := newDiscoveryService(t, root)
	err := svc.PerformFreshBranch(context.Background(), FreshBranchRequest{
		RepoPath:   repoPath,
		BaseBranch: "main",
		NewBranch:  "feature/x",
	})
	var dirty *ErrDirtyWorkingTree
	if !errors.As(err, &dirty) {
		t.Fatalf("expected ErrDirtyWorkingTree, got %v", err)
	}
	if len(dirty.DirtyFiles) == 0 {
		t.Fatalf("expected dirty files in error")
	}
	// Branch must NOT have changed when caller didn't confirm.
	if got := readCurrentBranch(t, repoPath); got != "main" {
		t.Fatalf("expected branch unchanged on rejection, got %q", got)
	}
	// Untracked file must NOT have been deleted.
	if _, err := os.Stat(filepath.Join(repoPath, "untracked.txt")); err != nil {
		t.Fatalf("expected dirty file preserved, stat err: %v", err)
	}
}

func TestPerformFreshBranch_DirtyWithConfirm(t *testing.T) {
	isolateGitEnvForTest(t)
	root := t.TempDir()
	repoPath := filepath.Join(root, "repo")
	initRealGitRepo(t, repoPath)
	writeDirty(t, repoPath, "untracked.txt", "hi")

	svc := newDiscoveryService(t, root)
	err := svc.PerformFreshBranch(context.Background(), FreshBranchRequest{
		RepoPath:       repoPath,
		BaseBranch:     "main",
		NewBranch:      "feature/x",
		ConfirmDiscard: true,
	})
	if err != nil {
		t.Fatalf("PerformFreshBranch error: %v", err)
	}
	if got := readCurrentBranch(t, repoPath); got != "feature/x" {
		t.Fatalf("expected branch feature/x, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(repoPath, "untracked.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected dirty file removed, got err=%v", err)
	}
}

func TestPerformFreshBranch_RejectsEmptyFields(t *testing.T) {
	isolateGitEnvForTest(t)
	root := t.TempDir()
	repoPath := filepath.Join(root, "repo")
	initRealGitRepo(t, repoPath)

	svc := newDiscoveryService(t, root)
	if err := svc.PerformFreshBranch(context.Background(), FreshBranchRequest{
		RepoPath: repoPath, NewBranch: "feature/x",
	}); err == nil {
		t.Fatal("expected error for empty BaseBranch")
	}
	if err := svc.PerformFreshBranch(context.Background(), FreshBranchRequest{
		RepoPath: repoPath, BaseBranch: "main",
	}); err == nil {
		t.Fatal("expected error for empty NewBranch")
	}
}

func TestLocalRepositoryStatus_CleanRepo(t *testing.T) {
	isolateGitEnvForTest(t)
	root := t.TempDir()
	repoPath := filepath.Join(root, "repo")
	initRealGitRepo(t, repoPath)

	svc := newDiscoveryService(t, root)
	status, err := svc.LocalRepositoryStatus(context.Background(), repoPath)
	if err != nil {
		t.Fatalf("LocalRepositoryStatus error: %v", err)
	}
	if status.CurrentBranch != "main" {
		t.Fatalf("expected current branch main, got %q", status.CurrentBranch)
	}
	if len(status.DirtyFiles) != 0 {
		t.Fatalf("expected clean tree, got dirty files: %v", status.DirtyFiles)
	}
}

func TestLocalRepositoryStatus_ListsDirtyFiles(t *testing.T) {
	isolateGitEnvForTest(t)
	root := t.TempDir()
	repoPath := filepath.Join(root, "repo")
	initRealGitRepo(t, repoPath)
	writeDirty(t, repoPath, "a.txt", "hi")
	writeDirty(t, repoPath, "b.txt", "hi")

	svc := newDiscoveryService(t, root)
	status, err := svc.LocalRepositoryStatus(context.Background(), repoPath)
	if err != nil {
		t.Fatalf("LocalRepositoryStatus error: %v", err)
	}
	if len(status.DirtyFiles) != 2 {
		t.Fatalf("expected 2 dirty files, got %d: %v", len(status.DirtyFiles), status.DirtyFiles)
	}
}

// initRealGitRepo creates a git repo with a single commit on `main`.
func initRealGitRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	env := isolatedGitEnv()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "core.hooksPath", "/dev/null"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %s", args, out)
		}
	}
}

func readCurrentBranch(t *testing.T, repoPath string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse failed: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func writeDirty(t *testing.T, repoPath, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repoPath, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write dirty %q: %v", name, err)
	}
}

func isolatedGitEnv() []string {
	env := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		if strings.HasPrefix(key, "GIT_") {
			continue
		}
		env = append(env, e)
	}
	return append(env,
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.local",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.local",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
	)
}

func isolateGitEnvForTest(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE"} {
		if val, ok := os.LookupEnv(key); ok {
			_ = os.Unsetenv(key)
			t.Cleanup(func() { _ = os.Setenv(key, val) })
		}
	}
}
