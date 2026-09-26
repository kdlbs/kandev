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

func TestHTTPForceSyncSanitizesProviderResponseBodyFromSyncAndConfigResponses(t *testing.T) {
	const providerBodyMarker = "provider-response-body-must-not-leak"
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	svc := NewService(
		setupTestStore(t),
		failingGitHubClients{err: &github.GitHubAPIError{
			StatusCode:  http.StatusBadGateway,
			Endpoint:    "/repos/acme/flows/contents",
			Body:        providerBodyMarker,
			FailureKind: github.FailureTransient,
		}},
		nil,
		&fakeApplier{},
		log,
	)
	configureWorkspace(t, svc, victimWorkspace)
	router := newTestRouter(t, svc)

	syncResponse := doJSON(t, router, http.MethodPost, "/api/v1/workflow-sync/sync?workspace_id="+victimWorkspace, nil)
	require.Equal(t, http.StatusOK, syncResponse.Code)
	assert.NotContains(t, syncResponse.Body.String(), providerBodyMarker)

	configResponse := doJSON(t, router, http.MethodGet, "/api/v1/workflow-sync/config?workspace_id="+victimWorkspace, nil)
	require.Equal(t, http.StatusOK, configResponse.Code)
	assert.NotContains(t, configResponse.Body.String(), providerBodyMarker)

	var config Config
	require.NoError(t, json.Unmarshal(configResponse.Body.Bytes(), &config))
	assert.False(t, config.LastOk)
	assert.Equal(t, "GitHub request failed with HTTP status 502", config.LastError)
	assert.Equal(t, string(github.FailureTransient), config.LastErrorClass)
	assert.Equal(t, 1, config.ConsecutiveFailures)
	assert.NotNil(t, config.NextRetryAt)
}

func TestHTTPHandlersSanitizeLegacyStoredProviderErrorFromConfigAndSync(t *testing.T) {
	const providerBodyMarker = "legacy-provider-private-qa-marker"
	retryAt := time.Date(2026, 9, 25, 18, 0, 0, 0, time.UTC)
	deferred := &github.AdmissionDeferredError{Reason: "background admission deferred"}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	store := setupTestStore(t)
	svc := NewService(store, failingGitHubClients{err: deferred}, nil, &fakeApplier{}, log)
	configureWorkspace(t, svc, victimWorkspace)
	_, err = store.db.Exec(`
		UPDATE workflow_sync_configs
		SET last_ok = 0, last_error = ?, failure_class = ?, consecutive_failures = 3,
			next_retry_at = ?, last_error_class = ?
		WHERE workspace_id = ?
	`, "github API error: "+providerBodyMarker, "transient", retryAt, "transient", victimWorkspace)
	require.NoError(t, err)
	router := newTestRouter(t, svc)

	configResponse := doJSON(t, router, http.MethodGet, "/api/v1/workflow-sync/config?workspace_id="+victimWorkspace, nil)
	require.Equal(t, http.StatusOK, configResponse.Code)
	assert.NotContains(t, configResponse.Body.String(), providerBodyMarker)
	var configBody Config
	require.NoError(t, json.Unmarshal(configResponse.Body.Bytes(), &configBody))
	assert.Equal(t, "Workflow sync failed", configBody.LastError)
	assert.Equal(t, "transient", string(configBody.FailureClass))
	assert.Equal(t, "transient", configBody.LastErrorClass)
	assert.Equal(t, 3, configBody.ConsecutiveFailures)
	require.NotNil(t, configBody.NextRetryAt)
	assert.Equal(t, retryAt, *configBody.NextRetryAt)

	syncResponse := doJSON(t, router, http.MethodPost, "/api/v1/workflow-sync/sync?workspace_id="+victimWorkspace, nil)
	require.Equal(t, http.StatusOK, syncResponse.Code)
	assert.NotContains(t, syncResponse.Body.String(), providerBodyMarker)
}

func TestHTTPHandlersSanitizeSuspensionReasons(t *testing.T) {
	const providerBodyMarker = "suspension-reason-provider-body-must-not-leak"
	legacyRetryAt := time.Date(2026, 9, 26, 1, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name             string
		providerErr      error
		legacy           bool
		wantFailureClass string
		wantErrorClass   string
		wantReason       string
		wantRetryAt      *time.Time
	}{
		{
			name: "missing resource",
			providerErr: &github.GitHubAPIError{
				StatusCode: http.StatusNotFound, Body: providerBodyMarker,
				FailureKind: github.FailureMissingResource,
			},
			wantFailureClass: "config",
			wantErrorClass:   string(github.FailureMissingResource),
			wantReason:       "GitHub request failed with HTTP status 404",
		},
		{
			name: "invalid credentials",
			providerErr: &github.GitHubAPIError{
				StatusCode: http.StatusUnauthorized, Body: providerBodyMarker,
				FailureKind: github.FailureInvalidCredentials,
			},
			wantFailureClass: "auth",
			wantErrorClass:   string(github.FailureInvalidCredentials),
			wantReason:       "GitHub request failed with HTTP status 401",
		},
		{
			name:             "legacy stored reason",
			providerErr:      &github.AdmissionDeferredError{Reason: "background admission deferred"},
			legacy:           true,
			wantFailureClass: "config",
			wantErrorClass:   string(github.FailureMissingResource),
			wantReason:       genericSyncFailureMessage,
			wantRetryAt:      &legacyRetryAt,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
			require.NoError(t, err)
			store := setupTestStore(t)
			svc := NewService(store, failingGitHubClients{err: tc.providerErr}, nil, &fakeApplier{}, log)
			configureWorkspace(t, svc, victimWorkspace)
			if tc.legacy {
				_, err = store.db.Exec(`
					UPDATE workflow_sync_configs
					SET last_ok = 0, failure_class = ?, last_error_class = ?, poll_suspended = 1,
						poll_suspension_reason = ?, consecutive_failures = 1, next_retry_at = ?
					WHERE workspace_id = ?
				`, tc.wantFailureClass, tc.wantErrorClass, "github API error: "+providerBodyMarker, tc.wantRetryAt, victimWorkspace)
				require.NoError(t, err)
			}
			router := newTestRouter(t, svc)

			if !tc.legacy {
				syncResponse := doJSON(t, router, http.MethodPost, "/api/v1/workflow-sync/sync?workspace_id="+victimWorkspace, nil)
				require.Equal(t, http.StatusOK, syncResponse.Code)
				assert.NotContains(t, syncResponse.Body.String(), providerBodyMarker)
			}

			configResponse := doJSON(t, router, http.MethodGet, "/api/v1/workflow-sync/config?workspace_id="+victimWorkspace, nil)
			require.Equal(t, http.StatusOK, configResponse.Code)
			assert.NotContains(t, configResponse.Body.String(), providerBodyMarker)
			var config Config
			require.NoError(t, json.Unmarshal(configResponse.Body.Bytes(), &config))
			assert.True(t, config.PollSuspended)
			assert.Equal(t, tc.wantFailureClass, string(config.FailureClass))
			assert.Equal(t, tc.wantErrorClass, config.LastErrorClass)
			assert.Equal(t, tc.wantReason, config.PollSuspensionReason)
			assert.Equal(t, 1, config.ConsecutiveFailures)
			if tc.wantRetryAt == nil {
				assert.Nil(t, config.NextRetryAt)
			} else {
				require.NotNil(t, config.NextRetryAt)
				assert.Equal(t, *tc.wantRetryAt, *config.NextRetryAt)
			}
		})
	}
}

func TestHTTPForceSyncShowsActionableGitHubConnectionFailure(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	svc := NewService(setupTestStore(t), nil, nil, &fakeApplier{}, log)
	configureWorkspace(t, svc, victimWorkspace)
	router := newTestRouter(t, svc)

	syncResponse := doJSON(t, router, http.MethodPost, "/api/v1/workflow-sync/sync?workspace_id="+victimWorkspace, nil)
	require.Equal(t, http.StatusOK, syncResponse.Code)
	assert.Contains(t, syncResponse.Body.String(), githubConnectionFailureMessage)
	configResponse := doJSON(t, router, http.MethodGet, "/api/v1/workflow-sync/config?workspace_id="+victimWorkspace, nil)
	require.Equal(t, http.StatusOK, configResponse.Code)
	var config Config
	require.NoError(t, json.Unmarshal(configResponse.Body.Bytes(), &config))
	assert.True(t, config.PollSuspended)
	assert.Equal(t, string(github.FailureInvalidCredentials), config.LastErrorClass)
	assert.Equal(t, githubConnectionFailureMessage, config.LastError)
	assert.Equal(t, githubConnectionFailureMessage, config.PollSuspensionReason)
	assert.Nil(t, config.NextRetryAt)
}

func TestHTTPForceSyncReturnsRateLimitDetailsWhenAdmissionWaitIsCanceled(t *testing.T) {
	now := time.Date(2026, 8, 30, 11, 18, 0, 0, time.UTC)
	retryAt := now.Add(2 * time.Minute)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	svc := NewService(
		setupTestStore(t),
		failingGitHubClients{err: &github.AdmissionWaitError{
			Resource: github.ResourceCore, RetryAt: retryAt,
			RetrySource: github.RetrySourceRetryAfter,
			Reason:      "observed_secondary_rate_limit", Cause: context.Canceled,
		}},
		nil, &fakeApplier{}, log,
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
			Kind     string    `json:"kind"`
			Resource string    `json:"resource"`
			RetryAt  time.Time `json:"retry_at"`
			Source   string    `json:"source"`
		} `json:"rate_limit"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	assert.Equal(t, "GitHub operation is rate limited", body.Error)
	assert.Equal(t, "github_rate_limited", body.ErrorCode)
	assert.Equal(t, "secondary_throttle", body.RateLimit.Kind)
	assert.Equal(t, "core", body.RateLimit.Resource)
	assert.Equal(t, retryAt, body.RateLimit.RetryAt)
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
