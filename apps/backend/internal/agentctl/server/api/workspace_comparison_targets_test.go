package api

import (
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestHandleSetComparisonTargetsRejectsMalformedAndUnsafeBindings(t *testing.T) {
	fixture := newGitAPIFixture(t)

	if rec := postGitAPI(t, fixture.server, "/api/v1/workspace/comparison-targets", "{not json"); rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed status = %d, want 400", rec.Code)
	}
	if rec := postGitAPI(t, fixture.server, "/api/v1/workspace/comparison-targets", SetComparisonTargetsRequest{
		ComparisonTargets: map[string]models.ComparisonTarget{"../outside": {}},
	}); rec.Code != http.StatusBadRequest {
		t.Fatalf("unsafe key status = %d, want 400", rec.Code)
	}
	if rec := postGitAPI(t, fixture.server, "/api/v1/workspace/comparison-targets", SetComparisonTargetsRequest{
		ComparisonTargets: map[string]models.ComparisonTarget{"": {}},
	}); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid target status = %d, want 400", rec.Code)
	}
}
