package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/user/controller"
	"github.com/kandev/kandev/internal/user/dto"
	"github.com/kandev/kandev/internal/user/service"
	userstore "github.com/kandev/kandev/internal/user/store"
	"go.uber.org/zap"
)

func TestHTTPUpdateSidebarDraftFromCleanSettings(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	repo, cleanup, err := userstore.Provide(conn, conn)
	if err != nil {
		t.Fatalf("create user repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandlers(controller.NewController(service.NewService(repo, nil, log)), log).registerHTTP(router)

	patch := []byte(`{
		"sidebar_active_view_id":"view-all-tasks",
		"sidebar_draft":{
			"base_view_id":"view-all-tasks",
			"filters":[],
			"sort":{"key":"updatedAt","direction":"desc"},
			"group":"workflow"
		}
	}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/user/settings", bytes.NewReader(patch))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("PATCH clean sidebar settings status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}

	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, httptest.NewRequest(http.MethodGet, "/api/v1/user/settings", nil))
	if getResponse.Code != http.StatusOK {
		t.Fatalf("GET user settings status = %d, want %d: %s", getResponse.Code, http.StatusOK, getResponse.Body.String())
	}
	var payload dto.UserSettingsResponse
	if err := json.NewDecoder(getResponse.Body).Decode(&payload); err != nil {
		t.Fatalf("decode user settings: %v", err)
	}
	if len(payload.Settings.SidebarViews) != 1 || payload.Settings.SidebarViews[0].ID != "view-all-tasks" {
		t.Fatalf("sidebar views = %+v, want canonical All tasks view", payload.Settings.SidebarViews)
	}
	if payload.Settings.SidebarActiveViewID != "view-all-tasks" {
		t.Fatalf("active sidebar view = %q, want %q", payload.Settings.SidebarActiveViewID, "view-all-tasks")
	}
	if payload.Settings.SidebarDraft == nil || payload.Settings.SidebarDraft.Group != "workflow" {
		t.Fatalf("sidebar draft = %+v, want workflow draft", payload.Settings.SidebarDraft)
	}
}
