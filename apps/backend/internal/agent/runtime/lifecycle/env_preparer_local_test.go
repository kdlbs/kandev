package lifecycle

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/worktree"
)

func newTestLocalLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	return log
}

func TestLocalPreparer_ReuseRequiredSkipsCheckoutAndSetup(t *testing.T) {
	workspacePath := t.TempDir()
	marker := filepath.Join(workspacePath, "setup-ran")
	preparer := NewLocalPreparer(newTestLocalLogger())

	result, err := preparer.Prepare(context.Background(), &EnvPrepareRequest{
		WorkspacePath:          workspacePath,
		CheckoutBranch:         "must-not-be-checked-out",
		SetupScript:            "touch " + marker,
		WorkspaceReuseRequired: true,
	}, nil)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("Prepare() result = %#v, want success", result)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reuse-required preparation ran setup script: stat error = %v", err)
	}
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

// TestLocalPreparer_SkipsCheckoutWhenAlreadyOnBranch is the regression for
// the local-mode bug where Prepare ran `git checkout main` even when the
// workspace was already on `main`. The unconditional checkout failed on
// dirty/unmerged worktrees ("you need to resolve your current index first")
// and surfaced as "Failed to launch agent" for the user even though they
// hadn't asked to switch branches. Local mode treats "request branch matches
// current branch" as "use current state" and skips the git ops entirely.
func TestLocalPreparer_SkipsCheckoutWhenAlreadyOnBranch(t *testing.T) {
	isolateGitEnv(t)
	log := newTestLocalLogger()
	preparer := NewLocalPreparer(log)

	repoDir := initGitRepo(t)
	env := newIsolatedGitEnv()

	// Stage a dirty/unmerged index that would reject `git checkout` outright.
	conflictFile := filepath.Join(repoDir, "package-lock.json")
	if err := os.WriteFile(conflictFile, []byte("conflict\n"), 0o644); err != nil {
		t.Fatalf("write conflict file: %v", err)
	}
	for _, args := range [][]string{
		{"add", "package-lock.json"},
		{"update-index", "--unresolve", "package-lock.json"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = env
		_ = cmd.Run() // best-effort — older git may not have --unresolve
	}
	startBranch := currentBranch(t, repoDir)

	req := &EnvPrepareRequest{
		TaskID:         "task-1",
		RepositoryPath: repoDir,
		BaseBranch:     "main", // matches current — should be a no-op
	}

	result, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Prepare() failed: %s", result.ErrorMessage)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(result.Steps))
	}
	step := result.Steps[1]
	if step.Name != "Checkout branch" {
		t.Fatalf("expected step name 'Checkout branch', got %q", step.Name)
	}
	if step.Status != PrepareStepCompleted {
		t.Fatalf("expected step completed, got %q", step.Status)
	}
	if !strings.Contains(step.Output, "skipping") {
		t.Fatalf("expected output to indicate skip, got %q", step.Output)
	}
	if got := currentBranch(t, repoDir); got != startBranch {
		t.Fatalf("local preparer modified branch: was %q, now %q", startBranch, got)
	}
}

// TestLocalPreparer_ChecksOutDifferentBranch covers the explicit-switch case:
// the user picked a different existing branch in the chip. The preparer must
// switch the working tree to that branch so the agent runs against it.
func TestLocalPreparer_ChecksOutDifferentBranch(t *testing.T) {
	isolateGitEnv(t)
	log := newTestLocalLogger()
	preparer := NewLocalPreparer(log)

	repoDir := initGitRepo(t)
	env := newIsolatedGitEnv()
	for _, args := range [][]string{
		{"checkout", "-b", "feature/existing"},
		{"commit", "--allow-empty", "-m", "feature commit"},
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
		BaseBranch:     "feature/existing",
	}

	result, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Prepare() failed: %s", result.ErrorMessage)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(result.Steps))
	}
	if result.Steps[1].Name != "Checkout branch" {
		t.Fatalf("expected step name 'Checkout branch', got %q", result.Steps[1].Name)
	}
	if result.Steps[1].Status != PrepareStepCompleted {
		t.Fatalf("expected checkout step completed, got %q", result.Steps[1].Status)
	}
	if got := currentBranch(t, repoDir); got != "feature/existing" {
		t.Fatalf("expected branch 'feature/existing', got %q", got)
	}
}

// TestLocalPreparer_CheckoutBranchPriorityOverBaseBranch keeps the PR-watch
// flow working: when both fields are set, CheckoutBranch (PR head) wins.
func TestLocalPreparer_CheckoutBranchPriorityOverBaseBranch(t *testing.T) {
	isolateGitEnv(t)
	log := newTestLocalLogger()
	preparer := NewLocalPreparer(log)

	repoDir := initGitRepo(t)
	env := newIsolatedGitEnv()
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
	if got := currentBranch(t, repoDir); got != "feature/pr-branch" {
		t.Fatalf("expected branch 'feature/pr-branch' (CheckoutBranch priority), got %q", got)
	}
}

func TestLocalPreparer_UsesAuthenticatedRefreshWithoutSecondFetch(t *testing.T) {
	isolateGitEnv(t)
	repoDir := initGitRepo(t)
	env := newIsolatedGitEnv()
	for _, args := range [][]string{
		{"checkout", "-b", "feature/refreshed"},
		{"commit", "--allow-empty", "-m", "refreshed branch"},
		{"checkout", "main"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %s", args, out)
		}
	}

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	fakeBin := t.TempDir()
	fetchMarker := filepath.Join(t.TempDir(), "unexpected-fetch")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = fetch ]; then : > \"$FETCH_MARKER\"; exit 99; fi\n" +
		"exec \"$REAL_GIT\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("REAL_GIT", realGit)
	t.Setenv("FETCH_MARKER", fetchMarker)

	result, err := NewLocalPreparer(newTestLocalLogger()).Prepare(context.Background(), &EnvPrepareRequest{
		TaskID: "task-1", RepositoryPath: repoDir, CheckoutBranch: "feature/refreshed",
		RemoteSyncHandled: true,
	}, nil)
	if err != nil || !result.Success {
		t.Fatalf("Prepare() = success %v, error %v", result.Success, err)
	}
	if _, err := os.Stat(fetchMarker); !os.IsNotExist(err) {
		t.Fatal("local preparer performed a second unauthenticated fetch")
	}
	if got := currentBranch(t, repoDir); got != "feature/refreshed" {
		t.Fatalf("branch = %q, want feature/refreshed", got)
	}
}

// TestLocalPreparer_CheckoutFailureSurfaces ensures actual checkout failures
// (e.g. trying to switch branches with a dirty workdir) bubble up as a
// failed step + non-nil error so the orchestrator marks the task FAILED.
func TestLocalPreparer_CheckoutFailureSurfaces(t *testing.T) {
	isolateGitEnv(t)
	log := newTestLocalLogger()
	preparer := NewLocalPreparer(log)

	repoDir := initGitRepo(t)
	req := &EnvPrepareRequest{
		TaskID:         "task-1",
		RepositoryPath: repoDir,
		CheckoutBranch: "nonexistent-branch", // current is "main", so checkout fires and fails
	}

	result, err := preparer.Prepare(context.Background(), req, nil)
	if err == nil {
		t.Fatal("expected error for missing branch")
	}
	if result.Success {
		t.Fatal("expected result.Success = false")
	}
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps (validate + failed checkout), got %d", len(result.Steps))
	}
	if result.Steps[1].Status != PrepareStepFailed {
		t.Fatalf("expected checkout step failed, got %q", result.Steps[1].Status)
	}
	if got := currentBranch(t, repoDir); got != "main" {
		t.Fatalf("expected branch unchanged on failure, got %q", got)
	}
}

func TestLocalPreparer_PreservesFetchFailureWhenCheckoutAlsoFails(t *testing.T) {
	const token = "github-token-that-must-not-leak"
	fakeBin := t.TempDir()
	fakeGit := filepath.Join(fakeBin, "git")
	script := `#!/bin/sh
case "$1" in
fetch)
  echo "fatal: unable to access 'https://oauth2:github-token-that-must-not-leak@github.com/kdlbs/kandev.git/': Could not resolve host: github.com" >&2
  exit 128
  ;;
checkout)
  echo "error: pathspec '$2' did not match any file(s) known to git; token=github-token-that-must-not-leak" >&2
  exit 1
  ;;
esac
`
	if err := os.WriteFile(fakeGit, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	preparer := NewLocalPreparer(newTestLocalLogger())
	result, err := preparer.Prepare(context.Background(), &EnvPrepareRequest{
		TaskID:         "task-1",
		RepositoryPath: t.TempDir(),
		CheckoutBranch: "feature/missing",
		Env:            map[string]string{"GITHUB_TOKEN": token},
	}, nil)
	if err == nil {
		t.Fatal("expected Prepare() to fail")
	}
	for _, want := range []string{"fetch branch failed", "Could not resolve host: github.com", "pathspec 'feature/missing' did not match", "[REDACTED]"} {
		if !strings.Contains(result.ErrorMessage, want) || !strings.Contains(err.Error(), want) {
			t.Fatalf("expected result and error to contain %q\nresult: %s\nerror: %v", want, result.ErrorMessage, err)
		}
	}
	for _, secret := range []string{"oauth2", token} {
		if strings.Contains(result.ErrorMessage, secret) || strings.Contains(err.Error(), secret) {
			t.Fatalf("checkout failure leaked %q\nresult: %s\nerror: %v", secret, result.ErrorMessage, err)
		}
	}
}

func TestLocalPreparer_RunsSetupScriptWithoutCheckout(t *testing.T) {
	isolateGitEnv(t)
	log := newTestLocalLogger()
	preparer := NewLocalPreparer(log)

	repoDir := initGitRepo(t)
	startBranch := currentBranch(t, repoDir)
	markerFile := filepath.Join(repoDir, "setup-ran")
	req := &EnvPrepareRequest{
		TaskID:         "task-1",
		RepositoryPath: repoDir,
		// BaseBranch left empty — "use current" path, no checkout step at all.
		SetupScript: "touch " + markerFile,
	}

	result, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Prepare() failed: %s", result.ErrorMessage)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps (validate + setup), got %d: %+v", len(result.Steps), result.Steps)
	}
	if result.Steps[0].Name != "Validate workspace" {
		t.Fatalf("expected step 0 'Validate workspace', got %q", result.Steps[0].Name)
	}
	if result.Steps[1].Name == "Checkout branch" {
		t.Fatal("expected no checkout step when BaseBranch is empty")
	}
	if _, err := os.Stat(markerFile); os.IsNotExist(err) {
		t.Fatal("setup script did not run")
	}
	if got := currentBranch(t, repoDir); got != startBranch {
		t.Fatalf("local preparer modified branch: was %q, now %q", startBranch, got)
	}
}

// TestLocalPreparer_BranchCheckedOutElsewhereFailsClosed is the regression for
// the bug where a second task's environment creation fails when its target
// branch is already checked out in another worktree AND the remote no longer
// carries that ref. The local preparer must fail closed with a typed
// ErrBranchCheckedOut error so the orchestrator does not misclassify the
// failure as a missing-branch fetch failure and post PR-recovery guidance for
// a branch that is, in fact, present locally.
//
// Before the fix, checkoutBranch concatenated the fetch stderr ("couldn't find
// remote ref") with the checkout stderr ("is already checked out at ..."); the
// combined message made the orchestrator's substring-based
// isMissingBranchError match the "couldn't find remote ref" fragment and emit
// branch_fetch_failed guidance — wrong because the local branch exists, just
// not in this working tree.
//
// The shared checkout's HEAD and index must also be untouched: switching to
// the target branch was never possible here, and partial state changes (e.g.
// the fetch updating FETCH_HEAD or the index) would corrupt another task's
// workspace.
func TestLocalPreparer_BranchCheckedOutElsewhereFailsClosed(t *testing.T) {
	isolateGitEnv(t)
	repoDir := initGitRepo(t)
	env := newIsolatedGitEnv()

	for _, args := range [][]string{
		{"checkout", "-b", "feature/shared"},
		{"commit", "--allow-empty", "-m", "shared work"},
		{"checkout", "main"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %s", args, out)
		}
	}

	// Use a sibling git worktree so the real git binary (used for setup)
	// knows the branch is checked out elsewhere. This mirrors what the user
	// hit: another task's worktree holds the branch.
	sibling := filepath.Join(t.TempDir(), "sibling")
	siblingCmd := exec.Command("git", "worktree", "add", sibling, "feature/shared")
	siblingCmd.Dir = repoDir
	siblingCmd.Env = env
	if out, err := siblingCmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add failed: %s", out)
	}
	t.Cleanup(func() {
		cleanupCmd := exec.Command("git", "worktree", "remove", "--force", sibling)
		cleanupCmd.Dir = repoDir
		cleanupCmd.Env = env
		_ = cleanupCmd.Run()
	})

	// Snapshot HEAD + index before the failing prepare so we can prove the
	// shared checkout was untouched.
	headBefore := readHeadSHA(t, repoDir)
	indexBefore := readIndexSHA(t, repoDir)
	branchBefore := currentBranch(t, repoDir)

	// Fake only fetch: it supplies the missing-remote-ref text that triggers
	// the substring match, then lets the real checkout detect the sibling
	// worktree and emit its "already checked out at" diagnostic. Pass through
	// everything else (symbolic-ref, rev-parse, ls-files, worktree, etc.) by
	// exec'ing the real git at its absolute path — re-resolving via PATH would
	// loop back to this fake.
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("locate real git: %v", err)
	}
	fakeBin := t.TempDir()
	fakeGit := filepath.Join(fakeBin, "git")
	script := `#!/bin/sh
case "$1" in
fetch)
  echo "fatal: couldn't find remote ref feature/shared" >&2
  exit 128
  ;;
esac
exec "` + realGit + `" "$@"
`
	if err := os.WriteFile(fakeGit, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	result, err := NewLocalPreparer(newTestLocalLogger()).Prepare(context.Background(), &EnvPrepareRequest{
		TaskID:         "task-2",
		RepositoryPath: repoDir,
		BaseBranch:     "feature/shared",
	}, nil)
	if err == nil {
		t.Fatal("expected Prepare() to fail when branch is checked out elsewhere")
	}
	if result.Success {
		t.Fatalf("expected result.Success = false; error=%v", err)
	}
	if !errors.Is(err, worktree.ErrBranchCheckedOut) {
		t.Fatalf("expected error to wrap worktree.ErrBranchCheckedOut, got %v", err)
	}
	// Combined fetch+checkout stderr must remain visible (the operator still
	// needs to see what happened) — this is the same string content that
	// previously fooled isMissingBranchError into a false match. The typed
	// sentinel is what prevents the misclassification.
	if !strings.Contains(err.Error(), "couldn't find remote ref") {
		t.Fatalf("expected fetch stderr to be preserved in error chain, got %v", err)
	}
	if !strings.Contains(err.Error(), "already checked out at") &&
		!strings.Contains(err.Error(), "already used by worktree at") {
		t.Fatalf("expected real checkout stderr to be preserved in error chain, got %v", err)
	}

	if got := currentBranch(t, repoDir); got != branchBefore {
		t.Fatalf("local preparer changed branch: was %q, now %q", branchBefore, got)
	}
	if got := readHeadSHA(t, repoDir); got != headBefore {
		t.Fatalf("local preparer changed HEAD: was %s, now %s", headBefore, got)
	}
	if got := readIndexSHA(t, repoDir); got != indexBefore {
		t.Fatalf("local preparer changed index: was %s, now %s", indexBefore, got)
	}
}

func readHeadSHA(t *testing.T, repoDir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD failed: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func readIndexSHA(t *testing.T, repoDir string) string {
	t.Helper()
	cmd := exec.Command("git", "ls-files", "--stage")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files failed: %v", err)
	}
	hash := sha1.New()
	hash.Write(out)
	return hex.EncodeToString(hash.Sum(nil))
}
