package api

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

// TestComputeMergeBase_PrefersOriginOverStaleLocalBranch reproduces the bug
// from the user-reported "111 commits in panel" debug payload. The worktree's
// local `main` ref had fallen far behind `origin/main`. Computing merge-base
// against local `main` returned an old SHA and the log range swept in dozens
// of unrelated commits. The fix is to prefer `origin/<target_branch>` so the
// merge-base reflects the upstream's actual state.
func TestComputeMergeBase_PrefersOriginOverStaleLocalBranch(t *testing.T) {
	repoDir, cleanup := setupAPITestRepo(t)
	defer cleanup()

	staleLocalMain := strings.TrimSpace(runGitAPI(t, repoDir, "rev-parse", "HEAD"))

	// Move both main and origin/main forward, then reset local main to its
	// old SHA — origin/main is now ahead, local main is stale.
	for i := 0; i < 3; i++ {
		writeFileAPI(t, repoDir, "main-x.txt", strings.Repeat("main x\n", i+1))
		runGitAPI(t, repoDir, "add", ".")
		runGitAPI(t, repoDir, "commit", "-m", "chore: main forward")
	}
	advancedMain := strings.TrimSpace(runGitAPI(t, repoDir, "rev-parse", "HEAD"))
	runGitAPI(t, repoDir, "push", "origin", "main")
	runGitAPI(t, repoDir, "reset", "--hard", staleLocalMain)
	runGitAPI(t, repoDir, "fetch", "origin", "main:refs/remotes/origin/main")

	// Branch from the (advanced) origin/main and add one commit.
	runGitAPI(t, repoDir, "checkout", "-b", "feature/x", advancedMain)
	writeFileAPI(t, repoDir, "feature.txt", "feature work\n")
	runGitAPI(t, repoDir, "add", ".")
	runGitAPI(t, repoDir, "commit", "-m", "feat: feature work")

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	srv := &Server{logger: log}
	gitOp := process.NewGitOperator(repoDir, log, nil)

	sha, err := srv.computeMergeBase(context.Background(), gitOp, "main")
	if err != nil {
		t.Fatalf("computeMergeBase returned error: %v", err)
	}
	if sha != advancedMain {
		t.Errorf("expected merge-base = %s (origin/main tip), got %s — likely fell back to stale local main %s",
			advancedMain, sha, staleLocalMain)
	}
}

// TestComputeMergeBase_FallsBackToLocalWhenRemoteMissing covers the case
// where `origin/<target_branch>` doesn't exist (e.g. unfetched remote, or
// a branch that only lives locally). The implementation must not error out
// — it must fall back to the local ref so log filtering still works.
func TestComputeMergeBase_FallsBackToLocalWhenRemoteMissing(t *testing.T) {
	repoDir, cleanup := setupAPITestRepo(t)
	defer cleanup()

	mainSHA := strings.TrimSpace(runGitAPI(t, repoDir, "rev-parse", "HEAD"))
	runGitAPI(t, repoDir, "checkout", "-b", "feature/x")
	writeFileAPI(t, repoDir, "feature.txt", "feature work\n")
	runGitAPI(t, repoDir, "add", ".")
	runGitAPI(t, repoDir, "commit", "-m", "feat: feature work")
	// Delete origin/<some-other-branch> to ensure it doesn't exist; we'll
	// query merge-base against that branch name.
	runGitAPI(t, repoDir, "branch", "develop", "main")

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	srv := &Server{logger: log}
	gitOp := process.NewGitOperator(repoDir, log, nil)

	sha, err := srv.computeMergeBase(context.Background(), gitOp, "develop")
	if err != nil {
		t.Fatalf("computeMergeBase returned error when remote missing: %v", err)
	}
	if sha != mainSHA {
		t.Errorf("expected merge-base = %s (local develop tip), got %s", mainSHA, sha)
	}
}

// TestComputeMergeBase_CorrectAnchorForCumulativeDiff documents that the
// shared computeMergeBase helper — which both the commit log and the
// cumulative diff paths consume — returns the right anchor in the
// stale-local / fresh-origin shape. It does not exercise
// runGitCumulativeDiffForRepo end-to-end (that would need a Server +
// httptest stack); the integration is structurally trivial since both
// callers route through this helper.
func TestComputeMergeBase_CorrectAnchorForCumulativeDiff(t *testing.T) {
	repoDir, cleanup := setupAPITestRepo(t)
	defer cleanup()

	// Capture initial main as the "stored base" — what the kandev session
	// would have recorded at session-start time.
	storedBase := strings.TrimSpace(runGitAPI(t, repoDir, "rev-parse", "HEAD"))

	// Push main forward (committing changes that should be excluded from
	// the feature's diff because they belong to main).
	for i := 0; i < 3; i++ {
		writeFileAPI(t, repoDir, "main-x.txt", strings.Repeat("main x\n", i+1))
		runGitAPI(t, repoDir, "add", ".")
		runGitAPI(t, repoDir, "commit", "-m", "chore: main forward")
	}
	runGitAPI(t, repoDir, "push", "origin", "main")
	advancedMain := strings.TrimSpace(runGitAPI(t, repoDir, "rev-parse", "HEAD"))

	// Branch off the *advanced* main and add one commit. Local main stays
	// at storedBase to simulate a stale worktree.
	runGitAPI(t, repoDir, "checkout", "-b", "feature/x", advancedMain)
	writeFileAPI(t, repoDir, "feature.txt", "feature work\n")
	runGitAPI(t, repoDir, "add", ".")
	runGitAPI(t, repoDir, "commit", "-m", "feat: feature work")
	runGitAPI(t, repoDir, "checkout", "main")
	runGitAPI(t, repoDir, "reset", "--hard", storedBase)
	runGitAPI(t, repoDir, "checkout", "feature/x")

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	srv := &Server{logger: log}
	gitOp := process.NewGitOperator(repoDir, log, nil)

	// Sanity-check: merge-base against origin/main is the advanced tip,
	// proving computeMergeBase returns the right anchor.
	sha, err := srv.computeMergeBase(context.Background(), gitOp, "main")
	if err != nil {
		t.Fatalf("computeMergeBase failed: %v", err)
	}
	if sha != advancedMain {
		t.Errorf("expected merge-base = %s (origin/main), got %s", advancedMain, sha)
	}
	// And it's not the stored base (which would have been the buggy result
	// before the fix).
	if sha == storedBase {
		t.Errorf("merge-base fell back to stored base %s — origin path didn't take", storedBase)
	}
}

// --- test scaffolding (api package can't reuse process_test helpers) ---

func setupAPITestRepo(t *testing.T) (string, func()) {
	t.Helper()
	remoteDir, err := os.MkdirTemp("", "api-test-remote-*")
	if err != nil {
		t.Fatalf("failed to create remote dir: %v", err)
	}
	localDir, err := os.MkdirTemp("", "api-test-local-*")
	if err != nil {
		_ = os.RemoveAll(remoteDir)
		t.Fatalf("failed to create local dir: %v", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(remoteDir)
		_ = os.RemoveAll(localDir)
	}

	runGitAPI(t, remoteDir, "init", "--bare", "--initial-branch=main")
	runGitAPI(t, localDir, "init", "--initial-branch=main")
	runGitAPI(t, localDir, "config", "user.email", "test@test.com")
	runGitAPI(t, localDir, "config", "user.name", "Test User")
	runGitAPI(t, localDir, "config", "core.hooksPath", "/dev/null")
	writeFileAPI(t, localDir, "README.md", "# Test")
	runGitAPI(t, localDir, "add", ".")
	runGitAPI(t, localDir, "commit", "-m", "Initial commit")
	runGitAPI(t, localDir, "remote", "add", "origin", remoteDir)
	runGitAPI(t, localDir, "push", "-u", "origin", "main")
	return localDir, cleanup
}

func runGitAPI(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{
		"-C", dir,
		"-c", "commit.gpgsign=false",
		"-c", "tag.gpgsign=false",
	}, args...)
	cmd := exec.Command("git", full...)
	env := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "GIT_") {
			continue
		}
		env = append(env, e)
	}
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\nOutput: %s", args, err, out)
	}
	return string(out)
}

func writeFileAPI(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write file %s: %v", name, err)
	}
}
