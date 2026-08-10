package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

// postGitPull drives POST /api/v1/git/pull through the real router and decodes
// the handler's result. Anything other than 200 is fatal: the handler answers
// operational failures with 200 + Success:false, so a non-200 means the request
// never reached GitOperator.Pull.
func postGitPull(t *testing.T, srv *Server, body string) process.GitOperationResult {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/git/pull", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/git/pull status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var result process.GitOperationResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode pull response: %v (body %s)", err, rec.Body.String())
	}
	return result
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
