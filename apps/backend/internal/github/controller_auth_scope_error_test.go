package github

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// A repository-scope rejection must reach the client as the same sanitized 404
// used for every genuinely unresolvable repository: the endpoint keeps its
// no-leak contract, so a caller cannot enumerate which repositories merely fall
// outside the workspace's configured scope. Only the message and the code
// differ, and both exist so operators stop diagnosing a scope problem as a
// credential or App-installation problem.
func TestWriteGitHubOperationalAuthErrorScopeRejectionKeepsSanitized404(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	if !writeGitHubOperationalAuthError(ctx, newRepoOutsideScopeErr("acme", "widget")) {
		t.Fatal("scope rejection must be handled by the shared mapping, not fall through to a raw 500")
	}
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}

	var body struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != "github_repository_out_of_scope" {
		t.Fatalf("code = %q, want github_repository_out_of_scope", body.Code)
	}
	if !strings.Contains(body.Error, "GitHub repository scope") {
		t.Fatalf("error must name the real cause, got %q", body.Error)
	}
	// Each repository identity carries the same status, so the endpoint does not
	// become a scope-enumeration oracle.
	for _, leaked := range []string{"acme", "widget", "outside this workspace's scope"} {
		if strings.Contains(recorder.Body.String(), leaked) {
			t.Fatalf("scope rejection body leaked %q: %s", leaked, recorder.Body.String())
		}
	}
}

// The unresolvable-repository branch keeps its own code under the same 404, so
// server-side triage can still separate the two causes apart from the status.
func TestWriteGitHubOperationalAuthErrorUnresolvableKeeps404(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	if !writeGitHubOperationalAuthError(ctx, ErrRepoNotResolvable) {
		t.Fatal("unresolvable repository must be handled by the shared mapping")
	}
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}

	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != "github_repository_not_found" {
		t.Fatalf("code = %q, want github_repository_not_found", body.Code)
	}
}
