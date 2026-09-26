package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// publishStagingAt creates and pushes `staging` at the given ref, giving the
// fixture a second live upstream branch to compare against. Without a second
// branch, selecting a base branch is indistinguishable from the default.
func (f *gitAPIFixture) publishStagingAt(t *testing.T, ref string) string {
	t.Helper()
	runGitAPI(t, f.repo, "branch", "staging", ref)
	runGitAPI(t, f.repo, "push", "origin", "staging")
	return strings.TrimSpace(runGitAPI(t, f.repo, "rev-parse", "staging"))
}

// joinDetachedBaseBranchRefresh blocks until the goroutine UpdateBaseBranches
// spawns has finished. That goroutine runs RefreshGitStatus with
// context.Background — it does not stop when the request ends — and it shells
// out to git inside the fixture's t.TempDir(), so a test that returned without
// joining it would race its own directory removal.
//
// RefreshGitStatus takes the tracker's updateMu for the whole call, so calling
// it here cannot return until the detached one has released it. That makes it a
// join, not a wait.
func joinDetachedBaseBranchRefresh(t *testing.T, srv *Server) {
	t.Helper()
	srv.procMgr.GetWorkspaceTracker().RefreshGitStatus(context.Background())
}

func gitStatusBaseCommit(t *testing.T, srv *Server) string {
	t.Helper()
	rec := getGitAPI(t, srv, "/api/v1/git/status?fresh=true")
	if rec.Code != http.StatusOK {
		t.Fatalf("git status = %d (body %s)", rec.Code, rec.Body.String())
	}
	var status GitStatusResult
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode git status: %v", err)
	}
	if !status.Success {
		t.Fatalf("git status failed: %+v", status)
	}
	return status.BaseCommit
}

// TestHandleSetBaseBranches_RetargetsComparison asserts the endpoint's actual
// effect: after selecting `staging`, the tracker's reported base commit moves
// to the merge-base with that branch. Returning ok:true without re-stamping the
// tracker would leave the changes panel comparing against the old branch until
// the session restarts, which is the bug this endpoint exists to prevent.
func TestHandleSetBaseBranches_RetargetsComparison(t *testing.T) {
	fixture := newGitAPIFixture(t)
	stagingTip := fixture.publishStagingAt(t, "HEAD~1")
	defaultBase := gitStatusBaseCommit(t, fixture.server)
	if defaultBase == stagingTip {
		t.Fatalf("fixture is degenerate: the default base already equals staging's tip %q", stagingTip)
	}

	rec := postGitAPI(t, fixture.server, "/api/v1/workspace/base-branches", SetBaseBranchesRequest{
		BaseBranches: map[string]string{"": "staging"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	var body map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body["ok"] {
		t.Errorf("response = %v, want ok:true", body)
	}
	joinDetachedBaseBranchRefresh(t, fixture.server)
	if got := gitStatusBaseCommit(t, fixture.server); got != stagingTip {
		t.Errorf("base_commit = %q, want the merge-base with staging %q", got, stagingTip)
	}
}

// TestHandleSetBaseBranches_DropsUnsafeRefs is the sanitiser test. Each of
// these refs is interpolated straight into `git` arguments downstream, so the
// handler must drop them — observable as the tracker keeping the default
// comparison base instead of adopting the rejected ref.
func TestHandleSetBaseBranches_DropsUnsafeRefs(t *testing.T) {
	for _, unsafe := range []string{"staging;whoami", "staging..HEAD", "--upload-pack=touch", "/etc/passwd", "staging.lock"} {
		t.Run(unsafe, func(t *testing.T) {
			fixture := newGitAPIFixture(t)
			stagingTip := fixture.publishStagingAt(t, "HEAD~1")
			defaultBase := gitStatusBaseCommit(t, fixture.server)

			rec := postGitAPI(t, fixture.server, "/api/v1/workspace/base-branches", SetBaseBranchesRequest{
				BaseBranches: map[string]string{"": unsafe},
			})
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
			}
			joinDetachedBaseBranchRefresh(t, fixture.server)
			got := gitStatusBaseCommit(t, fixture.server)
			if got == stagingTip {
				t.Fatalf("base_commit moved to staging's tip; %q was accepted as a ref", unsafe)
			}
			if got != defaultBase {
				t.Errorf("base_commit = %q, want the untouched default %q", got, defaultBase)
			}
		})
	}
}

// TestHandleSetBaseBranches_AcceptsOriginPrefixedRefs pins the one shape the
// allowlist has to special-case: `origin/<branch>` is split before validation,
// because the underlying validator rejects a leading path separator.
func TestHandleSetBaseBranches_AcceptsOriginPrefixedRefs(t *testing.T) {
	fixture := newGitAPIFixture(t)
	stagingTip := fixture.publishStagingAt(t, "HEAD~1")

	rec := postGitAPI(t, fixture.server, "/api/v1/workspace/base-branches", SetBaseBranchesRequest{
		BaseBranches: map[string]string{"": "origin/staging"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	joinDetachedBaseBranchRefresh(t, fixture.server)
	if got := gitStatusBaseCommit(t, fixture.server); got != stagingTip {
		t.Errorf("base_commit = %q, want the merge-base with origin/staging %q", got, stagingTip)
	}
}

// TestHandleSetBaseBranches_RejectsMalformedBody covers the binding guard.
func TestHandleSetBaseBranches_RejectsMalformedBody(t *testing.T) {
	fixture := newGitAPIFixture(t)

	rec := postGitAPI(t, fixture.server, "/api/v1/workspace/base-branches", "{not json")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body[errKey] != "invalid JSON body" {
		t.Errorf("error = %q, want %q", body[errKey], "invalid JSON body")
	}
}
