package jira

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
)

// TestMockController_SetAuthHealth_PersistsWithoutWorkspaceID confirms that
// the mock auth-health route now accepts the install-wide payload (no
// workspaceId field) and forwards it to the singleton store, matching the
// production handler shape after the per-workspace → singleton refactor.
func TestMockController_SetAuthHealth_PersistsWithoutWorkspaceID(t *testing.T) {
	store := newTestStore(t)
	if err := store.UpsertConfig(t.Context(), &JiraConfig{
		SiteURL: "https://acme.atlassian.net", Email: "u@example.com", AuthMethod: AuthMethodAPIToken,
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	mock := NewMockClient()
	ctrl := NewMockController(mock, store, logger.Default())

	gin.SetMode(gin.TestMode)
	router := gin.New()
	ctrl.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/jira/mock/auth-health",
		bytes.NewBufferString(`{"ok": false, "error": "session expired"}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	cfg, err := store.GetConfig(t.Context())
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if cfg.LastOk {
		t.Error("expected LastOk=false after auth-health write")
	}
	if cfg.LastError != "session expired" {
		t.Errorf("expected error preserved, got %q", cfg.LastError)
	}
	if cfg.LastCheckedAt == nil {
		t.Error("expected LastCheckedAt set")
	}
}

// TestMockController_SetAuthHealth_RejectsInvalidJSON guards the bind step:
// junk in the body should fail fast with a 400 rather than write garbage to
// the store.
func TestMockController_SetAuthHealth_RejectsInvalidJSON(t *testing.T) {
	store := newTestStore(t)
	mock := NewMockClient()
	ctrl := NewMockController(mock, store, logger.Default())

	gin.SetMode(gin.TestMode)
	router := gin.New()
	ctrl.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/jira/mock/auth-health",
		bytes.NewBufferString(`not json`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}
