package sentry

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

// seedInstance persists a config + secret directly (bypassing Service.Create,
// which fires an async probe we'd have to drain) and returns the instance id.
func seedInstance(t *testing.T, ctrl *Controller) string {
	t.Helper()
	ctx := context.Background()
	cfg := &SentryConfig{Name: "Prod", AuthMethod: AuthMethodAuthToken, URL: "https://sentry.io"}
	if err := ctrl.service.store.CreateConfig(ctx, cfg); err != nil {
		t.Fatalf("create config: %v", err)
	}
	if err := ctrl.service.secrets.Set(ctx, secretKeyFor(cfg.ID), "sentry", "tok"); err != nil {
		t.Fatalf("set secret: %v", err)
	}
	return cfg.ID
}

// seedInstanceNoSecret persists a config row without a token, modelling an
// instance that exists but is not yet usable.
func seedInstanceNoSecret(t *testing.T, ctrl *Controller) string {
	t.Helper()
	cfg := &SentryConfig{Name: "Half", AuthMethod: AuthMethodAuthToken, URL: "https://sentry.io"}
	if err := ctrl.service.store.CreateConfig(context.Background(), cfg); err != nil {
		t.Fatalf("create config: %v", err)
	}
	return cfg.ID
}

func TestHTTPListInstances_Empty(t *testing.T) {
	_, router, _ := newTestController(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sentry/instances", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Instances []SentryConfig `json:"instances"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Instances) != 0 {
		t.Errorf("expected no instances, got %d", len(resp.Instances))
	}
}

func TestHTTPInstances_CreateListGetDelete(t *testing.T) {
	_, router, _ := newTestController(t)

	// Create
	body := `{"name":"Prod","authMethod":"auth_token","url":"https://sentry.io","secret":"tok"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sentry/instances", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", w.Code, w.Body.String())
	}
	var created SentryConfig
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	if created.ID == "" || created.Name != "Prod" || !created.HasSecret {
		t.Fatalf("created instance = %+v", created)
	}

	// List
	req = httptest.NewRequest(http.MethodGet, "/api/v1/sentry/instances", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var listResp struct {
		Instances []SentryConfig `json:"instances"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &listResp)
	if len(listResp.Instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(listResp.Instances))
	}

	// Get
	req = httptest.NewRequest(http.MethodGet, "/api/v1/sentry/instances/"+created.ID, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get status = %d", w.Code)
	}

	// Delete
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/sentry/instances/"+created.ID, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete status = %d", w.Code)
	}
}

func TestHTTPGetInstance_NotFound(t *testing.T) {
	_, router, _ := newTestController(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sentry/instances/ghost", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"code":"SENTRY_INSTANCE_NOT_FOUND"`) {
		t.Errorf("missing code in body: %s", w.Body.String())
	}
}

func TestHTTPDeleteInstance_InUse_Returns409(t *testing.T) {
	ctrl, router, _ := newTestController(t)
	id := seedInstance(t, ctrl)
	if err := ctrl.service.store.CreateIssueWatch(context.Background(), &IssueWatch{
		WorkspaceID:    "ws-1",
		InstanceID:     id,
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Filter:         SearchFilter{OrgSlug: "org", ProjectSlug: "proj"},
	}); err != nil {
		t.Fatalf("seed watch: %v", err)
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sentry/instances/"+id, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"code":"SENTRY_INSTANCE_IN_USE"`) {
		t.Errorf("missing code in body: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"watchCount":1`) {
		t.Errorf("missing watchCount in body: %s", w.Body.String())
	}
}

func TestHTTPCreateInstance_BadJSON(t *testing.T) {
	_, router, _ := newTestController(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sentry/instances", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHTTPSearchIssues_MultiValueLevelsAndStatuses(t *testing.T) {
	ctrl, router, client := newTestController(t)
	id := seedInstance(t, ctrl)
	var seen SearchFilter
	client.searchIssuesFn = func(filter SearchFilter, _ string) (*SearchResult, error) {
		seen = filter
		return &SearchResult{IsLast: true}, nil
	}
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/sentry/issues?instanceId="+id+"&orgSlug=acme&projectSlug=fe&level=error&level=fatal&status=unresolved&query=boom&statsPeriod=24h",
		nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
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

func TestHTTPSearchIssues_MissingInstance_Returns400(t *testing.T) {
	_, router, _ := newTestController(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sentry/issues?orgSlug=acme&projectSlug=fe", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"code":"SENTRY_INSTANCE_REQUIRED"`) {
		t.Errorf("missing code in body: %s", w.Body.String())
	}
}

func TestHTTPGetIssue_ForwardsID(t *testing.T) {
	ctrl, router, client := newTestController(t)
	id := seedInstance(t, ctrl)
	var seenID string
	client.getIssueFn = func(issueID string) (*SentryIssue, error) {
		seenID = issueID
		return &SentryIssue{ID: "99", ShortID: issueID}, nil
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sentry/issues/PROJ-7?instanceId="+id, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
	if seenID != "PROJ-7" {
		t.Errorf("id forwarded = %q", seenID)
	}
}

func TestHTTPListProjects_NotConfigured_Returns503(t *testing.T) {
	ctrl, router, _ := newTestController(t)
	id := seedInstanceNoSecret(t, ctrl)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sentry/projects?instanceId="+id, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"code":"SENTRY_NOT_CONFIGURED"`) {
		t.Errorf("missing code in body: %s", w.Body.String())
	}
}

func TestHTTPListProjects_MissingInstance_Returns400(t *testing.T) {
	_, router, _ := newTestController(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sentry/projects", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHTTPListOrganizations_ReturnsOrganizations(t *testing.T) {
	ctrl, router, client := newTestController(t)
	id := seedInstance(t, ctrl)
	client.listOrganizationsFn = func() ([]SentryOrganization, error) {
		return []SentryOrganization{{ID: "1", Slug: "acme", Name: "Acme"}}, nil
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sentry/organizations?instanceId="+id, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
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

func TestHTTPListOrganizations_MissingInstance_Returns400(t *testing.T) {
	_, router, _ := newTestController(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sentry/organizations", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
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
	// Sanity check that RegisterRoutes wires the instance + browse routes
	// without registering any WS handlers (no WS surface).
	gin.SetMode(gin.TestMode)
	router := gin.New()
	store := newTestStore(t)
	svc := NewService(store, newFakeSecretStore(), func(_ *SentryConfig, _ string) Client {
		return &fakeClient{}
	}, logger.Default())
	dispatcher := ws.NewDispatcher()
	RegisterRoutes(router, dispatcher, svc, logger.Default())
}
