package dashboard_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/routing"
)

var testSetupWarnings = []routing.SetupWarning{
	{
		Code:        routing.SetupWarningAgentSingleProvider,
		Declaration: routing.ExclusionDeclaration{WorkflowName: "Office Review", StepName: "Review", Role: "reviewer"},
		ProviderID:  "codex-acp", AgentID: "q", AgentName: "QA",
	},
}

func TestRouting_GetIncludesAuthorExclusionWarnings(t *testing.T) {
	deps, fake := newRoutingTestDeps(t)
	fake.setupWarnings = testSetupWarnings
	rec := httptest.NewRecorder()
	deps.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws-1/routing", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var resp dashboard.RoutingConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := dashboard.AuthorExclusionWarningDTO{
		Code: "agent_single_provider", WorkflowName: "Office Review", StepName: "Review",
		Role: "reviewer", ProviderID: "codex-acp", AgentName: "QA",
	}
	if len(resp.AuthorExclusionWarnings) != 1 || resp.AuthorExclusionWarnings[0] != want {
		t.Fatalf("warnings = %+v, want [%+v]", resp.AuthorExclusionWarnings, want)
	}
}

func TestRouting_GetWithoutWarningsReturnsEmptyList(t *testing.T) {
	deps, _ := newRoutingTestDeps(t)
	rec := httptest.NewRecorder()
	deps.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws-1/routing", nil))
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(raw["author_exclusion_warnings"]) != "[]" {
		t.Fatalf("author_exclusion_warnings = %s, want []", raw["author_exclusion_warnings"])
	}
}

func TestRouting_GetReportsCheckFailedWithoutFailingRequest(t *testing.T) {
	deps, fake := newRoutingTestDeps(t)
	fake.setupErr = errors.New("db down")
	rec := httptest.NewRecorder()
	deps.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws-1/routing", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var resp dashboard.RoutingConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.AuthorExclusionWarnings) != 1 || resp.AuthorExclusionWarnings[0].Code != "check_failed" {
		t.Fatalf("warnings = %+v, want one check_failed", resp.AuthorExclusionWarnings)
	}
}

func TestRouting_PutReturnsAuthorExclusionWarnings(t *testing.T) {
	deps, fake := newRoutingTestDeps(t)
	fake.setupWarnings = testSetupWarnings
	body, _ := json.Marshal(routing.WorkspaceConfig{DefaultTier: routing.TierBalanced})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/office/workspaces/ws-1/routing", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var resp dashboard.RoutingUpdateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.OK || len(resp.AuthorExclusionWarnings) != 1 || resp.AuthorExclusionWarnings[0].AgentName != "QA" {
		t.Fatalf("response = %+v", resp)
	}
}
