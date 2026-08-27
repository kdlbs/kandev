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

// postGitJSON drives a git endpoint through the real router and decodes the
// handler's result. Anything other than 200 is fatal: these handlers answer
// operational failures with 200 + Success:false, so a non-200 means the request
// never reached the GitOperator at all.
func postGitJSON(t *testing.T, srv *Server, path, body string) process.GitOperationResult {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST %s status = %d, want 200: %s", path, rec.Code, rec.Body.String())
	}
	var result process.GitOperationResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode %s response: %v (body %s)", path, err, rec.Body.String())
	}
	return result
}

func postGitPull(t *testing.T, srv *Server, body string) process.GitOperationResult {
	t.Helper()
	return postGitJSON(t, srv, "/api/v1/git/pull", body)
}

// newPullAPIServer wires the api Server over an existing working copy.
func newPullAPIServer(t *testing.T, repoDir string) *Server {
	t.Helper()
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	cfg := &config.InstanceConfig{WorkDir: repoDir}
	mgr := process.NewManager(cfg, log)
	t.Cleanup(func() { _ = mgr.StopForTeardown(context.Background()) })
	return NewServer(cfg, mgr, nil, nil, log)
}

// advanceOriginMain pushes one commit to the bare origin behind repoDir without
// touching repoDir's working copy, so the next pull has something to integrate.
// Returns the SHA of the new origin/main tip.
func advanceOriginMain(t *testing.T, repoDir, file, content, message string) string {
	t.Helper()
	remoteURL := strings.TrimSpace(runGitAPI(t, repoDir, "remote", "get-url", "origin"))
	otherDir := t.TempDir()
	runGitAPI(t, otherDir, "clone", remoteURL, ".")
	runGitAPI(t, otherDir, "config", "user.email", "other@test.com")
	runGitAPI(t, otherDir, "config", "user.name", "Other User")
	runGitAPI(t, otherDir, "config", "core.hooksPath", "/dev/null")
	writeFileAPI(t, otherDir, file, content)
	runGitAPI(t, otherDir, "add", ".")
	runGitAPI(t, otherDir, "commit", "-m", message)
	runGitAPI(t, otherDir, "push", "origin", "main")
	return strings.TrimSpace(runGitAPI(t, otherDir, "rev-parse", "HEAD"))
}

// commitLocally adds one commit on the current branch of repoDir.
func commitLocally(t *testing.T, repoDir, file, content, message string) {
	t.Helper()
	writeFileAPI(t, repoDir, file, content)
	runGitAPI(t, repoDir, "add", ".")
	runGitAPI(t, repoDir, "commit", "-m", message)
}

// TestHandleGitPull_RebaseReplaysLocalCommitOntoOrigin drives the documented
// {"rebase": true} option end to end against a real bare origin. Local main and
// origin/main have each moved on by one commit, so a successful pull must
// produce a linear history with the local commit replayed on top of the remote
// tip — a merge commit would mean the rebase flag was ignored.
//
// The flag allowlist in securityutil.IsKnownSafeGitFlag used to omit --rebase,
// so this path returned 200 with {"success":false,"error":"potentially unsafe
// flag: --rebase"} for every caller.
func TestHandleGitPull_RebaseReplaysLocalCommitOntoOrigin(t *testing.T) {
	repoDir, cleanup := setupAPITestRepo(t)
	t.Cleanup(cleanup)

	remoteTip := advanceOriginMain(t, repoDir, "remote.txt", "remote work\n", "feat: remote work")
	commitLocally(t, repoDir, "local.txt", "local work\n", "feat: local work")
	localMessage := strings.TrimSpace(runGitAPI(t, repoDir, "log", "-1", "--format=%s"))

	srv := newPullAPIServer(t, repoDir)
	result := postGitPull(t, srv, `{"rebase": true}`)

	if !result.Success {
		t.Fatalf("pull --rebase failed: error=%q output=%q conflicts=%v",
			result.Error, result.Output, result.ConflictFiles)
	}
	if result.Operation != "pull" {
		t.Errorf("result.Operation = %q, want %q", result.Operation, "pull")
	}

	// Rebase, not merge: the local commit is still the tip, its parent is the
	// remote tip, and the branch carries no merge commit.
	if got := strings.TrimSpace(runGitAPI(t, repoDir, "log", "-1", "--format=%s")); got != localMessage {
		t.Errorf("HEAD subject = %q, want the replayed local commit %q", got, localMessage)
	}
	if got := strings.TrimSpace(runGitAPI(t, repoDir, "rev-parse", "HEAD^")); got != remoteTip {
		t.Errorf("HEAD^ = %q, want the origin/main tip %q — the local commit was not replayed on top", got, remoteTip)
	}
	if merges := strings.TrimSpace(runGitAPI(t, repoDir, "rev-list", "--merges", "HEAD")); merges != "" {
		t.Errorf("branch contains merge commits %q — the pull merged instead of rebasing", merges)
	}
	// Both files present means neither side's work was dropped.
	for _, name := range []string{"remote.txt", "local.txt"} {
		if _, err := os.Stat(filepath.Join(repoDir, name)); err != nil {
			t.Errorf("expected %s in the working tree after the rebase: %v", name, err)
		}
	}
}

// TestHandleGitPull_RebaseConflictAutoAborts covers Pull's conflict-recovery
// path, which runs "rebase --abort" so the caller is not left in a detached
// mid-rebase state. --abort was missing from the same allowlist, so the abort
// itself failed and the repository stayed stuck.
func TestHandleGitPull_RebaseConflictAutoAborts(t *testing.T) {
	repoDir, cleanup := setupAPITestRepo(t)
	t.Cleanup(cleanup)

	// Both sides edit the same file so the replay conflicts.
	advanceOriginMain(t, repoDir, "shared.txt", "remote version\n", "feat: remote edit")
	commitLocally(t, repoDir, "shared.txt", "local version\n", "feat: local edit")
	localTip := strings.TrimSpace(runGitAPI(t, repoDir, "rev-parse", "HEAD"))

	srv := newPullAPIServer(t, repoDir)
	result := postGitPull(t, srv, `{"rebase": true}`)

	if result.Success {
		t.Fatalf("pull --rebase reported success despite a conflicting change: %+v", result)
	}
	if len(result.ConflictFiles) == 0 {
		t.Fatalf("expected the conflicting file to be reported, got none (error=%q output=%q)",
			result.Error, result.Output)
	}
	if result.ConflictFiles[0] != "shared.txt" {
		t.Errorf("ConflictFiles = %v, want [shared.txt]", result.ConflictFiles)
	}

	// The abort must have run: no in-progress rebase state, and HEAD is back
	// on the local commit with a clean tree.
	for _, dir := range []string{"rebase-merge", "rebase-apply"} {
		if _, err := os.Stat(filepath.Join(repoDir, ".git", dir)); err == nil {
			t.Errorf(".git/%s still exists — the conflicting rebase was not aborted", dir)
		}
	}
	if got := strings.TrimSpace(runGitAPI(t, repoDir, "rev-parse", "HEAD")); got != localTip {
		t.Errorf("HEAD = %q after the aborted rebase, want the pre-pull local tip %q", got, localTip)
	}
	if status := strings.TrimSpace(runGitAPI(t, repoDir, "status", "--porcelain")); status != "" {
		t.Errorf("working tree is dirty after the abort:\n%s", status)
	}
}

// runGitAPIExpectFailure runs git and requires a non-zero exit. Leaving a
// repository mid-rebase or mid-merge means running a command that conflicts,
// so the failure is the point — runGitAPI would call t.Fatalf on it.
func runGitAPIExpectFailure(t *testing.T, dir string, args ...string) {
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
	if err == nil {
		t.Fatalf("git %v was expected to conflict but succeeded:\n%s", args, out)
	}
}

// setupConflictingBranches leaves repoDir checked out on feature/x, whose only
// commit adds shared.txt with content that conflicts with main's. Returns the
// feature tip, which an abort must restore.
func setupConflictingBranches(t *testing.T, repoDir string) string {
	t.Helper()
	runGitAPI(t, repoDir, "checkout", "-b", "feature/x")
	commitLocally(t, repoDir, "shared.txt", "feature version\n", "feat: feature edit")
	featureTip := strings.TrimSpace(runGitAPI(t, repoDir, "rev-parse", "HEAD"))

	runGitAPI(t, repoDir, "checkout", "main")
	commitLocally(t, repoDir, "shared.txt", "main version\n", "feat: main edit")

	runGitAPI(t, repoDir, "checkout", "feature/x")
	return featureTip
}

// assertAbortRestoredRepo pins the post-abort state: no in-progress operation
// left behind, HEAD back where it started, and a clean working tree.
func assertAbortRestoredRepo(t *testing.T, repoDir, wantHead string) {
	t.Helper()
	for _, name := range []string{"rebase-merge", "rebase-apply", "MERGE_HEAD"} {
		if _, err := os.Stat(filepath.Join(repoDir, ".git", name)); err == nil {
			t.Errorf(".git/%s still exists — the operation was not aborted", name)
		}
	}
	if got := strings.TrimSpace(runGitAPI(t, repoDir, "rev-parse", "HEAD")); got != wantHead {
		t.Errorf("HEAD = %q after the abort, want %q", got, wantHead)
	}
	if status := strings.TrimSpace(runGitAPI(t, repoDir, "status", "--porcelain")); status != "" {
		t.Errorf("working tree is dirty after the abort:\n%s", status)
	}
}

// TestHandleGitAbort_ClearsInProgressRebase drives POST /api/v1/git/abort with
// {"operation": "rebase"} against a repository genuinely stuck mid-rebase.
// GitOperator.Abort issues "rebase --abort", which the flag allowlist rejected,
// so this endpoint failed for every caller regardless of repository state.
func TestHandleGitAbort_ClearsInProgressRebase(t *testing.T) {
	repoDir, cleanup := setupAPITestRepo(t)
	t.Cleanup(cleanup)

	featureTip := setupConflictingBranches(t, repoDir)
	runGitAPIExpectFailure(t, repoDir, "rebase", "main")
	if _, err := os.Stat(filepath.Join(repoDir, ".git", "rebase-merge")); err != nil {
		t.Fatalf("fixture did not leave a rebase in progress: %v", err)
	}

	srv := newPullAPIServer(t, repoDir)
	result := postGitJSON(t, srv, "/api/v1/git/abort", `{"operation": "rebase"}`)

	if !result.Success {
		t.Fatalf("abort rebase failed: error=%q output=%q", result.Error, result.Output)
	}
	if result.Operation != "abort" {
		t.Errorf("result.Operation = %q, want %q", result.Operation, "abort")
	}
	assertAbortRestoredRepo(t, repoDir, featureTip)
}

// TestHandleGitAbort_ClearsInProgressMerge is the "merge" half of the same
// endpoint; it issues "merge --abort" through the same allowlist.
func TestHandleGitAbort_ClearsInProgressMerge(t *testing.T) {
	repoDir, cleanup := setupAPITestRepo(t)
	t.Cleanup(cleanup)

	featureTip := setupConflictingBranches(t, repoDir)
	runGitAPIExpectFailure(t, repoDir, "merge", "main")
	if _, err := os.Stat(filepath.Join(repoDir, ".git", "MERGE_HEAD")); err != nil {
		t.Fatalf("fixture did not leave a merge in progress: %v", err)
	}

	srv := newPullAPIServer(t, repoDir)
	result := postGitJSON(t, srv, "/api/v1/git/abort", `{"operation": "merge"}`)

	if !result.Success {
		t.Fatalf("abort merge failed: error=%q output=%q", result.Error, result.Output)
	}
	assertAbortRestoredRepo(t, repoDir, featureTip)
}
