package lifecycle

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

func newTestLocalLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	return log
}

// initGitRepo creates a minimal git repo with an initial commit and returns the path.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Start from a clean env: filter all GIT_* vars that may leak from parent
	// processes (e.g. pre-commit hooks set GIT_DIR, GIT_WORK_TREE, GIT_INDEX_FILE).
	gitEnv := filterLocalTestGitEnv(os.Environ())
	gitEnv = append(gitEnv,
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.local",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.local",
		"HOME="+dir,
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
	)
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "core.hooksPath", "/dev/null"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = gitEnv
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %s", args, out)
		}
	}
	return dir
}

// filterLocalTestGitEnv removes GIT_* environment variables that can leak from
// parent processes (especially git hooks) and cause test git operations to
// modify the wrong repository.
func filterLocalTestGitEnv(env []string) []string {
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		key, _, _ := strings.Cut(e, "=")
		if strings.HasPrefix(key, "GIT_") {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}

// newIsolatedGitEnv returns a clean environment for test git commands that
// filters leaked GIT_* vars and re-adds isolation + committer identity vars.
func newIsolatedGitEnv() []string {
	env := filterLocalTestGitEnv(os.Environ())
	return append(env,
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.local",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.local",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
	)
}

func currentBranch(t *testing.T, repoDir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse failed: %v", err)
	}
	return string(out[:len(out)-1]) // trim newline
}

// isolateGitEnv sets env vars to prevent git from reading the user's global
// config (which may have commit signing enabled) during tests, and clears
// GIT_* vars that leak from parent git hooks (pre-commit sets GIT_DIR, etc.).
func isolateGitEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	// Unset vars set by git hooks that would redirect commands to the host repo.
	// Cannot use t.Setenv("", "") because GIT_DIR="" makes git fail differently.
	for _, key := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE"} {
		if val, ok := os.LookupEnv(key); ok {
			_ = os.Unsetenv(key)
			t.Cleanup(func() { _ = os.Setenv(key, val) })
		}
	}
}

func TestLocalPreparer_NoCheckoutBranch(t *testing.T) {
	isolateGitEnv(t)
	log := newTestLocalLogger()
	preparer := NewLocalPreparer(log)

	repoDir := initGitRepo(t)
	req := &EnvPrepareRequest{
		TaskID:         "task-1",
		RepositoryPath: repoDir,
	}

	result, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Prepare() failed: %s", result.ErrorMessage)
	}
	// Only 1 step: validate workspace (no checkout step)
	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}
	if result.Steps[0].Name != "Validate workspace" {
		t.Fatalf("expected step name 'Validate workspace', got %q", result.Steps[0].Name)
	}
	// Branch should still be main
	if branch := currentBranch(t, repoDir); branch != "main" {
		t.Fatalf("expected branch 'main', got %q", branch)
	}
}

func TestLocalPreparer_CheckoutBranchSuccess(t *testing.T) {
	isolateGitEnv(t)
	log := newTestLocalLogger()
	preparer := NewLocalPreparer(log)

	repoDir := initGitRepo(t)
	// Create a feature branch
	env := newIsolatedGitEnv()
	for _, args := range [][]string{
		{"checkout", "-b", "feature/pr-branch"},
		{"commit", "--allow-empty", "-m", "pr commit"},
		{"checkout", "main"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %s", args, out)
		}
	}

	req := &EnvPrepareRequest{
		TaskID:         "task-1",
		RepositoryPath: repoDir,
		CheckoutBranch: "feature/pr-branch",
	}

	result, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Prepare() failed: %s", result.ErrorMessage)
	}
	// 2 steps: validate workspace + checkout branch
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(result.Steps))
	}
	if result.Steps[1].Name != "Checkout branch" {
		t.Fatalf("expected step name 'Checkout branch', got %q", result.Steps[1].Name)
	}
	if result.Steps[1].Status != PrepareStepCompleted {
		t.Fatalf("expected checkout step completed, got %q", result.Steps[1].Status)
	}
	// Branch should be feature/pr-branch
	if branch := currentBranch(t, repoDir); branch != "feature/pr-branch" {
		t.Fatalf("expected branch 'feature/pr-branch', got %q", branch)
	}
}

func TestLocalPreparer_CheckoutBranchNotFound(t *testing.T) {
	isolateGitEnv(t)
	log := newTestLocalLogger()
	preparer := NewLocalPreparer(log)

	repoDir := initGitRepo(t)
	req := &EnvPrepareRequest{
		TaskID:         "task-1",
		RepositoryPath: repoDir,
		CheckoutBranch: "nonexistent-branch",
	}

	result, err := preparer.Prepare(context.Background(), req, nil)
	if err == nil {
		t.Fatal("Prepare() should fail when branch doesn't exist")
	}
	if result.Success {
		t.Fatal("expected result.Success = false")
	}
	// 2 steps: validate (completed) + checkout (failed)
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(result.Steps))
	}
	if result.Steps[1].Status != PrepareStepFailed {
		t.Fatalf("expected checkout step failed, got %q", result.Steps[1].Status)
	}
	// Branch should still be main (checkout failed)
	if branch := currentBranch(t, repoDir); branch != "main" {
		t.Fatalf("expected branch 'main' after failed checkout, got %q", branch)
	}
}

func TestLocalPreparer_CheckoutBranchWithSetupScript(t *testing.T) {
	isolateGitEnv(t)
	log := newTestLocalLogger()
	preparer := NewLocalPreparer(log)

	repoDir := initGitRepo(t)
	env := newIsolatedGitEnv()
	for _, args := range [][]string{
		{"checkout", "-b", "feature/pr-branch"},
		{"commit", "--allow-empty", "-m", "pr commit"},
		{"checkout", "main"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %s", args, out)
		}
	}

	markerFile := filepath.Join(repoDir, "setup-ran")
	req := &EnvPrepareRequest{
		TaskID:         "task-1",
		RepositoryPath: repoDir,
		CheckoutBranch: "feature/pr-branch",
		SetupScript:    "touch " + markerFile,
	}

	result, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Prepare() failed: %s", result.ErrorMessage)
	}
	// 3 steps: validate + checkout + setup script
	if len(result.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(result.Steps))
	}
	// Branch checked out before script runs
	if branch := currentBranch(t, repoDir); branch != "feature/pr-branch" {
		t.Fatalf("expected branch 'feature/pr-branch', got %q", branch)
	}
	// Script ran
	if _, err := os.Stat(markerFile); os.IsNotExist(err) {
		t.Fatal("setup script did not run")
	}
}

func TestLocalPreparer_CheckoutWithDirtyWorkdir(t *testing.T) {
	isolateGitEnv(t)
	log := newTestLocalLogger()
	preparer := NewLocalPreparer(log)

	repoDir := initGitRepo(t)
	env := newIsolatedGitEnv()
	// Create feature branch
	for _, args := range [][]string{
		{"checkout", "-b", "feature/pr-branch"},
		{"commit", "--allow-empty", "-m", "pr commit"},
		{"checkout", "main"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %s", args, out)
		}
	}

	// Create a dirty file that conflicts with checkout
	dirtyFile := filepath.Join(repoDir, "dirty.txt")
	if err := os.WriteFile(dirtyFile, []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Stage and modify to create a conflict scenario
	addCmd := exec.Command("git", "add", "dirty.txt")
	addCmd.Dir = repoDir
	addCmd.Env = env
	if out, err := addCmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %s", out)
	}

	// Commit on main so both branches diverge on this file
	commitCmd := exec.Command("git", "commit", "-m", "add dirty on main")
	commitCmd.Dir = repoDir
	commitCmd.Env = env
	if out, err := commitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %s", out)
	}

	// Now modify the file to make checkout fail
	if err := os.WriteFile(dirtyFile, []byte("modified"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := &EnvPrepareRequest{
		TaskID:         "task-1",
		RepositoryPath: repoDir,
		CheckoutBranch: "feature/pr-branch",
	}

	result, err := preparer.Prepare(context.Background(), req, nil)
	if err == nil {
		t.Fatal("Prepare() should fail with dirty workdir")
	}
	if result.Success {
		t.Fatal("expected result.Success = false")
	}
	// Should have 2 steps: validate (completed) + checkout (failed)
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(result.Steps))
	}
	if result.Steps[1].Status != PrepareStepFailed {
		t.Fatalf("expected checkout step failed, got %q", result.Steps[1].Status)
	}
	// Branch should still be main
	if branch := currentBranch(t, repoDir); branch != "main" {
		t.Fatalf("expected branch 'main' after failed checkout, got %q", branch)
	}
}

func TestLocalPreparer_BaseBranchCheckout(t *testing.T) {
	isolateGitEnv(t)
	log := newTestLocalLogger()
	preparer := NewLocalPreparer(log)

	repoDir := initGitRepo(t)
	env := newIsolatedGitEnv()
	// Create a feature branch
	for _, args := range [][]string{
		{"checkout", "-b", "develop"},
		{"commit", "--allow-empty", "-m", "develop commit"},
		{"checkout", "main"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %s", args, out)
		}
	}

	req := &EnvPrepareRequest{
		TaskID:         "task-1",
		RepositoryPath: repoDir,
		BaseBranch:     "develop",
	}

	result, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Prepare() failed: %s", result.ErrorMessage)
	}
	// 2 steps: validate workspace + checkout branch
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(result.Steps))
	}
	if result.Steps[1].Name != "Checkout branch" {
		t.Fatalf("expected step name 'Checkout branch', got %q", result.Steps[1].Name)
	}
	// Branch should be develop
	if branch := currentBranch(t, repoDir); branch != "develop" {
		t.Fatalf("expected branch 'develop', got %q", branch)
	}
}

// TestLocalPreparer_FreshBranchInputs covers the post-fresh-branch state: the
// HTTP layer now persists the new branch name as the task's BaseBranch, so the
// preparer must successfully checkout that branch even when it was just
// created moments earlier. This is essentially the existing BaseBranch path —
// the test exists to catch any regression that breaks resume after a fresh
// checkout.
func TestLocalPreparer_FreshBranchPersistedAsBaseBranch(t *testing.T) {
	isolateGitEnv(t)
	log := newTestLocalLogger()
	preparer := NewLocalPreparer(log)

	repoDir := initGitRepo(t)
	env := newIsolatedGitEnv()
	// Simulate fresh-branch having just created "feature/new" from main.
	for _, args := range [][]string{
		{"checkout", "-b", "feature/new"},
		{"commit", "--allow-empty", "-m", "fresh"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %s", args, out)
		}
	}

	req := &EnvPrepareRequest{
		TaskID:         "task-1",
		RepositoryPath: repoDir,
		BaseBranch:     "feature/new",
	}
	result, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Prepare() failed: %s", result.ErrorMessage)
	}
	if branch := currentBranch(t, repoDir); branch != "feature/new" {
		t.Fatalf("expected branch 'feature/new', got %q", branch)
	}
}

func TestLocalPreparer_CheckoutBranchPriorityOverBaseBranch(t *testing.T) {
	isolateGitEnv(t)
	log := newTestLocalLogger()
	preparer := NewLocalPreparer(log)

	repoDir := initGitRepo(t)
	env := newIsolatedGitEnv()
	// Create two branches
	for _, args := range [][]string{
		{"checkout", "-b", "develop"},
		{"commit", "--allow-empty", "-m", "develop commit"},
		{"checkout", "main"},
		{"checkout", "-b", "feature/pr-branch"},
		{"commit", "--allow-empty", "-m", "pr commit"},
		{"checkout", "main"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %s", args, out)
		}
	}

	req := &EnvPrepareRequest{
		TaskID:         "task-1",
		RepositoryPath: repoDir,
		BaseBranch:     "develop",
		CheckoutBranch: "feature/pr-branch",
	}

	result, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Prepare() failed: %s", result.ErrorMessage)
	}
	// CheckoutBranch should win over BaseBranch
	if branch := currentBranch(t, repoDir); branch != "feature/pr-branch" {
		t.Fatalf("expected branch 'feature/pr-branch' (CheckoutBranch priority), got %q", branch)
	}
}
