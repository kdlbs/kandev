package api

import (
	"net/http"
	"strings"
	"testing"
)

// newLocalOnlyGitAPIFixture starts from the shared remote fixture, then removes
// origin so the operation must resolve the checked-out repository's local refs.
func newLocalOnlyGitAPIFixture(t *testing.T) *gitAPIFixture {
	t.Helper()
	fixture := newGitAPIFixture(t)
	runGitAPI(t, fixture.repo, "remote", "remove", "origin")
	return fixture
}

func advanceLocalMain(t *testing.T, fixture *gitAPIFixture) string {
	t.Helper()
	runGitAPI(t, fixture.repo, "checkout", "main")
	writeFileAPI(t, fixture.repo, "local-main-only.txt", "local main\n")
	runGitAPI(t, fixture.repo, "add", ".")
	runGitAPI(t, fixture.repo, "commit", "-m", "local main moves")
	mainHead := fixture.head(t)
	runGitAPI(t, fixture.repo, "checkout", "feature/work")
	return mainHead
}

func TestHandleGitRebase_LocalOnly(t *testing.T) {
	fixture := newLocalOnlyGitAPIFixture(t)
	mainHead := advanceLocalMain(t, fixture)

	rec := postGitAPI(t, fixture.server, "/api/v1/git/rebase", GitRebaseRequest{BaseBranch: "main"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	if result := decodeGitOperationResult(t, rec); !result.Success {
		t.Fatalf("rebase failed: %+v", result)
	}
	if got := strings.TrimSpace(runGitAPI(t, fixture.repo, "merge-base", "HEAD", "main")); got != mainHead {
		t.Errorf("merge-base with main = %q, want local main's tip %q", got, mainHead)
	}
}

func TestHandleGitMerge_LocalOnly(t *testing.T) {
	fixture := newLocalOnlyGitAPIFixture(t)
	featureHead := fixture.head(t)
	mainHead := advanceLocalMain(t, fixture)

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

func TestHandleGitRebase_MissingLocalBase(t *testing.T) {
	assertMissingLocalBaseDoesNotMoveHead(t, "/api/v1/git/rebase", GitRebaseRequest{BaseBranch: "missing"})
}

func TestHandleGitMerge_MissingLocalBase(t *testing.T) {
	assertMissingLocalBaseDoesNotMoveHead(t, "/api/v1/git/merge", GitMergeRequest{BaseBranch: "missing"})
}

func assertMissingLocalBaseDoesNotMoveHead(t *testing.T, path string, body any) {
	t.Helper()
	fixture := newLocalOnlyGitAPIFixture(t)
	before := fixture.head(t)

	rec := postGitAPI(t, fixture.server, path, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	result := decodeGitOperationResult(t, rec)
	if result.Success {
		t.Fatalf("operation succeeded: %+v", result)
	}
	if !strings.HasPrefix(result.Error, `base branch "missing" does not exist locally`) {
		t.Errorf("error = %q, want missing-local-base error", result.Error)
	}
	if !strings.Contains(result.Error, "fatal: Needed a single revision") {
		t.Errorf("error = %q, want underlying git error", result.Error)
	}
	if after := fixture.head(t); after != before {
		t.Errorf("HEAD moved from %q to %q", before, after)
	}
}
