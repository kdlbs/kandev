package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/process"
)

func TestContributionHistoryAPI(t *testing.T) {
	fixture := newGitAPIFixture(t)
	localHead := fixture.head(t)
	remoteHead := strings.TrimSpace(runGitAPI(t, fixture.repo, "rev-parse", "origin/main"))

	rec := postGitAPI(t, fixture.server, "/api/v1/git/contribution/history-explanation", map[string]any{
		"branch":               fixture.branch(t),
		"expected_local_head":  localHead,
		"expected_remote_head": remoteHead,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var result process.ContributionHistoryExplanationResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Branch != fixture.branch(t) || result.ExpectedLocalHead != localHead ||
		result.ExpectedRemoteHead != remoteHead {
		t.Fatalf("result identity = %+v, want request identity", result)
	}
	if result.Kind != "unexplained" || result.Reason != "no_matching_reflog" {
		t.Fatalf("result = %+v, want neutral no-matching-reflog response", result)
	}
}

func TestContributionHistoryAPIRejectsMissingFields(t *testing.T) {
	fixture := newGitAPIFixture(t)
	for _, field := range []string{"branch", "expected_local_head", "expected_remote_head"} {
		t.Run(field, func(t *testing.T) {
			body := map[string]any{
				"branch":               fixture.branch(t),
				"expected_local_head":  localHeadForAPI(t, fixture),
				"expected_remote_head": strings.TrimSpace(runGitAPI(t, fixture.repo, "rev-parse", "origin/main")),
			}
			delete(body, field)
			rec := postGitAPI(t, fixture.server, "/api/v1/git/contribution/history-explanation", body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func localHeadForAPI(t *testing.T, fixture *gitAPIFixture) string {
	t.Helper()
	return fixture.head(t)
}
