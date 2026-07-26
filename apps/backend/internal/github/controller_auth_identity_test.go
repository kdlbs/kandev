package github

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth/authn"
)

func TestCurrentGitHubUserIDUsesRequestIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	authn.SetOnGin(ctx, authn.Identity{UserID: "member-1", Role: authn.RoleMember})

	if got := currentGitHubUserID(ctx); got != "member-1" {
		t.Fatalf("currentGitHubUserID() = %q, want request identity", got)
	}
}

func TestGHCLIOperatorRoutesDenyMembersWithoutLeakingHostAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, _ := newWorkspaceConnectionService(t, "operator")
	service.ghAccountLister = func(_ context.Context) ([]GHAccount, error) {
		return []GHAccount{{Host: "github.com", Login: "host-account"}}, nil
	}
	router := gin.New()
	NewController(service, testLogger(t)).RegisterHTTPRoutes(router)

	member := authn.Identity{UserID: "member-1", Role: authn.RoleMember}
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/v1/github/auth/gh-cli/accounts", nil),
		httptest.NewRequest(http.MethodPut, "/api/v1/github/workspace-connection?workspace_id=ws-1",
			bytes.NewBufferString(`{"source":"gh_cli","host":"github.com","login":"host-account"}`)),
	} {
		request = request.WithContext(authn.WithIdentity(request.Context(), member))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("member response = %d %s, want sanitized 403", response.Code, response.Body.String())
		}
		if bytes.Contains(response.Body.Bytes(), []byte("host-account")) ||
			bytes.Contains(response.Body.Bytes(), []byte("github.com")) {
			t.Fatalf("member denial leaked host account: %s", response.Body.String())
		}
	}
}

func TestGHCLIOperatorAccountListingAllowsAdminAndSyntheticIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, _ := newWorkspaceConnectionService(t, "operator")
	service.ghAccountLister = func(_ context.Context) ([]GHAccount, error) {
		return []GHAccount{{Host: "github.com", Login: "host-account"}}, nil
	}
	router := gin.New()
	NewController(service, testLogger(t)).RegisterHTTPRoutes(router)

	for _, identity := range []authn.Identity{
		{UserID: "admin-1", Role: authn.RoleAdmin},
		{UserID: DefaultUserID, Role: authn.RoleAdmin, Synthetic: true},
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/github/auth/gh-cli/accounts", nil)
		request = request.WithContext(authn.WithIdentity(request.Context(), identity))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("host-account")) {
			t.Fatalf("identity %#v listing = %d %s, want account", identity, response.Code, response.Body.String())
		}
	}
}
