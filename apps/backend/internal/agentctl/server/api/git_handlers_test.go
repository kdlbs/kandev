package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

// gitAPIFixture is a single-repo workspace wired to a bare `origin` remote.
// The remote matters: it gives `origin/main` a real ref so merge-base anchoring
// in handleGitLog / handleGitCumulativeDiff behaves the way it does in
// production rather than falling through the no-upstream recovery paths.
type gitAPIFixture struct {
	server *Server
	repo   string
	bare   string
}

// newGitAPIFixture seeds main (one commit, pushed to origin) plus a
// feature/work branch carrying two more commits, and returns a Server whose
// workspace root is that repository.
func newGitAPIFixture(t *testing.T) *gitAPIFixture {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "origin.git")
	runGitAPI(t, root, "init", "--bare", "--initial-branch=main", bare)

	repo := filepath.Join(root, "work")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}
	runGitAPI(t, repo, "init", "--initial-branch=main")
	runGitAPI(t, repo, "config", "user.email", "test@test.com")
	runGitAPI(t, repo, "config", "user.name", "Test User")
	writeFileAPI(t, repo, "README.md", "base\n")
	runGitAPI(t, repo, "add", ".")
	runGitAPI(t, repo, "commit", "-m", "initial")
	runGitAPI(t, repo, "remote", "add", "origin", bare)
	runGitAPI(t, repo, "push", "-u", "origin", "main")

	runGitAPI(t, repo, "checkout", "-b", "feature/work")
	writeFileAPI(t, repo, "one.txt", "one\n")
	runGitAPI(t, repo, "add", ".")
	runGitAPI(t, repo, "commit", "-m", "feature one")
	writeFileAPI(t, repo, "two.txt", "two\n")
	runGitAPI(t, repo, "add", ".")
	runGitAPI(t, repo, "commit", "-m", "feature two")

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	cfg := &config.InstanceConfig{WorkDir: repo}
	return &gitAPIFixture{
		server: NewServer(cfg, process.NewManager(cfg, log), nil, nil, log),
		repo:   repo,
		bare:   bare,
	}
}

func (f *gitAPIFixture) head(t *testing.T) string {
	t.Helper()
	return strings.TrimSpace(runGitAPI(t, f.repo, "rev-parse", "HEAD"))
}

func (f *gitAPIFixture) branch(t *testing.T) string {
	t.Helper()
	return strings.TrimSpace(runGitAPI(t, f.repo, "rev-parse", "--abbrev-ref", "HEAD"))
}

// postGitAPI issues a POST with a JSON body (or a raw string body verbatim,
// which is how the malformed-payload cases are expressed).
func postGitAPI(t *testing.T, srv *Server, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	switch v := body.(type) {
	case string:
		payload = []byte(v)
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		payload = encoded
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	return rec
}

func getGitAPI(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func decodeGitOperationResult(t *testing.T, rec *httptest.ResponseRecorder) process.GitOperationResult {
	t.Helper()
	var result process.GitOperationResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode git operation result from %q: %v", rec.Body.String(), err)
	}
	return result
}

// TestGitHandlers_RejectMalformedBody pins the shape of the 400 every git POST
// handler writes for an unparseable body: the operation label the caller sent
// the request for, and an "invalid request: " prefixed error. The operation
// label is what the frontend keys its error toast off, so a copy-paste slip
// between handlers is a user-visible bug that no status-code-only test catches.
func TestGitHandlers_RejectMalformedBody(t *testing.T) {
	fixture := newGitAPIFixture(t)

	cases := []struct {
		path      string
		operation string
	}{
		{"/api/v1/git/pull", "pull"},
		{"/api/v1/git/push", "push"},
		{"/api/v1/git/push-preflight", "push_preflight"},
		{"/api/v1/git/rebase", "rebase"},
		{"/api/v1/git/merge", "merge"},
		{"/api/v1/git/abort", "abort"},
		{"/api/v1/git/commit", "commit"},
		{"/api/v1/git/rename-branch", "rename_branch"},
		{"/api/v1/git/stage", "stage"},
		{"/api/v1/git/unstage", "unstage"},
		{"/api/v1/git/discard", "discard"},
		{"/api/v1/git/revert-commit", "revert_commit"},
		{"/api/v1/git/reset", "reset"},
	}
	for _, tc := range cases {
		t.Run(tc.operation, func(t *testing.T) {
			rec := postGitAPI(t, fixture.server, tc.path, "{not json")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			result := decodeGitOperationResult(t, rec)
			if result.Success {
				t.Errorf("success = true, want false")
			}
			if result.Operation != tc.operation {
				t.Errorf("operation = %q, want %q", result.Operation, tc.operation)
			}
			if !strings.HasPrefix(result.Error, "invalid request: ") {
				t.Errorf("error = %q, want an %q prefix", result.Error, "invalid request: ")
			}
		})
	}
}

// TestGitHandlers_RejectMissingRequiredFields pins each handler's own
// pre-flight validation message. These are the only guards standing between an
// empty form field and a git invocation with a missing argument.
func TestGitHandlers_RejectMissingRequiredFields(t *testing.T) {
	fixture := newGitAPIFixture(t)

	cases := []struct {
		name      string
		path      string
		body      any
		operation string
		wantError string
	}{
		{"rebase without base", "/api/v1/git/rebase", GitRebaseRequest{}, "rebase", "base_branch is required"},
		{"merge without base", "/api/v1/git/merge", GitMergeRequest{}, "merge", "base_branch is required"},
		{
			"abort with unknown operation",
			"/api/v1/git/abort",
			GitAbortRequest{Operation: "cherry-pick"},
			"abort",
			"operation must be 'merge' or 'rebase'",
		},
		{"commit without message", "/api/v1/git/commit", GitCommitRequest{}, "commit", "message is required"},
		{
			"rename-branch without name",
			"/api/v1/git/rename-branch",
			GitRenameBranchRequest{},
			"rename_branch",
			"new_name is required",
		},
		{"discard without paths", "/api/v1/git/discard", GitDiscardRequest{}, "discard", "paths are required"},
		{
			"revert-commit without sha",
			"/api/v1/git/revert-commit",
			GitRevertCommitRequest{},
			"revert_commit",
			"commit_sha is required",
		},
		{"reset without sha", "/api/v1/git/reset", GitResetRequest{}, "reset", "commit_sha is required"},
		{
			"reset with unknown mode",
			"/api/v1/git/reset",
			GitResetRequest{CommitSHA: "HEAD", Mode: "squash"},
			"reset",
			"invalid reset mode: squash (must be soft, mixed, or hard)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := postGitAPI(t, fixture.server, tc.path, tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			result := decodeGitOperationResult(t, rec)
			if result.Success {
				t.Errorf("success = true, want false")
			}
			if result.Operation != tc.operation {
				t.Errorf("operation = %q, want %q", result.Operation, tc.operation)
			}
			if result.Error != tc.wantError {
				t.Errorf("error = %q, want %q", result.Error, tc.wantError)
			}
		})
	}
}

// TestGitOpForRepo_RejectsUnresolvableSubpath covers the shared repo-scope
// barrier. A bad `repo` must be refused before any git command runs, and the
// refusal has to carry the operation the caller asked for so the failure is
// attributable in a multi-repo UI.
func TestGitOpForRepo_RejectsUnresolvableSubpath(t *testing.T) {
	fixture := newGitAPIFixture(t)

	cases := []struct {
		name      string
		path      string
		body      any
		operation string
		wantError string
	}{
		{"missing subpath", "/api/v1/git/pull", GitPullRequest{Repo: "nope"}, "pull", "repo subpath not found"},
		{
			"traversal",
			"/api/v1/git/commit",
			GitCommitRequest{Message: "m", Repo: "../escape"},
			"commit",
			"invalid repo subpath",
		},
		{
			"absolute",
			"/api/v1/git/stage",
			GitStageRequest{Repo: filepath.Join(t.TempDir(), "abs")},
			"stage",
			"repo subpath must be relative",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := postGitAPI(t, fixture.server, tc.path, tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			result := decodeGitOperationResult(t, rec)
			if result.Operation != tc.operation {
				t.Errorf("operation = %q, want %q", result.Operation, tc.operation)
			}
			if !strings.Contains(result.Error, tc.wantError) {
				t.Errorf("error = %q, want it to contain %q", result.Error, tc.wantError)
			}
		})
	}
}

// TestHandleGitCommit_CreatesCommitInWorktree asserts the endpoint's real
// effect rather than only its response: the working-tree change is staged and
// committed with exactly the requested message.
func TestHandleGitCommit_CreatesCommitInWorktree(t *testing.T) {
	fixture := newGitAPIFixture(t)
	writeFileAPI(t, fixture.repo, "three.txt", "three\n")

	rec := postGitAPI(t, fixture.server, "/api/v1/git/commit", GitCommitRequest{
		Message:  "handler commit",
		StageAll: true,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	result := decodeGitOperationResult(t, rec)
	if !result.Success {
		t.Fatalf("commit failed: %+v", result)
	}
	if got := strings.TrimSpace(runGitAPI(t, fixture.repo, "log", "-1", "--format=%s")); got != "handler commit" {
		t.Errorf("HEAD subject = %q, want %q", got, "handler commit")
	}
	if got := strings.TrimSpace(runGitAPI(t, fixture.repo, "status", "--porcelain")); got != "" {
		t.Errorf("worktree not clean after stage_all commit: %q", got)
	}
}

// TestHandleGitStageAndUnstage checks both directions of the index against
// `git status --porcelain`, which is the only place the difference between
// staged and unstaged is observable.
func TestHandleGitStageAndUnstage(t *testing.T) {
	fixture := newGitAPIFixture(t)
	writeFileAPI(t, fixture.repo, "staged.txt", "staged\n")

	stage := postGitAPI(t, fixture.server, "/api/v1/git/stage", GitStageRequest{Paths: []string{"staged.txt"}})
	if stage.Code != http.StatusOK {
		t.Fatalf("stage status = %d (body %s)", stage.Code, stage.Body.String())
	}
	if result := decodeGitOperationResult(t, stage); !result.Success {
		t.Fatalf("stage failed: %+v", result)
	}
	if got := strings.TrimSpace(runGitAPI(t, fixture.repo, "status", "--porcelain", "staged.txt")); got != "A  staged.txt" {
		t.Fatalf("porcelain after stage = %q, want %q", got, "A  staged.txt")
	}

	unstage := postGitAPI(t, fixture.server, "/api/v1/git/unstage", GitUnstageRequest{Paths: []string{"staged.txt"}})
	if unstage.Code != http.StatusOK {
		t.Fatalf("unstage status = %d (body %s)", unstage.Code, unstage.Body.String())
	}
	if result := decodeGitOperationResult(t, unstage); !result.Success {
		t.Fatalf("unstage failed: %+v", result)
	}
	if got := strings.TrimSpace(runGitAPI(t, fixture.repo, "status", "--porcelain", "staged.txt")); got != "?? staged.txt" {
		t.Errorf("porcelain after unstage = %q, want %q", got, "?? staged.txt")
	}
}

// TestHandleGitDiscard_RestoresTrackedFile asserts the destructive path
// actually restores the committed bytes.
func TestHandleGitDiscard_RestoresTrackedFile(t *testing.T) {
	fixture := newGitAPIFixture(t)
	writeFileAPI(t, fixture.repo, "one.txt", "locally edited\n")

	rec := postGitAPI(t, fixture.server, "/api/v1/git/discard", GitDiscardRequest{Paths: []string{"one.txt"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	if result := decodeGitOperationResult(t, rec); !result.Success {
		t.Fatalf("discard failed: %+v", result)
	}
	content, err := os.ReadFile(filepath.Join(fixture.repo, "one.txt"))
	if err != nil {
		t.Fatalf("read one.txt: %v", err)
	}
	if string(content) != "one\n" {
		t.Errorf("one.txt = %q, want the committed %q", string(content), "one\n")
	}
}

// TestHandleGitRenameBranch_MovesCurrentBranch verifies the rename lands on the
// checked-out branch rather than merely returning success.
func TestHandleGitRenameBranch_MovesCurrentBranch(t *testing.T) {
	fixture := newGitAPIFixture(t)

	rec := postGitAPI(t, fixture.server, "/api/v1/git/rename-branch", GitRenameBranchRequest{NewName: "feature/renamed"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	if result := decodeGitOperationResult(t, rec); !result.Success {
		t.Fatalf("rename failed: %+v", result)
	}
	if got := fixture.branch(t); got != "feature/renamed" {
		t.Errorf("current branch = %q, want %q", got, "feature/renamed")
	}
}

// TestHandleGitReset_HardMovesHead asserts the mode is forwarded: a hard reset
// has to move HEAD *and* drop the working-tree file, which a soft reset would
// not do.
func TestHandleGitReset_HardMovesHead(t *testing.T) {
	fixture := newGitAPIFixture(t)
	target := strings.TrimSpace(runGitAPI(t, fixture.repo, "rev-parse", "HEAD~1"))

	rec := postGitAPI(t, fixture.server, "/api/v1/git/reset", GitResetRequest{CommitSHA: target, Mode: "hard"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	if result := decodeGitOperationResult(t, rec); !result.Success {
		t.Fatalf("reset failed: %+v", result)
	}
	if got := fixture.head(t); got != target {
		t.Errorf("HEAD = %q, want %q", got, target)
	}
	if _, err := os.Stat(filepath.Join(fixture.repo, "two.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("two.txt still present after a hard reset (stat err = %v)", err)
	}
}

// TestHandleGitRevertCommit_UncommitsHeadKeepingWork pins what revert_commit
// actually is: a soft un-commit of HEAD, which rewinds history one commit while
// leaving the files staged. Losing the staged work — or rewinding further than
// one commit — would be silent data loss for the user who clicked "undo commit".
func TestHandleGitRevertCommit_UncommitsHeadKeepingWork(t *testing.T) {
	fixture := newGitAPIFixture(t)
	target := fixture.head(t)
	parent := strings.TrimSpace(runGitAPI(t, fixture.repo, "rev-parse", "HEAD~1"))

	rec := postGitAPI(t, fixture.server, "/api/v1/git/revert-commit", GitRevertCommitRequest{CommitSHA: target})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	if result := decodeGitOperationResult(t, rec); !result.Success {
		t.Fatalf("revert failed: %+v", result)
	}
	if got := fixture.head(t); got != parent {
		t.Errorf("HEAD = %q, want the parent commit %q", got, parent)
	}
	if got := strings.TrimSpace(runGitAPI(t, fixture.repo, "status", "--porcelain", "two.txt")); got != "A  two.txt" {
		t.Errorf("porcelain for two.txt = %q, want it staged as %q", got, "A  two.txt")
	}
}

// TestHandleGitRevertCommit_RefusesOlderCommit covers the guard that keeps the
// soft un-commit from rewinding past HEAD, which would drop the commits in
// between without the caller asking for it.
func TestHandleGitRevertCommit_RefusesOlderCommit(t *testing.T) {
	fixture := newGitAPIFixture(t)
	head := fixture.head(t)
	older := strings.TrimSpace(runGitAPI(t, fixture.repo, "rev-parse", "HEAD~1"))

	rec := postGitAPI(t, fixture.server, "/api/v1/git/revert-commit", GitRevertCommitRequest{CommitSHA: older})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	result := decodeGitOperationResult(t, rec)
	if result.Success {
		t.Errorf("success = true; only the latest commit may be reverted")
	}
	if result.Error != "can only revert the latest commit" {
		t.Errorf("error = %q, want %q", result.Error, "can only revert the latest commit")
	}
	if got := fixture.head(t); got != head {
		t.Errorf("HEAD moved to %q despite the refusal; want it left at %q", got, head)
	}
}

// TestHandleGitShowCommit_ReturnsRequestedCommit pins the URI binding: the sha
// travels through the path parameter, and the response must be about that
// commit specifically.
func TestHandleGitShowCommit_ReturnsRequestedCommit(t *testing.T) {
	fixture := newGitAPIFixture(t)
	sha := fixture.head(t)

	rec := getGitAPI(t, fixture.server, "/api/v1/git/commit/"+sha)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	var result process.CommitDiffResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode commit diff: %v", err)
	}
	if !result.Success {
		t.Fatalf("show commit failed: %+v", result)
	}
	if result.CommitSHA != sha {
		t.Errorf("commit_sha = %q, want %q", result.CommitSHA, sha)
	}
	if _, ok := result.Files["two.txt"]; !ok {
		t.Errorf("files = %v, want it to contain two.txt", mapKeys(result.Files))
	}
}

// TestHandleGitShowCommit_UnknownCommitReportsInBody covers the failure branch.
// A missing object is a per-commit result failure, not a transport error, so it
// answers 200 with success=false — the frontend renders the message inline
// rather than treating the request as broken.
func TestHandleGitShowCommit_UnknownCommitReportsInBody(t *testing.T) {
	fixture := newGitAPIFixture(t)
	const missing = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

	rec := getGitAPI(t, fixture.server, "/api/v1/git/commit/"+missing)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var result process.CommitDiffResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode commit diff: %v", err)
	}
	if result.Success {
		t.Errorf("success = true, want false")
	}
	if result.CommitSHA != missing {
		t.Errorf("commit_sha = %q, want the requested sha echoed back", result.CommitSHA)
	}
	if !strings.Contains(result.Error, "bad object") {
		t.Errorf("error = %q, want it to name the bad object", result.Error)
	}
}

// TestHandleGitShowCommit_RejectsUnresolvableRepo covers the `?repo=` branch of
// the same handler, which has its own CommitDiffResult error shape.
func TestHandleGitShowCommit_RejectsUnresolvableRepo(t *testing.T) {
	fixture := newGitAPIFixture(t)
	sha := fixture.head(t)

	rec := getGitAPI(t, fixture.server, "/api/v1/git/commit/"+sha+"?repo=nope")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
	}
	var result process.CommitDiffResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode commit diff: %v", err)
	}
	if !strings.Contains(result.Error, "repo subpath not found") {
		t.Errorf("error = %q, want a repo-subpath failure", result.Error)
	}
}

// TestHandleGitPushAndPull_RoundTripThroughOrigin drives both remote-facing
// handlers against a real bare repository so the assertions are about the
// remote's state, not just a success flag.
func TestHandleGitPushAndPull_RoundTripThroughOrigin(t *testing.T) {
	fixture := newGitAPIFixture(t)
	head := fixture.head(t)

	push := postGitAPI(t, fixture.server, "/api/v1/git/push", GitPushRequest{SetUpstream: true})
	if push.Code != http.StatusOK {
		t.Fatalf("push status = %d (body %s)", push.Code, push.Body.String())
	}
	if result := decodeGitOperationResult(t, push); !result.Success {
		t.Fatalf("push failed: %+v", result)
	}
	remoteHead := strings.TrimSpace(runGitAPI(t, fixture.bare, "rev-parse", "refs/heads/feature/work"))
	if remoteHead != head {
		t.Errorf("origin feature/work = %q, want the pushed HEAD %q", remoteHead, head)
	}

	pull := postGitAPI(t, fixture.server, "/api/v1/git/pull", GitPullRequest{})
	if pull.Code != http.StatusOK {
		t.Fatalf("pull status = %d (body %s)", pull.Code, pull.Body.String())
	}
	if result := decodeGitOperationResult(t, pull); !result.Success {
		t.Fatalf("pull failed: %+v", result)
	}
	if got := fixture.head(t); got != head {
		t.Errorf("HEAD moved during a no-op pull: %q != %q", got, head)
	}
}

// TestHandleGitPushPreflight_ReportsRemoteState exercises the preflight
// endpoint, which the changes panel calls before offering a push.
func TestHandleGitPushPreflight_ReportsRemoteState(t *testing.T) {
	fixture := newGitAPIFixture(t)

	rec := postGitAPI(t, fixture.server, "/api/v1/git/push-preflight", GitPushPreflightRequest{})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	result := decodeGitOperationResult(t, rec)
	if result.Operation != "push_preflight" {
		t.Errorf("operation = %q, want %q", result.Operation, "push_preflight")
	}
}

// advanceOriginMain adds a commit to main and publishes it, so the rebase and
// merge handlers — which both fetch and then operate on `origin/<base>` rather
// than the local ref — have something to integrate.
func (f *gitAPIFixture) advanceOriginMain(t *testing.T) string {
	t.Helper()
	runGitAPI(t, f.repo, "checkout", "main")
	writeFileAPI(t, f.repo, "main-only.txt", "main\n")
	runGitAPI(t, f.repo, "add", ".")
	runGitAPI(t, f.repo, "commit", "-m", "main moves")
	runGitAPI(t, f.repo, "push", "origin", "main")
	mainHead := strings.TrimSpace(runGitAPI(t, f.repo, "rev-parse", "HEAD"))
	runGitAPI(t, f.repo, "checkout", "feature/work")
	return mainHead
}

// TestHandleGitRebase_ReplaysOntoBase asserts the branch is genuinely replayed:
// after rebasing onto a moved main, main's new commit is an ancestor of HEAD.
func TestHandleGitRebase_ReplaysOntoBase(t *testing.T) {
	fixture := newGitAPIFixture(t)
	mainHead := fixture.advanceOriginMain(t)

	rec := postGitAPI(t, fixture.server, "/api/v1/git/rebase", GitRebaseRequest{BaseBranch: "main"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	if result := decodeGitOperationResult(t, rec); !result.Success {
		t.Fatalf("rebase failed: %+v", result)
	}
	if got := strings.TrimSpace(runGitAPI(t, fixture.repo, "merge-base", "HEAD", "main")); got != mainHead {
		t.Errorf("merge-base with main = %q, want main's tip %q — the branch was not replayed", got, mainHead)
	}
}

// TestHandleGitMerge_BringsBaseCommitIn covers the merge counterpart, which
// leaves main's commit reachable from HEAD without rewriting the branch.
func TestHandleGitMerge_BringsBaseCommitIn(t *testing.T) {
	fixture := newGitAPIFixture(t)
	featureHead := fixture.head(t)
	mainHead := fixture.advanceOriginMain(t)

	rec := postGitAPI(t, fixture.server, "/api/v1/git/merge", GitMergeRequest{BaseBranch: "main"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	if result := decodeGitOperationResult(t, rec); !result.Success {
		t.Fatalf("merge failed: %+v", result)
	}
	for _, want := range []string{featureHead, mainHead} {
		runGitAPI(t, fixture.repo, "merge-base", "--is-ancestor", want, "HEAD")
	}
}

// TestHandleGitAbort_WithNothingInProgress covers the abort handler's runtime
// path: the guard accepts "rebase", git refuses because nothing is in flight,
// and the failure is reported inside a 200 GitOperationResult rather than as a
// transport error.
func TestHandleGitAbort_WithNothingInProgress(t *testing.T) {
	fixture := newGitAPIFixture(t)

	rec := postGitAPI(t, fixture.server, "/api/v1/git/abort", GitAbortRequest{Operation: "rebase"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	result := decodeGitOperationResult(t, rec)
	if result.Operation != "abort" {
		t.Errorf("operation = %q, want %q", result.Operation, "abort")
	}
	if result.Success {
		t.Errorf("success = true, but no rebase was in progress")
	}
}

// TestHandleGitCreatePR_UnsupportedRemoteReportsProviderError pins the
// create-PR success path's error shape. The fixture's origin is a local path,
// which no PR provider claims; the refusal has to reach the caller inside a
// PRCreateResult (not as a transport failure) so the changes panel can explain
// which remotes are supported.
func TestHandleGitCreatePR_UnsupportedRemoteReportsProviderError(t *testing.T) {
	fixture := newGitAPIFixture(t)

	rec := postGitAPI(t, fixture.server, "/api/v1/git/create-pr", GitCreatePRRequest{
		Title:      "add feature",
		Body:       "body",
		BaseBranch: "main",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var result process.PRCreateResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode PR result: %v", err)
	}
	if result.Success {
		t.Errorf("success = true, want false")
	}
	if !strings.Contains(result.Error, "unsupported git remote for PR creation") {
		t.Errorf("error = %q, want it to name the unsupported remote", result.Error)
	}
}

// TestHandleGitCreatePR_ValidationErrors covers the two 400 branches, both of
// which answer with a PRCreateResult (not the GitOperationResult shape the
// neighbouring handlers use).
func TestHandleGitCreatePR_ValidationErrors(t *testing.T) {
	fixture := newGitAPIFixture(t)

	cases := []struct {
		name      string
		body      any
		wantError string
	}{
		{"malformed body", "{not json", "invalid request: "},
		{"missing title", GitCreatePRRequest{Body: "b"}, "title is required"},
		{"unresolvable repo", GitCreatePRRequest{Title: "t", Repo: "nope"}, "repo subpath not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := postGitAPI(t, fixture.server, "/api/v1/git/create-pr", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
			var result process.PRCreateResult
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatalf("decode PR result: %v", err)
			}
			if result.Success {
				t.Errorf("success = true, want false")
			}
			if !strings.Contains(result.Error, tc.wantError) {
				t.Errorf("error = %q, want it to contain %q", result.Error, tc.wantError)
			}
		})
	}
}

// TestHandleGitLog_BoundsLimit pins both ends of the limit guard: an
// over-large limit is refused outright, and an unparseable one fails binding.
func TestHandleGitLog_BoundsLimit(t *testing.T) {
	fixture := newGitAPIFixture(t)

	cases := []struct {
		name      string
		query     string
		wantError string
	}{
		{"above maximum", "?limit=501", "limit must be between 1 and 500"},
		{"not a number", "?limit=many", "invalid request: "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := getGitAPI(t, fixture.server, "/api/v1/git/log"+tc.query)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
			var result process.GitLogResult
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatalf("decode git log: %v", err)
			}
			if result.Success {
				t.Errorf("success = true, want false")
			}
			if !strings.Contains(result.Error, tc.wantError) {
				t.Errorf("error = %q, want it to contain %q", result.Error, tc.wantError)
			}
		})
	}
}

// TestHandleGitLog_RejectsUnsafeTargetBranch is the sanitiser test. A valid
// target anchors the log at the merge-base with origin/main, so only the
// branch's own two commits come back. An injection-shaped ref must be dropped
// rather than interpolated into the git arguments — which is observable as the
// handler falling back to the open-ended "recent commits" walk that also
// includes main's initial commit.
func TestHandleGitLog_RejectsUnsafeTargetBranch(t *testing.T) {
	fixture := newGitAPIFixture(t)

	anchored := gitLogSubjects(t, fixture.server, "/api/v1/git/log?target_branch=main")
	if len(anchored) != 2 || anchored[0] != "feature two" || anchored[1] != "feature one" {
		t.Fatalf("anchored log = %v, want exactly the branch's own commits", anchored)
	}

	for _, unsafe := range []string{"main;whoami", "main..HEAD", "/etc/passwd", "main.lock", "-main"} {
		t.Run(unsafe, func(t *testing.T) {
			got := gitLogSubjects(t, fixture.server, "/api/v1/git/log?target_branch="+unsafe)
			if len(got) != 3 {
				t.Fatalf("log for %q = %v, want the unanchored 3-commit history "+
					"(the ref must be dropped, not passed to git)", unsafe, got)
			}
			if got[2] != "initial" {
				t.Errorf("oldest commit = %q, want %q", got[2], "initial")
			}
		})
	}
}

// TestHandleGitLog_HonoursSince covers the explicit-base path, where the caller
// supplies the anchor instead of a branch to merge-base against.
func TestHandleGitLog_HonoursSince(t *testing.T) {
	fixture := newGitAPIFixture(t)
	base := strings.TrimSpace(runGitAPI(t, fixture.repo, "rev-parse", "HEAD~1"))

	got := gitLogSubjects(t, fixture.server, "/api/v1/git/log?since="+base)
	if len(got) != 1 || got[0] != "feature two" {
		t.Errorf("log since HEAD~1 = %v, want just [feature two]", got)
	}
}

func gitLogSubjects(t *testing.T, srv *Server, path string) []string {
	t.Helper()
	rec := getGitAPI(t, srv, path)
	if rec.Code != http.StatusOK {
		t.Fatalf("git log %s: status = %d (body %s)", path, rec.Code, rec.Body.String())
	}
	var result process.GitLogResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode git log: %v", err)
	}
	if !result.Success {
		t.Fatalf("git log %s failed: %+v", path, result)
	}
	subjects := make([]string, 0, len(result.Commits))
	for _, commit := range result.Commits {
		subjects = append(subjects, commit.CommitMessage)
	}
	return subjects
}

// TestHandleGitCumulativeDiff_RequiresBase covers the query binding, which is
// the only required parameter on the endpoint.
func TestHandleGitCumulativeDiff_RequiresBase(t *testing.T) {
	fixture := newGitAPIFixture(t)

	rec := getGitAPI(t, fixture.server, "/api/v1/git/cumulative-diff")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
	}
	var result process.CumulativeDiffResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode cumulative diff: %v", err)
	}
	if !strings.HasPrefix(result.Error, "invalid request: ") {
		t.Errorf("error = %q, want an %q prefix", result.Error, "invalid request: ")
	}
}

// TestHandleGitCumulativeDiff_AnchorsOnTargetBranch checks the merge-base
// recomputation: the caller's stale base is replaced by the live divergence
// with origin/main, so the diff carries exactly the branch's own files.
func TestHandleGitCumulativeDiff_AnchorsOnTargetBranch(t *testing.T) {
	fixture := newGitAPIFixture(t)
	stale := fixture.head(t) // HEAD as base would produce an empty diff

	rec := getGitAPI(t, fixture.server, "/api/v1/git/cumulative-diff?base="+stale+"&target_branch=main")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	var result process.CumulativeDiffResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode cumulative diff: %v", err)
	}
	if !result.Success {
		t.Fatalf("cumulative diff failed: %+v", result)
	}
	for _, want := range []string{"one.txt", "two.txt"} {
		if _, ok := result.Files[want]; !ok {
			t.Errorf("files = %v, want it to contain %q", mapKeys(result.Files), want)
		}
	}
	if _, ok := result.Files["README.md"]; ok {
		t.Errorf("files include README.md; the diff is not anchored at the merge-base")
	}
}

// TestHandleGitCumulativeDiff_RejectsUnresolvableRepo covers the 400 branch
// that runGitCumulativeDiffForRepo returns before touching git.
func TestHandleGitCumulativeDiff_RejectsUnresolvableRepo(t *testing.T) {
	fixture := newGitAPIFixture(t)

	rec := getGitAPI(t, fixture.server, "/api/v1/git/cumulative-diff?base=HEAD&repo=nope")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
	}
	var result process.CumulativeDiffResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode cumulative diff: %v", err)
	}
	if !strings.Contains(result.Error, "repo subpath not found") {
		t.Errorf("error = %q, want a repo-subpath failure", result.Error)
	}
}

// TestHandleGitStatus_ReportsWorktreeState asserts the single-repo status
// endpoint reflects the real worktree, including the branch name and the
// classification of each dirty path.
func TestHandleGitStatus_ReportsWorktreeState(t *testing.T) {
	fixture := newGitAPIFixture(t)
	writeFileAPI(t, fixture.repo, "untracked.txt", "new\n")
	writeFileAPI(t, fixture.repo, "one.txt", "edited\n")

	rec := getGitAPI(t, fixture.server, "/api/v1/git/status?fresh=true")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	var result GitStatusResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode git status: %v", err)
	}
	if !result.Success {
		t.Fatalf("git status failed: %+v", result)
	}
	if result.Branch != "feature/work" {
		t.Errorf("branch = %q, want %q", result.Branch, "feature/work")
	}
	if !slicesContains(result.Untracked, "untracked.txt") {
		t.Errorf("untracked = %v, want it to contain untracked.txt", result.Untracked)
	}
	if !slicesContains(result.Modified, "one.txt") {
		t.Errorf("modified = %v, want it to contain one.txt", result.Modified)
	}
	if result.Timestamp == "" {
		t.Errorf("timestamp is empty; the response must carry a snapshot time")
	}
}

// TestHandleGitStatus_RejectsUnresolvableRepo covers the tracker-lookup 400.
func TestHandleGitStatus_RejectsUnresolvableRepo(t *testing.T) {
	fixture := newGitAPIFixture(t)

	rec := getGitAPI(t, fixture.server, "/api/v1/git/status?repo=nope")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
	}
	var result GitStatusResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode git status: %v", err)
	}
	if result.Success {
		t.Errorf("success = true, want false")
	}
	if !strings.Contains(result.Error, "repo subpath not found") {
		t.Errorf("error = %q, want a repo-subpath failure", result.Error)
	}
}

// TestHandleGitStatusMulti_SingleRepoUsesEmptyRepositoryName pins the uniform
// response shape callers rely on: a single-repo workspace still answers with
// the multi shape, tagged with an empty repository name.
func TestHandleGitStatusMulti_SingleRepoUsesEmptyRepositoryName(t *testing.T) {
	fixture := newGitAPIFixture(t)

	rec := getGitAPI(t, fixture.server, "/api/v1/git/status/multi")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	var result MultiRepoGitStatusResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode multi status: %v", err)
	}
	if !result.Success || len(result.Repos) != 1 {
		t.Fatalf("multi status = %+v, want exactly one repo entry", result)
	}
	if result.Repos[0].RepositoryName != "" {
		t.Errorf("repository_name = %q, want empty for a single-repo workspace", result.Repos[0].RepositoryName)
	}
	if result.Repos[0].Status.Branch != "feature/work" {
		t.Errorf("branch = %q, want %q", result.Repos[0].Status.Branch, "feature/work")
	}
}

// TestHandleGitError_ClassifiesOperationInProgress pins the one branch of
// handleGitError that is not a generic 500: a busy GitOperator must answer 409
// with a stable, non-leaking message, because the frontend retries on that
// code and shows the raw error on every other one.
func TestHandleGitError_ClassifiesOperationInProgress(t *testing.T) {
	fixture := newGitAPIFixture(t)

	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantError  string
	}{
		{
			"busy operator",
			process.ErrOperationInProgress,
			http.StatusConflict,
			"another git operation is already in progress",
		},
		{"other failure", errors.New("fatal: not a git repository"), http.StatusInternalServerError, "fatal: not a git repository"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			fixture.server.handleGitError(c, "rebase", tc.err)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			result := decodeGitOperationResult(t, rec)
			if result.Success {
				t.Errorf("success = true, want false")
			}
			if result.Operation != "rebase" {
				t.Errorf("operation = %q, want %q", result.Operation, "rebase")
			}
			if result.Error != tc.wantError {
				t.Errorf("error = %q, want %q", result.Error, tc.wantError)
			}
		})
	}
}

func slicesContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
