package workflowsync

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const (
	victimWorkspace = "ws-victim"
	victimOwnerID   = "owner-1"
	attackerID      = "attacker-1"
	victimRepoOwner = "victim-org"
	victimRepoName  = "victim-repo"
	victimBranch    = "victim-branch"
	victimPath      = "victim/path"
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

func withIdentity(req *http.Request, identity authn.Identity) *http.Request {
	return req.WithContext(authn.WithIdentity(req.Context(), identity))
}

// configOp is one of the four workflow-sync HTTP entry points, parameterized
// by workspace ID so every test in this file can drive all four uniformly.
type configOp struct {
	name   string
	method string
	path   func(workspaceID string) string
	body   []byte
}

var allConfigOps = []configOp{
	{
		name: "get", method: http.MethodGet,
		path: func(ws string) string { return "/api/v1/workflow-sync/config?workspace_id=" + ws },
	},
	{
		name: "post", method: http.MethodPost,
		path: func(ws string) string { return "/api/v1/workflow-sync/config?workspace_id=" + ws },
		body: []byte(`{"repo_owner":"attacker","repo_name":"evil"}`),
	},
	{
		name: "delete", method: http.MethodDelete,
		path: func(ws string) string { return "/api/v1/workflow-sync/config?workspace_id=" + ws },
	},
	{
		name: "sync", method: http.MethodPost,
		path: func(ws string) string { return "/api/v1/workflow-sync/sync?workspace_id=" + ws },
	},
}

func doOp(t *testing.T, router *gin.Engine, op configOp, workspaceID string, identity authn.Identity) *httptest.ResponseRecorder {
	t.Helper()
	var body *bytes.Reader
	if op.body != nil {
		body = bytes.NewReader(op.body)
	} else {
		body = bytes.NewReader(nil)
	}
	req := withIdentity(httptest.NewRequest(op.method, op.path(workspaceID), body), identity)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

// newOwnedWorkspace builds a fresh service + router with one configured
// workspace and the given owner wired as the sole authorized identity. Each
// op-level test gets its own instance so DELETE/POST attempts in one subtest
// can't affect another's assertions.
func newOwnedWorkspace(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	svc, _ := setupTestService(t, seededMockClient())
	configureWorkspace(t, svc, victimWorkspace)
	svc.SetWorkspaceAuthorizer(ownerOnlyAuthorizer(victimOwnerID, victimWorkspace))
	return newTestRouter(t, svc), victimWorkspace
}

func TestHTTPHandlers_OwnerSucceedsOnAllRoutes(t *testing.T) {
	owner := authn.Identity{UserID: victimOwnerID, Role: authn.RoleMember}
	for _, op := range allConfigOps {
		t.Run(op.name, func(t *testing.T) {
			router, ws := newOwnedWorkspace(t)
			resp := doOp(t, router, op, ws, owner)
			assert.Equal(t, http.StatusOK, resp.Code, "response body: %s", resp.Body.String())
		})
	}
}

func TestHTTPHandlers_SyntheticIdentitySucceedsOnAllRoutes(t *testing.T) {
	synthetic := authn.Identity{UserID: "single-user", Role: authn.RoleAdmin, Synthetic: true}
	for _, op := range allConfigOps {
		t.Run(op.name, func(t *testing.T) {
			router, ws := newOwnedWorkspace(t)
			resp := doOp(t, router, op, ws, synthetic)
			assert.Equal(t, http.StatusOK, resp.Code, "response body: %s", resp.Body.String())
		})
	}
}

// @covers AC-INTEGRATIONS-GITHUB-RATE-004.1
// @covers AC-INTEGRATIONS-GITHUB-RATE-004.2
// @covers AC-INTEGRATIONS-GITHUB-RATE-004.3
func TestHTTPForceSyncReturnsRateLimitDetailsWithFailedOperation(t *testing.T) {
	now := time.Date(2026, 8, 30, 11, 18, 0, 0, time.UTC)
	retryAt := now.Add(2 * time.Minute)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	svc := NewService(
		setupTestStore(t),
		failingGitHubClients{err: &github.GitHubAPIError{
			StatusCode: http.StatusTooManyRequests, Endpoint: "/repos/acme/flows/contents",
			Body: "secondary rate limit", FailureKind: github.FailureSecondaryRateLimit,
			Resource: github.ResourceCore, RetryAt: retryAt,
			RetrySource: github.RetrySourceRetryAfter,
		}},
		nil,
		&fakeApplier{},
		log,
	)
	svc.now = func() time.Time { return now }
	configureWorkspace(t, svc, victimWorkspace)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/workflow-sync/sync?workspace_id="+victimWorkspace,
		nil,
	)
	newTestRouter(t, svc).ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code, "response body: %s", resp.Body.String())
	var body struct {
		Error     string `json:"error"`
		ErrorCode string `json:"error_code"`
		RateLimit struct {
			Kind              string    `json:"kind"`
			Resource          string    `json:"resource"`
			RetryAt           time.Time `json:"retry_at"`
			RetryAfterSeconds int64     `json:"retry_after_seconds"`
			Source            string    `json:"source"`
		} `json:"rate_limit"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	assert.Equal(t, "GitHub operation is rate limited", body.Error)
	assert.Equal(t, "github_rate_limited", body.ErrorCode)
	assert.Equal(t, "secondary_throttle", body.RateLimit.Kind)
	assert.Equal(t, "core", body.RateLimit.Resource)
	assert.Equal(t, retryAt, body.RateLimit.RetryAt)
	assert.Equal(t, int64(120), body.RateLimit.RetryAfterSeconds)
	assert.Equal(t, "retry_after_header", body.RateLimit.Source)
}

// @covers AC-INTEGRATIONS-GITHUB-RATE-004.3
func TestHTTPForceSyncSuccessOmitsRateLimitDetails(t *testing.T) {
	svc, _ := setupTestService(t, seededMockClient())
	configureWorkspace(t, svc, victimWorkspace)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/workflow-sync/sync?workspace_id="+victimWorkspace,
		nil,
	)
	newTestRouter(t, svc).ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code, "response body: %s", resp.Body.String())
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	assert.NotContains(t, body, "error_code")
	assert.NotContains(t, body, "rate_limit")
}

func TestHTTPHandlers_ForeignMemberDeniedOnAllRoutesWithoutLeaking(t *testing.T) {
	svc, applier := setupTestService(t, seededMockClient())
	configureWorkspace(t, svc, victimWorkspace)
	require.NoError(t, updateVictimRepo(t, svc))
	before, err := svc.store.GetConfigForWorkspace(context.Background(), victimWorkspace)
	require.NoError(t, err)
	require.NotNil(t, before)

	svc.SetWorkspaceAuthorizer(ownerOnlyAuthorizer(victimOwnerID, victimWorkspace))
	router := newTestRouter(t, svc)
	attacker := authn.Identity{UserID: attackerID, Role: authn.RoleMember}

	for _, op := range allConfigOps {
		t.Run(op.name, func(t *testing.T) {
			resp := doOp(t, router, op, victimWorkspace, attacker)
			assert.Equal(t, http.StatusNotFound, resp.Code, "response body: %s", resp.Body.String())
			for _, secret := range []string{victimRepoOwner, victimRepoName, victimBranch, victimPath} {
				assert.NotContains(t, resp.Body.String(), secret)
			}
		})
	}

	after, err := svc.store.GetConfigForWorkspace(context.Background(), victimWorkspace)
	require.NoError(t, err)
	require.NotNil(t, after)
	assert.Equal(t, *before, *after, "denied requests must not change any field of the victim's config")
	assert.Empty(t, applier.released, "denied delete must not release synced workflows")
	assert.Zero(t, applier.callCount(), "denied sync must never reach the applier")
}

// TestHTTPHandlers_ForceSyncDeniesSecondLookupAfterAccessRevoked covers the
// narrow race in httpForceSync: SyncWorkspace and the follow-up
// GetConfigForWorkspace (used to build the response) are two separate
// authorization checks. If access is revoked between them — say a workspace
// is deleted, or reassigned, mid-request — the second call must still map to
// a sanitized 404, not fall through to the generic 500 path.
func TestHTTPHandlers_ForceSyncDeniesSecondLookupAfterAccessRevoked(t *testing.T) {
	svc, _ := setupTestService(t, seededMockClient())
	configureWorkspace(t, svc, victimWorkspace)

	calls := 0
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error {
		calls++
		if calls == 1 {
			return nil
		}
		return repoerrors.ErrWorkspaceNotFound
	})
	router := newTestRouter(t, svc)
	owner := authn.Identity{UserID: victimOwnerID, Role: authn.RoleMember}

	req := withIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/workflow-sync/sync?workspace_id="+victimWorkspace, nil), owner)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusNotFound, resp.Code, "response body: %s", resp.Body.String())
	assert.NotContains(t, resp.Body.String(), "acme")
	assert.GreaterOrEqual(t, calls, 2, "test requires both the sync and the follow-up config lookup to run")
}

// updateVictimRepo overwrites the seeded config with distinctive identity
// (repo, branch, and path) so leak assertions aren't relying on the shared
// default fixture values used elsewhere in this package's tests.
func updateVictimRepo(t *testing.T, svc *Service) error {
	t.Helper()
	_, err := svc.store.UpsertConfigForWorkspace(context.Background(), victimWorkspace, &SetConfigRequest{
		RepoOwner:       victimRepoOwner,
		RepoName:        victimRepoName,
		Branch:          victimBranch,
		Path:            victimPath,
		IntervalSeconds: DefaultIntervalSeconds,
	})
	return err
}
