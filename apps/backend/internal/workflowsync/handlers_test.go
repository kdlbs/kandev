package workflowsync

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const (
	victimWorkspace = "ws-victim"
	victimOwnerID   = "owner-1"
	attackerID      = "attacker-1"
	victimRepoOwner = "victim-org"
	victimRepoName  = "victim-repo"
)

// ownerOnlyAuthorizer mirrors the real callerScope semantics documented in
// apps/backend/AGENTS.md: no identity in context, or a synthetic identity
// (auth disabled), is unscoped; a real identity may only reach the workspace
// it owns.
func ownerOnlyAuthorizer(ownerID, workspaceID string) func(context.Context, string) error {
	return func(ctx context.Context, wsID string) error {
		identity, ok := authn.IdentityFromContext(ctx)
		if !ok || identity.Synthetic {
			return nil
		}
		if wsID == workspaceID && identity.UserID == ownerID {
			return nil
		}
		return repoerrors.ErrWorkspaceNotFound
	}
}

func newTestRouter(t *testing.T, svc *Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	RegisterRoutes(router, svc, log)
	return router
}

func withIdentity(req *http.Request, identity authn.Identity) *http.Request {
	return req.WithContext(authn.WithIdentity(req.Context(), identity))
}

func TestHTTPHandlers_OwnerCanReadAndWriteOwnWorkspace(t *testing.T) {
	svc, _ := setupTestService(t, seededMockClient())
	configureWorkspace(t, svc, victimWorkspace)
	svc.SetWorkspaceAuthorizer(ownerOnlyAuthorizer(victimOwnerID, victimWorkspace))
	router := newTestRouter(t, svc)
	owner := authn.Identity{UserID: victimOwnerID, Role: authn.RoleMember}

	req := withIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/workflow-sync/config?workspace_id="+victimWorkspace, nil), owner)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Contains(t, resp.Body.String(), "acme")
}

func TestHTTPHandlers_ForeignMemberDeniedOnAllRoutesWithoutLeaking(t *testing.T) {
	svc, applier := setupTestService(t, seededMockClient())
	configureWorkspace(t, svc, victimWorkspace)
	require.NoError(t, updateVictimRepo(t, svc))
	svc.SetWorkspaceAuthorizer(ownerOnlyAuthorizer(victimOwnerID, victimWorkspace))
	router := newTestRouter(t, svc)
	attacker := authn.Identity{UserID: attackerID, Role: authn.RoleMember}

	cases := []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{"get", http.MethodGet, "/api/v1/workflow-sync/config?workspace_id=" + victimWorkspace, nil},
		{
			"post", http.MethodPost, "/api/v1/workflow-sync/config?workspace_id=" + victimWorkspace,
			[]byte(`{"repo_owner":"attacker","repo_name":"evil"}`),
		},
		{"delete", http.MethodDelete, "/api/v1/workflow-sync/config?workspace_id=" + victimWorkspace, nil},
		{"sync", http.MethodPost, "/api/v1/workflow-sync/sync?workspace_id=" + victimWorkspace, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body *bytes.Reader
			if tc.body != nil {
				body = bytes.NewReader(tc.body)
			} else {
				body = bytes.NewReader(nil)
			}
			req := withIdentity(httptest.NewRequest(tc.method, tc.path, body), attacker)
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
			assert.Equal(t, http.StatusNotFound, resp.Code, "response body: %s", resp.Body.String())
			assert.NotContains(t, resp.Body.String(), victimRepoOwner)
			assert.NotContains(t, resp.Body.String(), victimRepoName)
		})
	}

	cfg, err := svc.store.GetConfigForWorkspace(context.Background(), victimWorkspace)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, victimRepoOwner, cfg.RepoOwner, "denied POST must not overwrite the victim's config")
	assert.Equal(t, victimRepoName, cfg.RepoName)
	assert.Empty(t, applier.released, "denied DELETE must not release synced workflows")
	assert.Zero(t, applier.callCount(), "denied sync must never reach the applier")
}

func TestHTTPHandlers_SyntheticIdentitySucceedsWhenAuthDisabled(t *testing.T) {
	svc, _ := setupTestService(t, seededMockClient())
	configureWorkspace(t, svc, victimWorkspace)
	svc.SetWorkspaceAuthorizer(ownerOnlyAuthorizer(victimOwnerID, victimWorkspace))
	router := newTestRouter(t, svc)
	synthetic := authn.Identity{UserID: "single-user", Role: authn.RoleAdmin, Synthetic: true}

	req := withIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/workflow-sync/config?workspace_id="+victimWorkspace, nil), synthetic)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Code)
}

// updateVictimRepo overwrites the seeded config with distinctive repo
// identity so leak assertions aren't relying on the shared default fixture
// values used elsewhere in this package's tests.
func updateVictimRepo(t *testing.T, svc *Service) error {
	t.Helper()
	_, err := svc.store.UpsertConfigForWorkspace(context.Background(), victimWorkspace, &SetConfigRequest{
		RepoOwner:       victimRepoOwner,
		RepoName:        victimRepoName,
		Branch:          DefaultBranch,
		Path:            DefaultPath,
		IntervalSeconds: DefaultIntervalSeconds,
	})
	return err
}
