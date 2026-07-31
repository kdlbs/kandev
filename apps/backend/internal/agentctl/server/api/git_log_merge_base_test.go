package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
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
	t.Cleanup(cleanup)

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
	runGitAPI(t, repoDir, "fetch", "origin", "refs/heads/main:refs/remotes/origin/main")

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
	t.Cleanup(cleanup)

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
	t.Cleanup(cleanup)

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

// TestCorrectStaleBase_MergedStackedParent reproduces the reported bug: the
// session's stored base branch is a stacked-PR parent that has since merged
// into the integration branch and had its origin ref deleted, lingering only
// as a local ref. computeMergeBase falls back to that stale local parent ref
// and the log range sweeps in commits that already landed on the integration
// branch (the 31-vs-1 count).
//
// The geometry deliberately advances main ONE commit PAST the parent tip after
// the merge, so the stale local parent merge-base is a STRICT ancestor of
// merge-base(HEAD, origin/main) rather than equal to it. That exercises the
// IsAncestor-based correction: a no-op implementation (returning its input) or
// one gated only on the equality check would fail this test. The shared
// comparison policy must re-anchor to the integration merge-base.
func TestCorrectStaleBase_MergedStackedParent(t *testing.T) {
	repoDir, cleanup := setupAPITestRepo(t)
	t.Cleanup(cleanup)

	// main starts at the initial commit; branch the stacked PARENT off it and
	// add three commits.
	mainStart := strings.TrimSpace(runGitAPI(t, repoDir, "rev-parse", "HEAD"))
	runGitAPI(t, repoDir, "checkout", "-b", "feature/parent", mainStart)
	for i := 0; i < 3; i++ {
		writeFileAPI(t, repoDir, "parent.txt", strings.Repeat("parent\n", i+1))
		runGitAPI(t, repoDir, "add", ".")
		runGitAPI(t, repoDir, "commit", "-m", "feat: parent work")
	}
	parentTip := strings.TrimSpace(runGitAPI(t, repoDir, "rev-parse", "HEAD"))

	// The parent PR merges into main (ff), then main advances ONE more commit
	// beyond the parent tip and origin is pushed. feature/parent's origin ref is
	// never created (merged+deleted), so it survives only as a stale local ref.
	runGitAPI(t, repoDir, "checkout", "main")
	runGitAPI(t, repoDir, "merge", "--ff-only", "feature/parent")
	writeFileAPI(t, repoDir, "main-after.txt", "main advances past parent\n")
	runGitAPI(t, repoDir, "add", ".")
	runGitAPI(t, repoDir, "commit", "-m", "chore: main past parent")
	runGitAPI(t, repoDir, "push", "origin", "main")
	runGitAPI(t, repoDir, "fetch", "origin", "refs/heads/main:refs/remotes/origin/main")

	// The child feature branch stacks on the advanced integration line and adds
	// one commit of its own.
	runGitAPI(t, repoDir, "checkout", "-b", "feature/child", "origin/main")
	writeFileAPI(t, repoDir, "child.txt", "child work\n")
	runGitAPI(t, repoDir, "add", ".")
	runGitAPI(t, repoDir, "commit", "-m", "feat: child work")

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	srv := &Server{logger: log}
	gitOp := process.NewGitOperator(repoDir, log, nil)

	// The stored target branch is the merged/deleted parent. computeMergeBase
	// falls back to the stale local parent ref (== parentTip).
	staleBase, err := srv.computeMergeBase(context.Background(), gitOp, "feature/parent")
	if err != nil {
		t.Fatalf("computeMergeBase(feature/parent) failed: %v", err)
	}
	if staleBase != parentTip {
		t.Fatalf("expected stale base = parent tip %s, got %s", parentTip, staleBase)
	}

	integ, err := srv.computeMergeBase(context.Background(), gitOp, "main")
	if err != nil {
		t.Fatalf("computeMergeBase(main) failed: %v", err)
	}
	// Guard against a no-op test: the stale base must be STRICTLY behind the
	// integration merge-base for the IsAncestor correction to be meaningful.
	if staleBase == integ {
		t.Fatalf("test setup invalid: stale base %s equals integration merge-base %s — "+
			"correction would be a no-op", staleBase, integ)
	}

	// feature/parent has no origin ref (merged+deleted), so the shared
	// comparison policy applies and re-anchors to integ.
	corrected := gitOp.CorrectStaleComparisonBase(
		context.Background(),
		staleBase,
		"feature/parent",
	)
	if corrected != integ {
		t.Errorf("expected corrected base = integration merge-base %s, got %s", integ, corrected)
	}

	// End-to-end: the commits panel now enumerates exactly the child's commit.
	result, err := gitOp.GetLog(context.Background(), corrected, 0)
	if err != nil {
		t.Fatalf("GetLog failed: %v", err)
	}
	if len(result.Commits) != 1 {
		shas := make([]string, 0, len(result.Commits))
		for _, c := range result.Commits {
			shas = append(shas, c.CommitSHA[:7]+" "+c.CommitMessage)
		}
		t.Errorf("expected exactly 1 child commit after correction, got %d:\n%s",
			len(result.Commits), strings.Join(shas, "\n"))
	}
}

func TestGitReviewEndpointsCorrectStaleBase(t *testing.T) {
	repoDir := t.TempDir()
	integrationBase := setupStaleComparisonRepoAt(t, repoDir)

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	cfg := &config.InstanceConfig{WorkDir: repoDir}
	srv := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)

	logResponse := httptest.NewRecorder()
	srv.Router().ServeHTTP(
		logResponse,
		httptest.NewRequest(
			http.MethodGet,
			"/api/v1/git/log?limit=100&target_branch=feature%2Fparent",
			nil,
		),
	)
	if logResponse.Code != http.StatusOK {
		t.Fatalf("git log status = %d: %s", logResponse.Code, logResponse.Body.String())
	}
	var commits process.GitLogResult
	if err := json.Unmarshal(logResponse.Body.Bytes(), &commits); err != nil {
		t.Fatalf("decode git log: %v", err)
	}
	if len(commits.Commits) != 1 || commits.Commits[0].CommitMessage != "feat: child work" {
		t.Fatalf("git log did not use corrected base: %s", logResponse.Body.String())
	}

	diffResponse := httptest.NewRecorder()
	srv.Router().ServeHTTP(
		diffResponse,
		httptest.NewRequest(
			http.MethodGet,
			"/api/v1/git/cumulative-diff?base="+integrationBase+
				"&target_branch=feature%2Fparent",
			nil,
		),
	)
	if diffResponse.Code != http.StatusOK {
		t.Fatalf("cumulative diff status = %d: %s", diffResponse.Code, diffResponse.Body.String())
	}
	var diff process.CumulativeDiffResult
	if err := json.Unmarshal(diffResponse.Body.Bytes(), &diff); err != nil {
		t.Fatalf("decode cumulative diff: %v", err)
	}
	if diff.BaseCommit != integrationBase {
		t.Errorf("cumulative diff base = %q, want integration base %q", diff.BaseCommit, integrationBase)
	}
	if _, ok := diff.Files["child.txt"]; !ok {
		t.Errorf("cumulative diff missing child.txt: %s", diffResponse.Body.String())
	}
	if _, ok := diff.Files["parent.txt"]; ok {
		t.Errorf("cumulative diff includes parent work from stale range: %s", diffResponse.Body.String())
	}
}

// TestCorrectStaleBase_CurrentBaseUnchanged verifies the correction does not
// over-reach: when the resolved base already equals (or descends from) the
// integration merge-base, the shared comparison policy returns it unchanged.
func TestCorrectStaleBase_CurrentBaseUnchanged(t *testing.T) {
	repoDir, cleanup := setupAPITestRepo(t)
	t.Cleanup(cleanup)

	// Feature branches straight off current main; its merge-base against main
	// is main's tip, which is already current.
	runGitAPI(t, repoDir, "checkout", "-b", "feature/x")
	writeFileAPI(t, repoDir, "feature.txt", "feature\n")
	runGitAPI(t, repoDir, "add", ".")
	runGitAPI(t, repoDir, "commit", "-m", "feat: work")

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	srv := &Server{logger: log}
	gitOp := process.NewGitOperator(repoDir, log, nil)

	base, err := srv.computeMergeBase(context.Background(), gitOp, "main")
	if err != nil {
		t.Fatalf("computeMergeBase(main) failed: %v", err)
	}
	// A non-integration target with no live upstream clears the first guard, so
	// the integration-merge-base equality branch is the one under test: because
	// feature/x branches straight off current main, so the integration base
	// equals base and exercises the unchanged path.
	corrected := gitOp.CorrectStaleComparisonBase(context.Background(), base, "feature/gone")
	if corrected != base {
		t.Errorf("expected unchanged base %s, got %s", base, corrected)
	}
}

// TestCorrectStaleBase_LiveUpstreamTargetPreserved verifies the Codex P1 guard:
// when the target branch is a LIVE non-default upstream (e.g. origin/develop)
// whose own merge-base is a strict ancestor of the main merge-base, the base is
// NOT overridden. Overriding would hide commits that are genuinely part of the
// develop comparison. The distinguishing signal is that the target still has a
// live origin/<name> ref, so targetHasUpstream is true and the correction is
// skipped even though the ancestry relationship holds.
func TestCorrectStaleBase_LiveUpstreamTargetPreserved(t *testing.T) {
	repoDir, cleanup := setupAPITestRepo(t)
	t.Cleanup(cleanup)

	// develop diverges early from main0 and is pushed (live upstream).
	runGitAPI(t, repoDir, "checkout", "-b", "develop")
	runGitAPI(t, repoDir, "push", "-u", "origin", "develop")
	runGitAPI(t, repoDir, "fetch", "origin", "refs/heads/develop:refs/remotes/origin/develop")
	developBase := strings.TrimSpace(runGitAPI(t, repoDir, "rev-parse", "HEAD"))

	// main advances past the develop branch point.
	runGitAPI(t, repoDir, "checkout", "main")
	writeFileAPI(t, repoDir, "main1.txt", "main advances\n")
	runGitAPI(t, repoDir, "add", ".")
	runGitAPI(t, repoDir, "commit", "-m", "chore: main forward")
	runGitAPI(t, repoDir, "push", "origin", "main")
	runGitAPI(t, repoDir, "fetch", "origin", "refs/heads/main:refs/remotes/origin/main")

	// feature branches off the advanced main but its stored target is develop,
	// so merge-base(HEAD, origin/develop) is strictly behind
	// merge-base(HEAD, origin/main) — the exact shape that would mis-fire.
	runGitAPI(t, repoDir, "checkout", "-b", "feature/y", "main")
	writeFileAPI(t, repoDir, "y.txt", "y\n")
	runGitAPI(t, repoDir, "add", ".")
	runGitAPI(t, repoDir, "commit", "-m", "feat: y")

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	srv := &Server{logger: log}
	gitOp := process.NewGitOperator(repoDir, log, nil)

	base, err := srv.computeMergeBase(context.Background(), gitOp, "develop")
	if err != nil {
		t.Fatalf("computeMergeBase(develop) failed: %v", err)
	}
	if base != developBase {
		t.Fatalf("expected develop merge-base %s, got %s", developBase, base)
	}
	// Sanity: the ancestry relationship that would trigger a naive re-anchor
	// does hold, so the guard is what prevents the regression.
	integ, err := srv.computeMergeBase(context.Background(), gitOp, "main")
	if err != nil {
		t.Fatalf("computeMergeBase(main) failed: %v", err)
	}
	if base == integ {
		t.Fatalf("test setup invalid: develop merge-base equals main merge-base %s", integ)
	}

	corrected := gitOp.CorrectStaleComparisonBase(context.Background(), base, "develop")
	if corrected != base {
		t.Errorf("expected live-upstream develop base %s preserved, got %s (integ=%s)",
			base, corrected, integ)
	}
}

// TestCorrectStaleBase_NoIntegrationRefFallsBack verifies that when no
// integration ref resolves (neither origin/main, origin/master, nor a local
// main/master) the base is returned unchanged and no error is raised. The
// origin/main tracking ref created by setupAPITestRepo is explicitly deleted so
// the no-integration fallback is genuinely exercised.
func TestCorrectStaleBase_NoIntegrationRefFallsBack(t *testing.T) {
	repoDir, cleanup := setupAPITestRepo(t)
	t.Cleanup(cleanup)

	// Rename the only integration branch away from main/master and drop the
	// origin/main tracking ref, so no integration candidate resolves locally or
	// on origin.
	runGitAPI(t, repoDir, "checkout", "-b", "feature/x")
	runGitAPI(t, repoDir, "branch", "-m", "main", "trunk")
	runGitAPI(t, repoDir, "update-ref", "-d", "refs/remotes/origin/main")
	writeFileAPI(t, repoDir, "feature.txt", "feature\n")
	runGitAPI(t, repoDir, "add", ".")
	runGitAPI(t, repoDir, "commit", "-m", "feat: work")

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	gitOp := process.NewGitOperator(repoDir, log, nil)

	base := strings.TrimSpace(runGitAPI(t, repoDir, "rev-parse", "trunk"))
	corrected := gitOp.CorrectStaleComparisonBase(context.Background(), base, "trunk")
	if corrected != base {
		t.Errorf("expected unchanged base %s when no integration ref, got %s", base, corrected)
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

func setupStaleComparisonRepoAt(t *testing.T, repoDir string) string {
	t.Helper()
	remoteDir := t.TempDir()
	runGitAPI(t, remoteDir, "init", "--bare", "--initial-branch=main")
	runGitAPI(t, repoDir, "init", "--initial-branch=main")
	runGitAPI(t, repoDir, "config", "user.email", "test@test.com")
	runGitAPI(t, repoDir, "config", "user.name", "Test User")
	runGitAPI(t, repoDir, "config", "core.hooksPath", "/dev/null")
	writeFileAPI(t, repoDir, "README.md", "# Test")
	runGitAPI(t, repoDir, "add", ".")
	runGitAPI(t, repoDir, "commit", "-m", "initial")
	runGitAPI(t, repoDir, "remote", "add", "origin", remoteDir)
	runGitAPI(t, repoDir, "push", "-u", "origin", "main")

	runGitAPI(t, repoDir, "checkout", "-b", "feature/parent")
	writeFileAPI(t, repoDir, "parent.txt", "parent work\n")
	runGitAPI(t, repoDir, "add", ".")
	runGitAPI(t, repoDir, "commit", "-m", "feat: parent work")

	runGitAPI(t, repoDir, "checkout", "main")
	runGitAPI(t, repoDir, "merge", "--ff-only", "feature/parent")
	writeFileAPI(t, repoDir, "main-after.txt", "main advances past parent\n")
	runGitAPI(t, repoDir, "add", ".")
	runGitAPI(t, repoDir, "commit", "-m", "chore: main past parent")
	runGitAPI(t, repoDir, "push", "origin", "main")
	integrationBase := strings.TrimSpace(runGitAPI(t, repoDir, "rev-parse", "origin/main"))

	runGitAPI(t, repoDir, "checkout", "-b", "feature/child", "origin/main")
	writeFileAPI(t, repoDir, "child.txt", "child work\n")
	runGitAPI(t, repoDir, "add", ".")
	runGitAPI(t, repoDir, "commit", "-m", "feat: child work")
	return integrationBase
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
