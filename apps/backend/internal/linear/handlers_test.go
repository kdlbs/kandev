package linear

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestSplitCSV(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", []string{}},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{"a, b , c", []string{"a", "b", "c"}}, // surrounding spaces trimmed
		{"a,,b", []string{"a", "b"}},          // empty segment dropped
		{",a,b,", []string{"a", "b"}},         // leading/trailing commas dropped
		{"   ,   ", []string{}},               // whitespace-only segments dropped
	}
	for _, tc := range cases {
		got := splitCSV(tc.in)
		// splitCSV may return a non-nil empty slice; normalise to a length
		// comparison so a `nil` vs `[]string{}` mismatch doesn't fail the test.
		if len(got) == 0 && len(tc.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitCSV(%q) = %#v, want %#v", tc.in, got, tc.want)
		}
	}
}

func newTestController(t *testing.T) (*Controller, *gin.Engine, *fakeClient) {
	t.Helper()
	store := newTestStore(t)
	secrets := newFakeSecretStore()
	client := &fakeClient{}
	svc := NewService(store, secrets, func(_ *LinearConfig, _ string) Client {
		return client
	}, logger.Default())
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ctrl := &Controller{service: svc, logger: logger.Default()}
	ctrl.RegisterHTTPRoutes(router)
	return ctrl, router, client
}

func TestWriteClientError_NotConfigured_Returns503WithCode(t *testing.T) {
	ctrl, _, _ := newTestController(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	ctrl.writeClientError(c, ErrNotConfigured)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
	body := w.Body.String()
	// errCodeLinearNotConfigured is private — assert on the wire constant.
	if !strings.Contains(body, `"code":"LINEAR_NOT_CONFIGURED"`) {
		t.Errorf("body does not surface code: %s", body)
	}
}

func TestWriteClientError_APIError_PassesThroughKnownStatuses(t *testing.T) {
	cases := []struct {
		upstream int
		want     int
	}{
		{http.StatusUnauthorized, http.StatusUnauthorized},
		{http.StatusForbidden, http.StatusForbidden},
		{http.StatusNotFound, http.StatusNotFound},
		{http.StatusBadRequest, http.StatusBadRequest},
		{http.StatusBadGateway, http.StatusInternalServerError}, // unmapped → 500
	}
	ctrl, _, _ := newTestController(t)
	for _, tc := range cases {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		ctrl.writeClientError(c, &APIError{StatusCode: tc.upstream, Message: "msg"})
		if w.Code != tc.want {
			t.Errorf("upstream %d → got status %d, want %d", tc.upstream, w.Code, tc.want)
		}
	}
}

func TestWriteClientError_GenericError_Returns500(t *testing.T) {
	ctrl, _, _ := newTestController(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	ctrl.writeClientError(c, errors.New("boom"))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestHTTPGetConfig_NoConfig_Returns204(t *testing.T) {
	_, router, _ := newTestController(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/linear/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}
}

func TestHTTPListStates_RequiresTeamKey(t *testing.T) {
	_, router, _ := newTestController(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/linear/states", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHTTPListStates_RoutesThroughService(t *testing.T) {
	ctrl, router, client := newTestController(t)
	ctx := context.Background()
	if err := ctrl.service.store.UpsertConfig(ctx, &LinearConfig{
		AuthMethod: AuthMethodAPIKey,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := ctrl.service.secrets.Set(ctx, SecretKeyForWorkspace("default"), "linear", "tok"); err != nil {
		t.Fatalf("set secret: %v", err)
	}
	var seenTeam string
	client.listStatesFn = func(teamKey string) ([]LinearWorkflowState, error) {
		seenTeam = teamKey
		return []LinearWorkflowState{{ID: "s1", Name: "In Progress"}}, nil
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/linear/states?team_key=ENG", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if seenTeam != "ENG" {
		t.Errorf("team key forwarded to client = %q, want ENG", seenTeam)
	}
}

func TestHTTPListLabels_RequiresTeamKey(t *testing.T) {
	_, router, _ := newTestController(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/linear/labels", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHTTPListLabels_RoutesThroughService(t *testing.T) {
	ctrl, router, client := newTestController(t)
	ctx := context.Background()
	if err := ctrl.service.store.UpsertConfig(ctx, &LinearConfig{
		AuthMethod: AuthMethodAPIKey,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := ctrl.service.secrets.Set(ctx, SecretKeyForWorkspace("default"), "linear", "tok"); err != nil {
		t.Fatalf("set secret: %v", err)
	}
	var seenTeam string
	client.listLabelsFn = func(teamKey string) ([]LinearLabel, error) {
		seenTeam = teamKey
		return []LinearLabel{{ID: "l1", Name: "bug"}}, nil
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/linear/labels?team_key=ENG", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if seenTeam != "ENG" {
		t.Errorf("team key forwarded to client = %q, want ENG", seenTeam)
	}
}

func TestHTTPListUsers_RequiresTeamKey(t *testing.T) {
	_, router, _ := newTestController(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/linear/users", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHTTPListUsers_RoutesThroughService(t *testing.T) {
	ctrl, router, client := newTestController(t)
	ctx := context.Background()
	if err := ctrl.service.store.UpsertConfig(ctx, &LinearConfig{
		AuthMethod: AuthMethodAPIKey,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := ctrl.service.secrets.Set(ctx, SecretKeyForWorkspace("default"), "linear", "tok"); err != nil {
		t.Fatalf("set secret: %v", err)
	}
	var seenTeam string
	client.listUsersFn = func(teamKey string) ([]LinearUser, error) {
		seenTeam = teamKey
		return []LinearUser{{ID: "u1", Name: "Alice"}}, nil
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/linear/users?team_key=ENG", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if seenTeam != "ENG" {
		t.Errorf("team key forwarded to client = %q, want ENG", seenTeam)
	}
}

func TestHTTPSearchIssues_RejectsBadNumericParams(t *testing.T) {
	ctrl, router, _ := newTestController(t)
	ctx := context.Background()
	if err := ctrl.service.store.UpsertConfig(ctx, &LinearConfig{
		AuthMethod: AuthMethodAPIKey,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := ctrl.service.secrets.Set(ctx, SecretKeyForWorkspace("default"), "linear", "tok"); err != nil {
		t.Fatalf("set secret: %v", err)
	}
	cases := map[string]string{
		"priorities out of range":        "priorities=99",
		"priorities mixed valid/invalid": "priorities=1,99",
		"priorities not a number":        "priorities=abc",
		"estimate_min not a number":      "estimate_min=abc",
		"estimate_min NaN":               "estimate_min=NaN",
		"estimate_min +Inf":              "estimate_min=%2BInf",
		"estimate_max -Inf":              "estimate_max=-Inf",
		"estimate_min negative":          "estimate_min=-1",
		"estimate_min > max":             "estimate_min=5&estimate_max=1",
	}
	for name, query := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/linear/issues?"+query, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400; body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestRegisterRoutes_RegistersWSHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	dispatcher := ws.NewDispatcher()
	store := newTestStore(t)
	svc := NewService(store, newFakeSecretStore(), func(_ *LinearConfig, _ string) Client {
		return &fakeClient{}
	}, logger.Default())

	RegisterRoutes(router, dispatcher, svc, logger.Default())

	for _, action := range []string{
		ws.ActionLinearConfigGet,
		ws.ActionLinearConfigSet,
		ws.ActionLinearConfigDelete,
		ws.ActionLinearConfigTest,
		ws.ActionLinearIssueGet,
		ws.ActionLinearIssueTransition,
		ws.ActionLinearTeamsList,
	} {
		if !dispatcher.HasHandler(action) {
			t.Errorf("WS action %q not registered", action)
		}
	}
}

func TestHTTPListProjects_RequiresTeamKey(t *testing.T) {
	_, router, _ := newTestController(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/linear/projects", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// @covers AC-INTEGRATIONS-LINEAR-PROJECTS-001.3
func TestHTTPListProjects_RoutesThroughService(t *testing.T) {
	ctrl, router, client := newTestController(t)
	ctx := context.Background()
	if err := ctrl.service.store.UpsertConfig(ctx, &LinearConfig{
		AuthMethod: AuthMethodAPIKey,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := ctrl.service.secrets.Set(ctx, SecretKeyForWorkspace("default"), "linear", "tok"); err != nil {
		t.Fatalf("set secret: %v", err)
	}
	var seenTeam string
	client.listProjectsFn = func(teamKey string) ([]LinearProject, error) {
		seenTeam = teamKey
		return []LinearProject{{ID: "p1", Name: "Checkout"}}, nil
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/linear/projects?team_key=ENG", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if seenTeam != "ENG" {
		t.Errorf("team key forwarded to client = %q, want ENG", seenTeam)
	}
	if !strings.Contains(w.Body.String(), `"projects":[{"id":"p1","name":"Checkout"}]`) {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestHTTPIssueDetailAndStateUseRequestedWorkspace(t *testing.T) {
	ctrl, router, _ := newTestController(t)
	ctx := context.Background()
	clients := map[string]*fakeClient{
		"default-token":     {},
		"workspace-a-token": {},
		"workspace-b-token": {},
	}
	for workspaceID, token := range map[string]string{
		"default":     "default-token",
		"workspace-a": "workspace-a-token",
		"workspace-b": "workspace-b-token",
	} {
		if err := ctrl.service.store.UpsertConfigForWorkspace(ctx, workspaceID, &LinearConfig{
			AuthMethod: AuthMethodAPIKey,
		}); err != nil {
			t.Fatalf("upsert %s config: %v", workspaceID, err)
		}
		if err := ctrl.service.secrets.Set(ctx, SecretKeyForWorkspace(workspaceID), "Linear API key", token); err != nil {
			t.Fatalf("set %s secret: %v", workspaceID, err)
		}
	}
	clients["default-token"].getIssueFn = func(identifier string) (*LinearIssue, error) {
		return &LinearIssue{Identifier: identifier, Title: "Default workspace"}, nil
	}
	clients["workspace-a-token"].getIssueFn = func(identifier string) (*LinearIssue, error) {
		return &LinearIssue{Identifier: identifier, Title: "Workspace A"}, nil
	}
	clients["workspace-b-token"].getIssueFn = func(identifier string) (*LinearIssue, error) {
		return &LinearIssue{Identifier: identifier, Title: "Workspace B"}, nil
	}
	ctrl.service.clientFn = func(_ *LinearConfig, secret string) Client {
		return clients[secret]
	}
	allowWorkspaceBWrite := false
	ctrl.service.SetWorkspaceWriteAuthorizer(func(_ context.Context, workspaceID string) error {
		if workspaceID == "workspace-b" && !allowWorkspaceBWrite {
			return ErrWorkspaceWriteForbidden
		}
		return nil
	})

	get := httptest.NewRequest(http.MethodGet, "/api/v1/linear/issues/REL-2173?workspace_id=workspace-b", nil)
	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, get)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200; body=%s", getResponse.Code, getResponse.Body.String())
	}
	if !strings.Contains(getResponse.Body.String(), `"title":"Workspace B"`) {
		t.Fatalf("GET body = %s, want issue from workspace B", getResponse.Body.String())
	}

	state := httptest.NewRequest(http.MethodPost, "/api/v1/linear/issues/issue-b/state?workspace_id=workspace-b", strings.NewReader(`{"stateId":"done"}`))
	state.Header.Set("Content-Type", "application/json")
	stateResponse := httptest.NewRecorder()
	router.ServeHTTP(stateResponse, state)
	if stateResponse.Code != http.StatusForbidden {
		t.Fatalf("viewer POST status = %d, want 403; body=%s", stateResponse.Code, stateResponse.Body.String())
	}
	if len(clients["workspace-b-token"].transitionLog) != 0 {
		t.Fatalf("viewer state change reached Linear client: %v", clients["workspace-b-token"].transitionLog)
	}

	allowWorkspaceBWrite = true
	state = httptest.NewRequest(http.MethodPost, "/api/v1/linear/issues/issue-b/state?workspace_id=workspace-b", strings.NewReader(`{"stateId":"done"}`))
	state.Header.Set("Content-Type", "application/json")
	stateResponse = httptest.NewRecorder()
	router.ServeHTTP(stateResponse, state)
	if stateResponse.Code != http.StatusOK {
		t.Fatalf("collaborator POST status = %d, want 200; body=%s", stateResponse.Code, stateResponse.Body.String())
	}
	if got := clients["workspace-b-token"].transitionLog; len(got) != 1 || got[0] != "issue-b:done" {
		t.Fatalf("workspace B transitions = %v, want [issue-b:done]", got)
	}
	if len(clients["default-token"].transitionLog) != 0 {
		t.Fatalf("default workspace received transitions: %v", clients["default-token"].transitionLog)
	}

	ctrl.service.SetWorkspaceAuthorizer(denyOnly("workspace-b"))
	deniedGet := httptest.NewRecorder()
	router.ServeHTTP(deniedGet, httptest.NewRequest(http.MethodGet, "/api/v1/linear/issues/REL-2173?workspace_id=workspace-b", nil))
	if deniedGet.Code != http.StatusNotFound {
		t.Fatalf("unauthorized GET status = %d, want 404", deniedGet.Code)
	}
	deniedState := httptest.NewRequest(http.MethodPost, "/api/v1/linear/issues/issue-b/state?workspace_id=workspace-b", strings.NewReader(`{"stateId":"blocked"}`))
	deniedState.Header.Set("Content-Type", "application/json")
	deniedStateResponse := httptest.NewRecorder()
	router.ServeHTTP(deniedStateResponse, deniedState)
	if deniedStateResponse.Code != http.StatusNotFound {
		t.Fatalf("unauthorized POST status = %d, want 404", deniedStateResponse.Code)
	}
	if got := clients["workspace-b-token"].transitionLog; len(got) != 1 {
		t.Fatalf("unauthorized state change reached Linear client: %v", got)
	}
}

func TestWSIssueStateWriteRequiresWorkspaceWriteScope(t *testing.T) {
	ctrl, _, client := newTestController(t)
	var authorizedWorkspace string
	ctrl.service.SetWorkspaceWriteAuthorizer(func(_ context.Context, workspaceID string) error {
		authorizedWorkspace = workspaceID
		return ErrWorkspaceWriteForbidden
	})
	request, err := ws.NewRequest("request-1", ws.ActionLinearIssueTransition, map[string]string{
		"issueId": "issue-default",
		"stateId": "done",
	})
	if err != nil {
		t.Fatalf("create WebSocket request: %v", err)
	}

	response, err := wsSetIssueState(ctrl.service)(context.Background(), request)
	if err != nil {
		t.Fatalf("handle WebSocket request: %v", err)
	}
	if response.Type != ws.MessageTypeError {
		t.Fatalf("response type = %q, want error", response.Type)
	}
	var payload ws.ErrorPayload
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("decode WebSocket error: %v", err)
	}
	if payload.Code != ws.ErrorCodeForbidden {
		t.Fatalf("error code = %q, want %q", payload.Code, ws.ErrorCodeForbidden)
	}
	if authorizedWorkspace != "default" {
		t.Fatalf("authorized workspace = %q, want default", authorizedWorkspace)
	}
	if len(client.transitionLog) != 0 {
		t.Fatalf("denied WebSocket state change reached Linear client: %v", client.transitionLog)
	}
}
