package sandbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/stretchr/testify/require"
)

func sandboxOperatorRouter(t *testing.T, service *Service) *gin.Engine {
	t.Helper()
	identity, _ := authn.IdentityFromContext(operatorContext(t))
	router := gin.New()
	router.Use(func(c *gin.Context) { authn.SetOnGin(c, identity) })
	service.RegisterRoutes(router)
	return router
}

func TestSandboxPolicyPreviewUsesSavedSelection(t *testing.T) {
	service, ceiling, read := policySelectionFixture(t)
	_, err := service.PutBinding(operatorContext(t), Binding{Scope: ScopeAgent, TargetID: "agent", RevisionID: read.ID}, 0)
	require.NoError(t, err)
	router := sandboxOperatorRouter(t, service)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/sandbox/preview", strings.NewReader(`{"executor_id":"executor","agent_profile_id":"agent"}`)))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var result struct {
		Resolved *Resolved   `json:"resolved"`
		Eligible []*Revision `json:"eligible"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.NotNil(t, result.Resolved)
	require.Equal(t, read.ID, result.Resolved.Revision.ID)
	require.Equal(t, ceiling.ID, result.Resolved.Ceiling.ID)
	require.Equal(t, ScopeAgent, result.Resolved.SelectionSource)
	require.Len(t, result.Eligible, 2)
	unchanged, err := service.GetBinding(operatorContext(t), ScopeAgent, "agent")
	require.NoError(t, err)
	require.Equal(t, int64(1), unchanged.Version)
}

func TestSandboxPolicyPreviewRejectsForeignContext(t *testing.T) {
	service, _, _ := policySelectionFixture(t)
	service.SetWorkspaceAuthorizer(func(_ context.Context, id string) error {
		if id == "workspace" {
			return nil
		}
		return ErrAuthority
	})
	service.SetTargetResolver(func(_ context.Context, scope, id string) (TargetScope, error) {
		if id == "foreign" {
			return TargetScope{OrgID: "foreign"}, nil
		}
		if scope == ScopeAgent {
			return TargetScope{OrgID: "org", WorkspaceID: "workspace"}, nil
		}
		return TargetScope{OrgID: "org"}, nil
	})
	router := sandboxOperatorRouter(t, service)
	for _, body := range []string{
		`{"executor_id":"foreign"}`,
		`{"executor_id":"executor","agent_profile_id":"foreign"}`,
		`{"executor_id":"executor","agent_profile_id":"agent"}`,
		`{"executor_id":"executor","workspace_id":"denied"}`,
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/sandbox/preview", strings.NewReader(body)))
		require.Equal(t, http.StatusNotFound, response.Code)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/sandbox/preview", strings.NewReader(`{"executor_id":"executor","org_id":"foreign"}`)))
	require.Equal(t, http.StatusBadRequest, response.Code)
}

func TestSandboxPolicyPreviewRetainsAlternativesForInvalidSelection(t *testing.T) {
	service, ceiling, _ := policySelectionFixture(t)
	policy := ceiling.Policy
	policy.ID = "other"
	other, err := service.Publish(operatorContext(t), policy, 0)
	require.NoError(t, err)
	_, err = service.PutBinding(operatorContext(t), Binding{Scope: ScopeAgent, TargetID: "agent", RevisionID: other.ID}, 0)
	require.NoError(t, err)
	response := httptest.NewRecorder()
	sandboxOperatorRouter(t, service).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/sandbox/preview", strings.NewReader(`{"executor_id":"executor","agent_profile_id":"agent"}`)))
	require.Equal(t, http.StatusOK, response.Code)
	var result struct {
		Resolved        *Resolved   `json:"resolved"`
		Eligible        []*Revision `json:"eligible"`
		ResolutionError string      `json:"resolution_error"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.Nil(t, result.Resolved)
	require.Equal(t, "sandbox_policy_ineligible", result.ResolutionError)
	require.Len(t, result.Eligible, 2)
	stored, err := service.GetBinding(operatorContext(t), ScopeAgent, "agent")
	require.NoError(t, err)
	require.Equal(t, other.ID, stored.RevisionID)
}

func TestSandboxProviderCatalogUsesGenericMetadata(t *testing.T) {
	service, _, _ := policySelectionFixture(t)
	service.SetProviderCatalog(func(context.Context) ([]ProviderView, error) {
		return []ProviderView{
			{InstallationID: "first", ProviderKey: "alpha", DisplayName: "Alpha", Version: "1.0.0", Available: true},
			{InstallationID: "second", ProviderKey: "beta", DisplayName: "Beta", Version: "2.0.0", Available: false},
		}, nil
	})
	response := httptest.NewRecorder()
	sandboxOperatorRouter(t, service).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/sandbox/providers", nil))
	require.Equal(t, http.StatusOK, response.Code)
	var result struct {
		Providers []ProviderView `json:"providers"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.Len(t, result.Providers, 2)
	require.True(t, result.Providers[0].Available)
	require.False(t, result.Providers[1].Available)
	service.SetProviderCatalog(nil)
	response = httptest.NewRecorder()
	sandboxOperatorRouter(t, service).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/sandbox/providers", nil))
	require.JSONEq(t, `{"providers":[]}`, response.Body.String())
}
