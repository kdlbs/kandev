package quickterminal

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/quickterminal/repository"
)

func testRouter(t *testing.T, svc *Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	svc.RegisterRoutes(router)
	return router
}

func TestHTTPQuickTerminalTabLifecycleIsWorkspaceScopedAndIdempotent(t *testing.T) {
	repo := serviceRepository(t)
	svc := NewService(repo, nil, nil)
	router := testRouter(t, svc)
	tabID := uuid.NewString()

	body, _ := json.Marshal(map[string]string{
		"tab_id":       tabID,
		"workspace_id": "workspace-1",
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/quick-terminal-tabs", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("create status = %d, body %s", response.Code, response.Body.String())
	}
	var first struct {
		TabID    string `json:"tabId"`
		Sequence int    `json:"sequence"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &first); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if first.TabID != tabID || first.Sequence != 1 {
		t.Fatalf("first descriptor = %#v", first)
	}

	repeated := httptest.NewRecorder()
	router.ServeHTTP(repeated, httptest.NewRequest(
		http.MethodPost,
		"/api/v1/quick-terminal-tabs",
		bytes.NewReader(body),
	))
	if repeated.Code != http.StatusOK {
		t.Fatalf("repeated status = %d", repeated.Code)
	}
	var repeatedTab struct {
		Sequence int `json:"sequence"`
	}
	_ = json.Unmarshal(repeated.Body.Bytes(), &repeatedTab)
	if repeatedTab.Sequence != 1 {
		t.Fatalf("repeated sequence = %d, want 1", repeatedTab.Sequence)
	}

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(
		http.MethodGet,
		"/api/v1/quick-terminal-tabs?workspace_id=workspace-1",
		nil,
	))
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d", list.Code)
	}
	var listed struct {
		Tabs []struct {
			TabID string `json:"tabId"`
		} `json:"tabs"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed.Tabs) != 1 || listed.Tabs[0].TabID != tabID {
		t.Fatalf("listed tabs = %#v", listed.Tabs)
	}

	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/quick-terminal-tabs/"+tabID,
		nil,
	))
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete status = %d", deleteResponse.Code)
	}
	if _, err := repo.Get(context.Background(), "default-user", tabID); err == nil {
		t.Fatal("deleted descriptor still exists")
	} else if err != repository.ErrNotFound {
		t.Fatalf("get deleted descriptor: %v", err)
	}
}

func TestHTTPQuickTerminalRejectsInvalidTabID(t *testing.T) {
	svc := NewService(serviceRepository(t), nil, nil)
	router := testRouter(t, svc)
	body := bytes.NewBufferString(`{"tab_id":"bad","workspace_id":"workspace-1"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/quick-terminal-tabs", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid tab status = %d, want 400", response.Code)
	}
}
