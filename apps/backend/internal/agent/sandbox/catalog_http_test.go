package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestSandboxPolicyCatalogScopesLatestRevisions(t *testing.T) {
	service, err := NewService(sqlitePolicyDB(t), true, nil)
	require.NoError(t, err)
	service.SetWorkspaceAuthorizer(func(_ context.Context, workspace string) error {
		if workspace == "workspace" {
			return nil
		}
		return ErrAuthority
	})
	operator := operatorContext(t)
	original := policyFixture(t)
	original.ID = "global"
	first, err := service.Publish(operator, original, 0)
	require.NoError(t, err)
	original.Name = "Updated policy"
	latest, err := service.Publish(operator, original, first.Number)
	require.NoError(t, err)
	scoped := policyFixture(t)
	scoped.ID, scoped.WorkspaceID = "scoped", "workspace"
	local, err := service.Publish(operator, scoped, 0)
	require.NoError(t, err)
	identity, _ := authn.IdentityFromContext(operator)
	foreign := identity
	foreign.OrgID = "foreign"
	_, err = service.Publish(authn.WithIdentity(t.Context(), foreign), policyFixture(t), 0)
	require.NoError(t, err)
	router := gin.New()
	router.Use(func(c *gin.Context) { authn.SetOnGin(c, identity) })
	service.RegisterRoutes(router)
	for _, test := range []struct {
		query  string
		status int
		ids    []string
	}{
		{"", 200, []string{latest.ID}},
		{"?workspace_id=workspace", 200, []string{latest.ID, local.ID}},
		{"?workspace_id=denied", 404, nil},
		{"?after=global", 200, []string{}},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/sandbox/policies"+test.query, nil))
		require.Equal(t, test.status, response.Code)
		if test.status != 200 {
			continue
		}
		var result struct {
			Policies []*Revision `json:"policies"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
		ids := []string{}
		for _, revision := range result.Policies {
			ids = append(ids, revision.ID)
		}
		require.Equal(t, test.ids, ids)
	}
}

func TestSandboxPolicyCatalogPaginationSQLite(t *testing.T) {
	testSandboxCatalogPagination(t, sqlitePolicyDB(t))
}

func TestSandboxPolicyCatalogPaginationPostgres(t *testing.T) {
	testSandboxCatalogPagination(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}

func testSandboxCatalogPagination(t *testing.T, database *sqlx.DB) {
	t.Helper()
	service, err := NewService(database, true, nil)
	require.NoError(t, err)
	ctx := operatorContext(t)
	policy := policyFixture(t)
	var last *Revision
	for i := range policyCatalogPageSize + 1 {
		policy.ID = fmt.Sprintf("policy-%03d", i)
		last, err = service.Publish(ctx, policy, 0)
		require.NoError(t, err)
	}
	require.NoError(t, service.Revoke(ctx, last.ID, last.Digest))
	first, err := service.ListPolicies(ctx, "", "")
	require.NoError(t, err)
	require.Len(t, first.Policies, policyCatalogPageSize)
	require.Equal(t, "policy-063", first.NextCursor)
	second, err := service.ListPolicies(ctx, "", first.NextCursor)
	require.NoError(t, err)
	require.Len(t, second.Policies, 1)
	require.Equal(t, last.ID, second.Policies[0].ID)
	require.True(t, second.Policies[0].Revoked)
	require.Empty(t, second.NextCursor)
}

func TestSandboxPolicyCatalogRequiresHumanOperator(t *testing.T) {
	for _, identity := range []authn.Identity{
		{UserID: "member", Role: authn.RoleMember, SessionID: "browser"},
		{UserID: "operator", Role: authn.RoleAdmin, TokenID: "token"},
	} {
		service, err := NewService(sqlitePolicyDB(t), true, nil)
		require.NoError(t, err)
		router := gin.New()
		router.Use(func(c *gin.Context) { authn.SetOnGin(c, identity) })
		service.RegisterRoutes(router)
		for _, path := range []string{"/policies", "/providers"} {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/sandbox"+path, nil))
			require.Equal(t, http.StatusForbidden, response.Code)
		}
		for _, path := range []string{"/preview", "/availability"} {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/sandbox"+path, strings.NewReader(`{"executor_id":"executor"}`)))
			require.Equal(t, http.StatusForbidden, response.Code)
		}
	}
}
