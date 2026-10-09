package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
)

func TestPromptMutationsRequireOrgConfigManage(t *testing.T) {
	router, cleanup := newTestRouterForIdentity(t, authn.Identity{
		UserID: "member-1", Role: authn.RoleMember, OrgID: "org-1",
	})
	defer cleanup()

	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/prompts"},
		{http.MethodPatch, "/api/v1/prompts/prompt-1"},
		{http.MethodDelete, "/api/v1/prompts/prompt-1"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			request := httptest.NewRequest(route.method, route.path, strings.NewReader(`{"name":"shared","content":"instructions"}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusForbidden, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), "org.config.manage") {
				t.Fatalf("body = %s, want an org.config.manage refusal", response.Body.String())
			}
		})
	}
}

func TestPromptReadsRemainAvailableToOrgMembers(t *testing.T) {
	router, cleanup := newTestRouterForIdentity(t, authn.Identity{
		UserID: "member-1", Role: authn.RoleMember, OrgID: "org-1",
	})
	defer cleanup()

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/prompts", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
}
