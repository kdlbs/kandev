package handlers

import (
	"bytes"
	"context"
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
	"github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/internal/user/service"
	userstore "github.com/kandev/kandev/internal/user/store"
	ws "github.com/kandev/kandev/pkg/websocket"
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

	resetRequest := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/user/settings",
		bytes.NewReader([]byte(`{"sidebar_views":[]}`)),
	)
	resetRequest.Header.Set("Content-Type", "application/json")
	resetResponse := httptest.NewRecorder()
	router.ServeHTTP(resetResponse, resetRequest)
	if resetResponse.Code != http.StatusOK {
		t.Fatalf("PATCH empty sidebar views status = %d, want %d: %s", resetResponse.Code, http.StatusOK, resetResponse.Body.String())
	}
	resetGetResponse := httptest.NewRecorder()
	router.ServeHTTP(resetGetResponse, httptest.NewRequest(http.MethodGet, "/api/v1/user/settings", nil))
	if resetGetResponse.Code != http.StatusOK {
		t.Fatalf("GET reset user settings status = %d, want %d: %s", resetGetResponse.Code, http.StatusOK, resetGetResponse.Body.String())
	}
	var resetPayload dto.UserSettingsResponse
	if err := json.NewDecoder(resetGetResponse.Body).Decode(&resetPayload); err != nil {
		t.Fatalf("decode reset user settings: %v", err)
	}
	if len(resetPayload.Settings.SidebarViews) != 1 || resetPayload.Settings.SidebarViews[0].ID != "view-all-tasks" {
		t.Fatalf("reset sidebar views = %+v, want canonical All tasks view", resetPayload.Settings.SidebarViews)
	}
	if resetPayload.Settings.SidebarActiveViewID != "view-all-tasks" {
		t.Fatalf("reset active sidebar view = %q, want %q", resetPayload.Settings.SidebarActiveViewID, "view-all-tasks")
	}

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

func newTestUserSettingsRouter(t *testing.T) *gin.Engine {
	t.Helper()
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
	return router
}

func TestHTTPUpdateUserSettingsKanbanHiddenStepIDsRoundTrip(t *testing.T) {
	router := newTestUserSettingsRouter(t)

	patch := []byte(`{"kanban_hidden_step_ids":{"wf-1":["step-a","step-b"]}}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/user/settings", bytes.NewReader(patch))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("PATCH kanban_hidden_step_ids status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
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
	got := payload.Settings.KanbanHiddenStepIDs["wf-1"]
	if len(got) != 2 || got[0] != "step-a" || got[1] != "step-b" {
		t.Fatalf("kanban_hidden_step_ids[wf-1] = %v, want [step-a step-b]", got)
	}
}

func TestHTTPUpdateUserSettingsSidebarTaskColorsRoundTrip(t *testing.T) {
	router := newTestUserSettingsRouter(t)

	patch := []byte(`{"sidebar_task_color_patch":{"colors":{"task-red":"red","task-cleared":null}}}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/user/settings", bytes.NewReader(patch))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("PATCH sidebar task colors status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}

	var payload dto.UserSettingsResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode settings response: %v", err)
	}
	if value := payload.Settings.SidebarTaskColors["task-red"]; value == nil || *value != "red" {
		t.Fatalf("response task-red = %#v, want red", value)
	}
	if value, present := payload.Settings.SidebarTaskColors["task-cleared"]; !present || value != nil {
		t.Fatalf("response task-cleared = (%#v, %t), want (nil, true)", value, present)
	}
}

func TestHTTPUpdateUserSettingsBodyTooLarge(t *testing.T) {
	router := newTestUserSettingsRouter(t)

	oversized := append(bytes.Repeat([]byte(" "), maxUpdateUserSettingsBodyBytes+10), []byte("{}")...)
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/user/settings", bytes.NewReader(oversized))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("PATCH oversized body status = %d, want %d: %s", response.Code, http.StatusRequestEntityTooLarge, response.Body.String())
	}
}

// conflictSettingsRepository forces every conditional settings write to
// exercise the service retry limit without requiring a live database race.
type conflictSettingsRepository struct {
	userstore.Repository
}

func (conflictSettingsRepository) GetUserSettings(_ context.Context, userID string) (*models.UserSettings, error) {
	return &models.UserSettings{UserID: userID}, nil
}

func (conflictSettingsRepository) UpsertUserSettingsPreservingTaskCreateLastUsed(
	context.Context,
	*models.UserSettings,
	*models.TaskCreateLastUsed,
	int64,
) (*models.UserSettings, error) {
	return nil, userstore.ErrUserSettingsRevisionConflict
}

func newConflictHandlers(t *testing.T) *Handlers {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	svc := service.NewService(conflictSettingsRepository{}, nil, log)
	return NewHandlers(controller.NewController(svc), log)
}

func TestHTTPUpdateUserSettingsRevisionConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	newConflictHandlers(t).registerHTTP(router)

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/user/settings",
		bytes.NewReader([]byte(`{"last_seen_display":"relative"}`)),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("PATCH revision conflict status = %d, want %d: %s", response.Code, http.StatusConflict, response.Body.String())
	}
}

func TestWSUpdateUserSettingsRevisionConflict(t *testing.T) {
	h := newConflictHandlers(t)
	msg, err := ws.NewRequest("settings-1", ws.ActionUserSettingsUpdate, map[string]string{
		"last_seen_display": "relative",
	})
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	response, err := h.wsUpdateUserSettings(context.Background(), msg)
	if err != nil {
		t.Fatalf("wsUpdateUserSettings returned error: %v", err)
	}
	if response.Type != ws.MessageTypeError {
		t.Fatalf("response type = %q, want %q", response.Type, ws.MessageTypeError)
	}
	var payload ws.ErrorPayload
	if err := response.ParsePayload(&payload); err != nil {
		t.Fatalf("parse error payload: %v", err)
	}
	if payload.Code != ws.ErrorCodeConflict {
		t.Fatalf("error code = %q, want %q", payload.Code, ws.ErrorCodeConflict)
	}
}
