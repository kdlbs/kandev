package sentry

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func newTestController(t *testing.T) (*Controller, *gin.Engine, *fakeClient) {
	t.Helper()
	store := newTestStore(t)
	secrets := newFakeSecretStore()
	client := &fakeClient{}
	svc := NewService(store, secrets, func(_ *SentryConfig, _ string) Client {
		return client
	}, logger.Default())
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ctrl := &Controller{service: svc, logger: logger.Default()}
	ctrl.RegisterHTTPRoutes(router)
	return ctrl, router, client
}

// seedInstance persists an instance (+ optional secret) directly via the store.
func seedInstance(t *testing.T, ctrl *Controller, workspaceID, name, secret string) *SentryConfig {
	t.Helper()
	ctx := context.Background()
	cfg := &SentryConfig{WorkspaceID: workspaceID, Name: name, AuthMethod: AuthMethodAuthToken, URL: DefaultSentryURL}
	if err := ctrl.service.store.CreateInstance(ctx, cfg); err != nil {
		t.Fatalf("seed instance: %v", err)
	}
	if secret != "" {
		if err := ctrl.service.secrets.Set(ctx, secretKeyForInstance(cfg.ID), "sentry", secret); err != nil {
			t.Fatalf("seed secret: %v", err)
		}
	}
	return cfg
}

func do(router *gin.Engine, method, target, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestHTTP_ListInstances_RequiresWorkspace(t *testing.T) {
	_, router, _ := newTestController(t)
	if w := do(router, http.MethodGet, "/api/v1/sentry/instances", ""); w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 without workspace_id, got %d", w.Code)
	}
}

func TestHTTP_ListInstances_Wrapped(t *testing.T) {
	ctrl, router, _ := newTestController(t)
	seedInstance(t, ctrl, "ws-1", "A", "tok")
	seedInstance(t, ctrl, "ws-2", "B", "tok")
	w := do(router, http.MethodGet, "/api/v1/sentry/instances?workspace_id=ws-1", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Instances []SentryConfig `json:"instances"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Instances) != 1 || resp.Instances[0].Name != "A" {
		t.Errorf("expected only ws-1's instance, got %+v", resp.Instances)
	}
}

func TestHTTP_CreateInstance_MismatchedWorkspaceRejected(t *testing.T) {
	_, router, _ := newTestController(t)
	// Body workspaceId mismatches the query → 400 (acceptance e).
	body := `{"workspaceId":"ws-2","name":"A","secret":"t"}`
	if w := do(router, http.MethodPost, "/api/v1/sentry/instances?workspace_id=ws-1", body); w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for mismatched workspaceId, got %d body=%s", w.Code, w.Body.String())
	}
	// Missing workspace_id query → 400.
	if w := do(router, http.MethodPost, "/api/v1/sentry/instances", `{"name":"A"}`); w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 without workspace_id, got %d", w.Code)
	}
}

func TestHTTP_CreateInstance_OK(t *testing.T) {
	ctrl, router, _ := newTestController(t)
	probed := make(chan struct{}, 4)
	ctrl.service.SetProbeHook(func() {
		select {
		case probed <- struct{}{}:
		default:
		}
	})
	w := do(router, http.MethodPost, "/api/v1/sentry/instances?workspace_id=ws-1",
		`{"workspaceId":"ws-1","name":"SaaS","secret":"tok"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var cfg SentryConfig
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cfg.ID == "" || cfg.Name != "SaaS" || !cfg.HasSecret {
		t.Errorf("unexpected created instance: %+v", cfg)
	}
	select {
	case <-probed:
	case <-time.After(2 * time.Second):
		t.Fatal("async probe did not fire")
	}
}

func TestHTTP_CreateInstance_BadJSON(t *testing.T) {
	_, router, _ := newTestController(t)
	if w := do(router, http.MethodPost, "/api/v1/sentry/instances?workspace_id=ws-1", "not-json"); w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHTTP_GetInstance_CrossWorkspace404(t *testing.T) {
	ctrl, router, _ := newTestController(t)
	inst := seedInstance(t, ctrl, "ws-1", "A", "tok")
	w := do(router, http.MethodGet, "/api/v1/sentry/instances/"+inst.ID+"?workspace_id=ws-2", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 cross-workspace, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"code":"SENTRY_INSTANCE_NOT_FOUND"`) {
		t.Errorf("missing code in body: %s", w.Body.String())
	}
}

func TestHTTP_UpdateInstance_UpdatesAndRejectsCrossWorkspace(t *testing.T) {
	ctrl, router, _ := newTestController(t)
	inst := seedInstance(t, ctrl, "ws-1", "Original", "tok")
	body := `{"name":"Renamed","authMethod":"auth_token","url":"https://sentry.example.com"}`

	updated := do(router, http.MethodPut, "/api/v1/sentry/instances/"+inst.ID+"?workspace_id=ws-1", body)
	if updated.Code != http.StatusOK {
		t.Fatalf("update status = %d, body=%s", updated.Code, updated.Body.String())
	}
	var cfg SentryConfig
	if err := json.Unmarshal(updated.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode update: %v", err)
	}
	if cfg.Name != "Renamed" || cfg.URL != "https://sentry.example.com" {
		t.Errorf("updated instance = %+v", cfg)
	}

	crossWorkspace := do(router, http.MethodPut, "/api/v1/sentry/instances/"+inst.ID+"?workspace_id=ws-2", body)
	if crossWorkspace.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace update status = %d, want 404; body=%s", crossWorkspace.Code, crossWorkspace.Body.String())
	}
	if !strings.Contains(crossWorkspace.Body.String(), `"code":"SENTRY_INSTANCE_NOT_FOUND"`) {
		t.Errorf("missing cross-workspace not-found code: %s", crossWorkspace.Body.String())
	}
}

func TestHTTP_DeleteInstance_InUse409(t *testing.T) {
	ctrl, router, _ := newTestController(t)
	inst := seedInstance(t, ctrl, "ws-1", "A", "tok")
	w := newTestIssueWatch("ws-1")
	w.SentryInstanceID = inst.ID
	if err := ctrl.service.store.CreateIssueWatch(context.Background(), w); err != nil {
		t.Fatalf("seed watch: %v", err)
	}
	resp := do(router, http.MethodDelete, "/api/v1/sentry/instances/"+inst.ID+"?workspace_id=ws-1", "")
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", resp.Code, resp.Body.String())
	}
	var body struct {
		Code       string `json:"code"`
		WatchCount int    `json:"watchCount"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code != "SENTRY_INSTANCE_IN_USE" || body.WatchCount != 1 {
		t.Errorf("unexpected 409 body: %+v", body)
	}
}

func TestHTTP_Browse_RequiresWorkspaceAndInstance(t *testing.T) {
	ctrl, router, _ := newTestController(t)
	seedInstance(t, ctrl, "ws-1", "A", "tok")
	// Missing workspace_id → 400.
	if w := do(router, http.MethodGet, "/api/v1/sentry/projects", ""); w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 without workspace_id, got %d", w.Code)
	}
	// Missing instanceId → 400 SENTRY_INSTANCE_REQUIRED.
	w := do(router, http.MethodGet, "/api/v1/sentry/projects?workspace_id=ws-1", "")
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"code":"SENTRY_INSTANCE_REQUIRED"`) {
		t.Errorf("expected 400 SENTRY_INSTANCE_REQUIRED, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHTTP_Browse_NotConfigured503(t *testing.T) {
	ctrl, router, _ := newTestController(t)
	// Instance with no secret → 503 SENTRY_NOT_CONFIGURED.
	inst := seedInstance(t, ctrl, "ws-1", "A", "")
	w := do(router, http.MethodGet, "/api/v1/sentry/projects?workspace_id=ws-1&instanceId="+inst.ID, "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"code":"SENTRY_NOT_CONFIGURED"`) {
		t.Errorf("missing code in body: %s", w.Body.String())
	}
}

func TestHTTP_SearchIssues_ForwardsFilter(t *testing.T) {
	ctrl, router, client := newTestController(t)
	inst := seedInstance(t, ctrl, "ws-1", "A", "tok")
	var seen SearchFilter
	client.searchIssuesFn = func(filter SearchFilter, _ string) (*SearchResult, error) {
		seen = filter
		return &SearchResult{IsLast: true}, nil
	}
	target := "/api/v1/sentry/issues?workspace_id=ws-1&instanceId=" + inst.ID +
		"&orgSlug=acme&projectSlug=fe&level=error&level=fatal&status=unresolved&query=boom&statsPeriod=24h"
	if w := do(router, http.MethodGet, target, ""); w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if seen.OrgSlug != "acme" || seen.ProjectSlug != "fe" || seen.Query != "boom" || seen.StatsPeriod != "24h" {
		t.Errorf("filter = %+v", seen)
	}
	if !sameStrings(seen.Levels, []string{"error", "fatal"}) {
		t.Errorf("levels = %v", seen.Levels)
	}
	if !sameStrings(seen.Statuses, []string{"unresolved"}) {
		t.Errorf("statuses = %v", seen.Statuses)
	}
}

func TestHTTP_GetIssue_ForwardsID(t *testing.T) {
	ctrl, router, client := newTestController(t)
	inst := seedInstance(t, ctrl, "ws-1", "A", "tok")
	var seenID string
	client.getIssueFn = func(id string) (*SentryIssue, error) {
		seenID = id
		return &SentryIssue{ID: "99", ShortID: id}, nil
	}
	target := "/api/v1/sentry/issues/PROJ-7?workspace_id=ws-1&instanceId=" + inst.ID
	if w := do(router, http.MethodGet, target, ""); w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if seenID != "PROJ-7" {
		t.Errorf("id forwarded = %q", seenID)
	}
}

func TestHTTP_ListOrganizations_ReturnsWrapped(t *testing.T) {
	ctrl, router, client := newTestController(t)
	inst := seedInstance(t, ctrl, "ws-1", "A", "tok")
	client.listOrganizationsFn = func() ([]SentryOrganization, error) {
		return []SentryOrganization{{ID: "1", Slug: "acme", Name: "Acme"}}, nil
	}
	w := do(router, http.MethodGet, "/api/v1/sentry/organizations?workspace_id=ws-1&instanceId="+inst.ID, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Organizations []SentryOrganization `json:"organizations"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Organizations) != 1 || resp.Organizations[0].Slug != "acme" {
		t.Errorf("organizations = %+v", resp.Organizations)
	}
}

func TestHTTP_TestConnection_RequiresWorkspace(t *testing.T) {
	_, router, _ := newTestController(t)
	if w := do(router, http.MethodPost, "/api/v1/sentry/test-connection", `{"secret":"t"}`); w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 without workspace_id, got %d", w.Code)
	}
}

func TestHTTP_TestConnection_OK(t *testing.T) {
	ctrl, router, client := newTestController(t)
	client.testAuthFn = func() (*TestConnectionResult, error) {
		return &TestConnectionResult{OK: true, DisplayName: "Alice"}, nil
	}
	_ = ctrl
	w := do(router, http.MethodPost, "/api/v1/sentry/test-connection?workspace_id=ws-1",
		`{"authMethod":"auth_token","secret":"tok","url":"https://sentry.example.com"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var res TestConnectionResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !res.OK {
		t.Errorf("expected OK result, got %+v", res)
	}
}

func TestHTTP_CopyConfig_ReturnsWrappedList(t *testing.T) {
	ctrl, router, _ := newTestController(t)
	seedInstance(t, ctrl, "ws-src", "SaaS", "sec")
	w := do(router, http.MethodPost, "/api/v1/sentry/config/copy?workspace_id=ws-src",
		`{"targetWorkspaceId":"ws-dst"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Instances []SentryConfig `json:"instances"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Instances) != 1 || resp.Instances[0].WorkspaceID != "ws-dst" {
		t.Errorf("copied = %+v", resp.Instances)
	}
}

func TestHTTP_CopyConfig_IgnoresBodySourceWorkspace(t *testing.T) {
	ctrl, router, _ := newTestController(t)
	seedInstance(t, ctrl, "ws-query", "Authoritative", "sec")

	w := do(router, http.MethodPost, "/api/v1/sentry/config/copy?workspace_id=ws-query",
		`{"sourceWorkspaceId":"ws-body","targetWorkspaceId":"ws-dst"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Instances []SentryConfig `json:"instances"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Instances) != 1 || resp.Instances[0].Name != "Authoritative" {
		t.Errorf("copied = %+v", resp.Instances)
	}
}

func TestHTTP_CopyConfig_RejectsBodyOnlySourceWorkspace(t *testing.T) {
	ctrl, router, _ := newTestController(t)
	seedInstance(t, ctrl, "ws-src", "SaaS", "sec")

	w := do(router, http.MethodPost, "/api/v1/sentry/config/copy",
		`{"sourceWorkspaceId":"ws-src","targetWorkspaceId":"ws-dst"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestHTTP_CopyConfig_DuplicateNameReturnsCodedConflict(t *testing.T) {
	ctrl, router, _ := newTestController(t)
	seedInstance(t, ctrl, "ws-src", "SaaS", "sec")
	if _, err := ctrl.service.store.db.Exec(`
		CREATE TRIGGER copy_duplicate_name
		BEFORE INSERT ON sentry_configs
		WHEN NEW.workspace_id = 'ws-dst'
		BEGIN
			SELECT RAISE(ABORT, 'UNIQUE constraint failed: sentry_configs.workspace_id, sentry_configs.name');
		END`); err != nil {
		t.Fatalf("create duplicate trigger: %v", err)
	}

	w := do(router, http.MethodPost, "/api/v1/sentry/config/copy?workspace_id=ws-src",
		`{"targetWorkspaceId":"ws-dst"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
	}
	var response struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if response.Code != errCodeSentryInstanceNameTaken {
		t.Errorf("code = %q, want %q", response.Code, errCodeSentryInstanceNameTaken)
	}
}

// TestHTTP_CreateIssueWatch_RejectsMismatchedWorkspace pins acceptance (e) for
// watch-create.
func TestHTTP_CreateIssueWatch_RejectsMismatchedWorkspace(t *testing.T) {
	ctrl, router, _ := newTestController(t)
	inst := seedInstance(t, ctrl, "ws-1", "A", "tok")
	body := `{"workspaceId":"ws-1","sentryInstanceId":"` + inst.ID + `","workflowId":"wf","workflowStepId":"step","filter":{"orgSlug":"acme","projectSlug":"fe"}}`
	// Missing workspace_id query → 400.
	if w := do(router, http.MethodPost, "/api/v1/sentry/watches/issue", body); w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 without workspace_id, got %d body=%s", w.Code, w.Body.String())
	}
	// Mismatched workspace_id query → 400.
	if w := do(router, http.MethodPost, "/api/v1/sentry/watches/issue?workspace_id=ws-2", body); w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for mismatched workspace_id, got %d", w.Code)
	}
	// Matching → created (200).
	if w := do(router, http.MethodPost, "/api/v1/sentry/watches/issue?workspace_id=ws-1", body); w.Code != http.StatusOK {
		t.Errorf("expected 200 for matching workspace_id, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHTTP_CreateIssueWatch_RequiresInstance(t *testing.T) {
	_, router, _ := newTestController(t)
	body := `{"workspaceId":"ws-1","workflowId":"wf","workflowStepId":"step","filter":{"orgSlug":"acme","projectSlug":"fe"}}`

	resp := do(router, http.MethodPost, "/api/v1/sentry/watches/issue?workspace_id=ws-1", body)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", resp.Code, resp.Body.String())
	}
	var response struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != errCodeSentryInstanceRequired {
		t.Errorf("error code = %q, want %q", response.Code, errCodeSentryInstanceRequired)
	}
}

func TestWriteClientError_APIErrorStatusPassthrough(t *testing.T) {
	cases := []struct {
		upstream int
		want     int
	}{
		{http.StatusUnauthorized, http.StatusUnauthorized},
		{http.StatusForbidden, http.StatusForbidden},
		{http.StatusNotFound, http.StatusNotFound},
		{http.StatusBadRequest, http.StatusBadRequest},
		{http.StatusBadGateway, http.StatusInternalServerError},
	}
	ctrl, _, _ := newTestController(t)
	for _, tc := range cases {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		ctrl.writeClientError(c, &APIError{StatusCode: tc.upstream, Message: "x"})
		if w.Code != tc.want {
			t.Errorf("upstream %d → got %d, want %d", tc.upstream, w.Code, tc.want)
		}
	}
}

func TestWriteClientError_GenericError(t *testing.T) {
	ctrl, _, _ := newTestController(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	ctrl.writeClientError(c, errors.New("boom"))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestRegisterRoutes_AcceptsNilDispatcher(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	store := newTestStore(t)
	svc := NewService(store, newFakeSecretStore(), func(_ *SentryConfig, _ string) Client {
		return &fakeClient{}
	}, logger.Default())
	dispatcher := ws.NewDispatcher()
	RegisterRoutes(router, dispatcher, svc, logger.Default())
}

// TestHTTP_TriggerIssueWatch_AmbiguousUnboundIs4xx pins the P2 fix: a manual
// trigger of an unbound watch that resolves ambiguously (multiple healthy
// instances) is routed through the issue-watch error mapper, yielding a 4xx
// (ErrInvalidConfig → 400) rather than a bare 500.
func TestHTTP_TriggerIssueWatch_AmbiguousUnboundIs4xx(t *testing.T) {
	ctrl, router, _ := newTestController(t)
	ctx := context.Background()
	a := seedInstance(t, ctrl, "ws-1", "A", "tok")
	b := seedInstance(t, ctrl, "ws-1", "B", "tok")
	now := time.Now().UTC()
	for _, id := range []string{a.ID, b.ID} {
		if err := ctrl.service.store.UpdateAuthHealthForInstance(ctx, id, true, "", now); err != nil {
			t.Fatalf("mark healthy: %v", err)
		}
	}
	// Unbound watch: no SentryInstanceID → resolves ambiguously across the two
	// healthy instances.
	w := newTestIssueWatch("ws-1")
	if err := ctrl.service.store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("seed unbound watch: %v", err)
	}
	resp := do(router, http.MethodPost, "/api/v1/sentry/watches/issue/"+w.ID+"/trigger?workspace_id=ws-1", "")
	if resp.Code < 400 || resp.Code >= 500 {
		t.Fatalf("expected 4xx for ambiguous unbound-watch trigger, got %d body=%s", resp.Code, resp.Body.String())
	}
}
