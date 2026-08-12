package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// newConfigRouteFixture wires the config HTTP routes against a store, a fake
// workspace secret store, and a stub GitLab whose /api/v4/user accepts exactly
// one token. The returned server URL is the host to configure.
func newConfigRouteFixture(t *testing.T, validToken string) (*Store, *configTestSecrets, *gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gitlabServer := httptest.NewServer(userHandler(func(token string) bool { return token == validToken }))
	t.Cleanup(gitlabServer.Close)

	store := newTestStore(t)
	seedWorkspace(t, store, "ws-1")
	seedWorkspace(t, store, "ws-2")
	secrets := &configTestSecrets{values: make(map[string]string)}
	svc := NewService(DefaultHost, NewNoopClient(DefaultHost), AuthMethodNone, nil, newTestLogger(t))
	svc.SetStore(store)
	svc.SetWorkspaceSecretStore(secrets)
	router := gin.New()
	NewController(svc, newTestLogger(t)).RegisterHTTPRoutes(router)
	return store, secrets, router, gitlabServer.URL
}

// configureWorkspace saves a PAT connection through the real PUT /config route
// so the follow-up delete/copy tests operate on state the API itself produced.
func configureWorkspace(t *testing.T, router *gin.Engine, workspaceID, host, token string) {
	t.Helper()
	recorder := watchScopeRequest(t, router, http.MethodPut,
		"/api/v1/gitlab/config?workspace_id="+workspaceID,
		map[string]string{"host": host, "auth_method": AuthMethodPAT, "token": token})
	if recorder.Code != http.StatusOK {
		t.Fatalf("configure %s: status = %d, body = %s", workspaceID, recorder.Code, recorder.Body.String())
	}
}

// --- DELETE /config ---

func TestHTTPDeleteConfigRemovesRowAndSecret(t *testing.T) {
	store, secrets, router, host := newConfigRouteFixture(t, "good-token")
	configureWorkspace(t, router, "ws-1", host, "good-token")
	if _, ok := secrets.values[SecretKeyForWorkspace("ws-1")]; !ok {
		t.Fatal("fixture did not store a workspace secret to begin with")
	}

	var body map[string]bool
	recorder := httpJSON(t, router, http.MethodDelete,
		"/api/v1/gitlab/config?workspace_id=ws-1", nil, &body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if !body[respKeyDeleted] {
		t.Errorf("payload = %v, want {\"deleted\":true}", body)
	}
	cfg, err := store.GetConfigForWorkspace(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if cfg != nil {
		t.Errorf("config = %+v, want the row removed", cfg)
	}
	if _, ok := secrets.values[SecretKeyForWorkspace("ws-1")]; ok {
		t.Error("the workspace token secret survived the connection delete")
	}
}

// TestHTTPDeleteConfigRequiresWorkspaceQuery covers writeConfigMutationError's
// ErrWorkspaceRequired branch, which must be a 400 rather than the default 500.
func TestHTTPDeleteConfigRequiresWorkspaceQuery(t *testing.T) {
	_, _, router, _ := newConfigRouteFixture(t, "good-token")

	recorder := watchScopeRequest(t, router, http.MethodDelete, "/api/v1/gitlab/config", nil)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", recorder.Code, recorder.Body.String())
	}
	if got := httpErrorMessage(t, recorder); got != "workspace_id query parameter required" {
		t.Errorf("error = %q, want the workspace_id requirement", got)
	}
}

// --- POST /config/test ---

func TestHTTPTestConfigReportsSuccessWithoutPersisting(t *testing.T) {
	store, secrets, router, host := newConfigRouteFixture(t, "good-token")

	var result TestConnectionResult
	recorder := httpJSON(t, router, http.MethodPost,
		"/api/v1/gitlab/config/test?workspace_id=ws-1",
		map[string]string{"host": host, "auth_method": AuthMethodPAT, "token": "good-token"}, &result)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if result.Error != "" {
		t.Fatalf("error = %q, want a successful probe", result.Error)
	}
	if result.Username != "alice" {
		t.Errorf("username = %q, want alice from the stub", result.Username)
	}
	cfg, err := store.GetConfigForWorkspace(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if cfg != nil {
		t.Errorf("config = %+v, want nothing persisted by a test probe", cfg)
	}
	if len(secrets.values) != 0 {
		t.Errorf("secrets = %v, want nothing persisted by a test probe", secrets.values)
	}
}

// TestHTTPTestConfigReportsFailureAsA200WithError pins the contract the
// settings UI depends on: a rejected credential is a *result*, not an HTTP
// error, so the form can render the message inline.
func TestHTTPTestConfigReportsFailureAsA200WithError(t *testing.T) {
	_, _, router, host := newConfigRouteFixture(t, "good-token")

	var result TestConnectionResult
	recorder := httpJSON(t, router, http.MethodPost,
		"/api/v1/gitlab/config/test?workspace_id=ws-1",
		map[string]string{"host": host, "auth_method": AuthMethodPAT, "token": "wrong-token"}, &result)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 even for a failed probe (body %s)",
			recorder.Code, recorder.Body.String())
	}
	if result.Error == "" {
		t.Error("error is empty; a rejected token must be reported in the result")
	}
	if result.Username != "" {
		t.Errorf("username = %q, want empty for a failed probe", result.Username)
	}
}

func TestHTTPTestConfigRejectsInvalidPayload(t *testing.T) {
	_, _, router, _ := newConfigRouteFixture(t, "good-token")

	recorder := watchScopeRequest(t, router, http.MethodPost,
		"/api/v1/gitlab/config/test?workspace_id=ws-1", []string{"not", "an", "object"})

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", recorder.Code, recorder.Body.String())
	}
	if got := httpErrorMessage(t, recorder); got != "invalid payload" {
		t.Errorf("error = %q, want \"invalid payload\"", got)
	}
}

// --- POST /config/copy ---

func TestHTTPCopyConfigDuplicatesConnectionToTargetWorkspace(t *testing.T) {
	store, secrets, router, host := newConfigRouteFixture(t, "good-token")
	configureWorkspace(t, router, "ws-1", host, "good-token")

	var copied GitLabConfig
	recorder := httpJSON(t, router, http.MethodPost,
		"/api/v1/gitlab/config/copy?workspace_id=ws-1",
		map[string]string{"targetWorkspaceId": "ws-2"}, &copied)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	target, err := store.GetConfigForWorkspace(context.Background(), "ws-2")
	if err != nil || target == nil {
		t.Fatalf("target config = %+v, err = %v", target, err)
	}
	if target.Host != host {
		t.Errorf("target host = %q, want %q", target.Host, host)
	}
	if secrets.values[SecretKeyForWorkspace("ws-2")] != "good-token" {
		t.Errorf("target secret = %q, want the source token copied under the target's own key",
			secrets.values[SecretKeyForWorkspace("ws-2")])
	}
	if secrets.values[SecretKeyForWorkspace("ws-1")] != "good-token" {
		t.Error("the source workspace's secret was disturbed by the copy")
	}
}

func TestHTTPCopyConfigRequiresTargetWorkspace(t *testing.T) {
	_, _, router, host := newConfigRouteFixture(t, "good-token")
	configureWorkspace(t, router, "ws-1", host, "good-token")

	for name, body := range map[string]interface{}{
		"absent": map[string]string{},
		"blank":  map[string]string{"targetWorkspaceId": "   "},
	} {
		t.Run(name, func(t *testing.T) {
			recorder := watchScopeRequest(t, router, http.MethodPost,
				"/api/v1/gitlab/config/copy?workspace_id=ws-1", body)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", recorder.Code, recorder.Body.String())
			}
			if got := httpErrorMessage(t, recorder); got != "targetWorkspaceId required" {
				t.Errorf("error = %q, want \"targetWorkspaceId required\"", got)
			}
		})
	}
}

// TestHTTPCopyConfigRejectsCopyingAWorkspaceOntoItself covers the
// ErrSameWorkspace branch of writeConfigMutationError.
func TestHTTPCopyConfigRejectsCopyingAWorkspaceOntoItself(t *testing.T) {
	_, _, router, host := newConfigRouteFixture(t, "good-token")
	configureWorkspace(t, router, "ws-1", host, "good-token")

	recorder := watchScopeRequest(t, router, http.MethodPost,
		"/api/v1/gitlab/config/copy?workspace_id=ws-1",
		map[string]string{"targetWorkspaceId": "ws-1"})

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", recorder.Code, recorder.Body.String())
	}
	if got := httpErrorMessage(t, recorder); got != "source and target workspace must differ" {
		t.Errorf("error = %q, want the same-workspace rejection", got)
	}
}

// TestHTTPCopyConfigRejectsUnconfiguredSource covers the ErrNothingToCopy
// branch: copying from a workspace with no connection is a client error, and
// the target must be left untouched.
func TestHTTPCopyConfigRejectsUnconfiguredSource(t *testing.T) {
	store, _, router, _ := newConfigRouteFixture(t, "good-token")

	recorder := watchScopeRequest(t, router, http.MethodPost,
		"/api/v1/gitlab/config/copy?workspace_id=ws-1",
		map[string]string{"targetWorkspaceId": "ws-2"})

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", recorder.Code, recorder.Body.String())
	}
	if got := httpErrorMessage(t, recorder); got != "source workspace has no GitLab connection" {
		t.Errorf("error = %q, want the nothing-to-copy rejection", got)
	}
	target, err := store.GetConfigForWorkspace(context.Background(), "ws-2")
	if err != nil {
		t.Fatalf("reload target config: %v", err)
	}
	if target != nil {
		t.Errorf("target config = %+v, want it left unconfigured", target)
	}
}
