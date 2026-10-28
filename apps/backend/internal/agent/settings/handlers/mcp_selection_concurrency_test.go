package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
)

func selectionRouter(t *testing.T) (*gin.Engine, *int) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	repo, _, err := store.Provide(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"docs", "other"} {
		err = repo.CreateMCPServerDefinition(context.Background(), &mcpconfig.MCPServerDefinition{ID: id, WorkspaceID: "w", RuntimeName: id, NormalizedRuntimeName: id, DisplayName: id, Enabled: true, ExecutionMode: "remote", Transport: "http", Configuration: mcpconfig.MCPServerConfiguration{URL: "https://example.com/mcp"}})
		if err != nil {
			t.Fatal(err)
		}
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	service := mcpconfig.NewSelectionService(repo, repo)
	service.SetSessionMCPStateRepository(repo)
	notifications := 0
	service.SetSessionMCPChangeNotifier(func(context.Context, string) { notifications++ })
	h := NewHandlers(nil, nil, log, "")
	h.selections = service
	router := gin.New()
	router.PUT("/repositories/:id", h.httpReplaceRepositoryMCPSelections)
	router.GET("/repositories/:id", h.httpListRepositoryMCPSelections)
	router.PUT("/sessions/:id", h.httpReplaceTaskSessionMCPSelections)
	router.GET("/sessions/:id", h.httpListTaskSessionMCPSelections)
	return router, &notifications
}

// @covers AC-AGENTS-MCP-ASSIGNMENT-003.1
func TestMCPSelectionConditionalReplace(t *testing.T) {
	router, _ := selectionRouter(t)
	initial := performCatalogRequest(router, http.MethodPut, "/repositories/r", `{"workspace_id":"w","definition_ids":["docs"],"expected_definition_ids":[]}`, "")
	if initial.Code != http.StatusOK {
		t.Fatalf("initial: %d %s", initial.Code, initial.Body.String())
	}
	stale := performCatalogRequest(router, http.MethodPut, "/repositories/r", `{"workspace_id":"w","definition_ids":["other"],"expected_definition_ids":[]}`, "")
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale replacement: got %d, want 409: %s", stale.Code, stale.Body.String())
	}
	got := performCatalogRequest(router, http.MethodGet, "/repositories/r?workspace_id=w", "", "")
	var body map[string]any
	if err := json.Unmarshal(got.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if ids := body["definition_ids"].([]any); len(ids) != 1 || ids[0] != "docs" {
		t.Fatalf("lost winning choices: %v", ids)
	}
}

// @covers AC-AGENTS-MCP-ASSIGNMENT-003.1, AC-AGENTS-MCP-ASSIGNMENT-003.3
func TestMCPSelectionSessionRevisionAndRetry(t *testing.T) {
	router, notifications := selectionRouter(t)
	for _, revision := range []string{"0", "1"} {
		response := performCatalogRequest(router, http.MethodPut, "/sessions/s", `{"workspace_id":"w","definition_ids":["docs"],"expected_revision":`+revision+`}`, "")
		if response.Code != http.StatusOK {
			t.Fatalf("save/retry: %d %s", response.Code, response.Body.String())
		}
	}
	stale := performCatalogRequest(router, http.MethodPut, "/sessions/s", `{"workspace_id":"w","definition_ids":["docs"],"expected_revision":1}`, "")
	if stale.Code != http.StatusConflict || *notifications != 2 {
		t.Fatalf("stale same-set retry: status=%d notifications=%d", stale.Code, *notifications)
	}
	response := performCatalogRequest(router, http.MethodGet, "/sessions/s?workspace_id=w", "", "")
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["selection_revision"] != float64(2) {
		t.Fatalf("missing consistent revision: %s", response.Body.String())
	}
}

func TestMCPSelectionInvalidConditions(t *testing.T) {
	router, notifications := selectionRouter(t)
	for _, tc := range []struct{ path, field string }{
		{"/repositories/r", `"expected_definition_ids":null`},
		{"/repositories/r", `"expected_revision":0`},
		{"/sessions/s", `"expected_revision":null`},
		{"/sessions/s", `"expected_revision":-1`},
		{"/sessions/s", `"expected_revision":1.5`},
		{"/sessions/s", `"expected_revision":"0"`},
		{"/sessions/s", `"expected_definition_ids":[]`},
		{"/sessions/s", `"expected_excluded_definition_ids":[]`},
	} {
		t.Run(tc.field+tc.path, func(t *testing.T) {
			response := performCatalogRequest(router, http.MethodPut, tc.path, `{"workspace_id":"w","definition_ids":["docs"],`+tc.field+`}`, "")
			if response.Code != http.StatusBadRequest {
				t.Fatalf("invalid condition accepted: %d %s", response.Code, response.Body.String())
			}
		})
	}
	if *notifications != 0 {
		t.Fatalf("invalid write notified runtime %d times", *notifications)
	}
}
