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

// seedConfig persists a singleton config + secret without going through
// Service.SetConfig (which fires an async probe we'd have to drain).
func seedConfig(t *testing.T, ctrl *Controller) {
	t.Helper()
	ctx := context.Background()
	if err := ctrl.service.store.UpsertConfig(ctx, &SentryConfig{
		AuthMethod: AuthMethodAuthToken,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := ctrl.service.secrets.Set(ctx, SecretKey, "sentry", "tok"); err != nil {
		t.Fatalf("set secret: %v", err)
	}
}

func TestHTTPGetConfig_NoConfig_Returns204(t *testing.T) {
	_, router, _ := newTestController(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sentry/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}
}

func TestHTTPGetConfig_ReturnsConfig(t *testing.T) {
	ctrl, router, _ := newTestController(t)
	seedConfig(t, ctrl)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sentry/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var cfg SentryConfig
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cfg.AuthMethod != AuthMethodAuthToken || !cfg.HasSecret {
		t.Errorf("config = %+v", cfg)
	}
}

func TestHTTPSearchIssues_MultiValueLevelsAndStatuses(t *testing.T) {
	ctrl, router, client := newTestController(t)
	seedConfig(t, ctrl)
	var seen SearchFilter
	client.searchIssuesFn = func(filter SearchFilter, _ string) (*SearchResult, error) {
		seen = filter
		return &SearchResult{IsLast: true}, nil
	}
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/sentry/issues?orgSlug=acme&projectSlug=fe&level=error&level=fatal&status=unresolved&query=boom&statsPeriod=24h",
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

func TestHTTPGetIssue_ForwardsID(t *testing.T) {
	ctrl, router, client := newTestController(t)
	seedConfig(t, ctrl)
	var seenID string
	client.getIssueFn = func(id string) (*SentryIssue, error) {
		seenID = id
		return &SentryIssue{ID: "99", ShortID: id}, nil
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sentry/issues/PROJ-7", nil)
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
	_, router, _ := newTestController(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sentry/projects", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"code":"SENTRY_NOT_CONFIGURED"`) {
		t.Errorf("missing code in body: %s", w.Body.String())
	}
}

func TestHTTPListOrganizations_ReturnsOrganizations(t *testing.T) {
	ctrl, router, client := newTestController(t)
	seedConfig(t, ctrl)
	client.listOrganizationsFn = func() ([]SentryOrganization, error) {
		return []SentryOrganization{{ID: "1", Slug: "acme", Name: "Acme"}}, nil
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sentry/organizations", nil)
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

func TestHTTPListOrganizations_NotConfigured_Returns503(t *testing.T) {
	_, router, _ := newTestController(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sentry/organizations", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"code":"SENTRY_NOT_CONFIGURED"`) {
		t.Errorf("missing code in body: %s", w.Body.String())
	}
}

func TestHTTPSetConfig_BadJSON(t *testing.T) {
	_, router, _ := newTestController(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/sentry/config", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
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
	// Sanity check that RegisterRoutes can be called with a real dispatcher
	// without registering any WS handlers (Phase 1 has no WS surface).
	gin.SetMode(gin.TestMode)
	router := gin.New()
	store := newTestStore(t)
	svc := NewService(store, newFakeSecretStore(), func(_ *SentryConfig, _ string) Client {
		return &fakeClient{}
	}, logger.Default())
	dispatcher := ws.NewDispatcher()
	RegisterRoutes(router, dispatcher, svc, logger.Default())
}
