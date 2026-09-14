package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/common/httpmw"
	"github.com/kandev/kandev/internal/common/logger"
)

type catalogHandlerRepository struct {
	definitions map[string]*mcpconfig.MCPServerDefinition
}

func (r *catalogHandlerRepository) ListMCPServerDefinitions(_ context.Context, workspaceID string) ([]*mcpconfig.MCPServerDefinition, error) {
	definitions := make([]*mcpconfig.MCPServerDefinition, 0)
	for _, definition := range r.definitions {
		if definition.WorkspaceID == workspaceID {
			definitions = append(definitions, definition)
		}
	}
	return definitions, nil
}

func (r *catalogHandlerRepository) GetMCPServerDefinition(_ context.Context, workspaceID, id string) (*mcpconfig.MCPServerDefinition, error) {
	definition, ok := r.definitions[id]
	if !ok || definition.WorkspaceID != workspaceID {
		return nil, mcpconfig.ErrMCPServerDefinitionNotFound
	}
	return definition, nil
}

func (r *catalogHandlerRepository) CreateMCPServerDefinition(_ context.Context, definition *mcpconfig.MCPServerDefinition) error {
	r.definitions[definition.ID] = definition
	return nil
}

func (r *catalogHandlerRepository) UpdateMCPServerDefinition(_ context.Context, definition *mcpconfig.MCPServerDefinition, expectedRevision int64) error {
	current, err := r.GetMCPServerDefinition(context.Background(), definition.WorkspaceID, definition.ID)
	if err != nil {
		return err
	}
	if current.Revision != expectedRevision {
		return &mcpconfig.MCPRevisionConflictError{Current: current}
	}
	r.definitions[definition.ID] = definition
	return nil
}

func (r *catalogHandlerRepository) DeleteMCPServerDefinition(_ context.Context, workspaceID, id string, expectedRevision int64) error {
	current, err := r.GetMCPServerDefinition(context.Background(), workspaceID, id)
	if err != nil {
		return err
	}
	if current.Revision != expectedRevision {
		return &mcpconfig.MCPRevisionConflictError{Current: current}
	}
	delete(r.definitions, id)
	return nil
}

func TestMCPCatalogRoutesCreateListUpdateDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	repo := &catalogHandlerRepository{definitions: make(map[string]*mcpconfig.MCPServerDefinition)}
	router := gin.New()
	RegisterRoutesWithMCPCatalog(router, nil, nil, log, "test-interlock", repo, allowCatalogWorkspace)

	createBody := `{"runtime_name":"linear","display_name":"Linear","execution_mode":"remote","transport":"http","configuration":{"url":"https://mcp.example.test"}}`
	create := performCatalogRequest(router, http.MethodPost, "/api/v1/workspaces/workspace-1/mcp-servers", createBody, "test-interlock")
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", create.Code, create.Body.String())
	}
	var created mcpconfig.MCPServerDefinition
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	list := performCatalogRequest(router, http.MethodGet, "/api/v1/workspaces/workspace-1/mcp-servers", "", "")
	if list.Code != http.StatusOK || !containsCatalogServer(list.Body.Bytes(), created.ID) {
		t.Fatalf("list status/body = %d/%s", list.Code, list.Body.String())
	}

	patchBody := `{"expected_revision":1,"description":"updated"}`
	update := performCatalogRequest(router, http.MethodPatch, "/api/v1/workspaces/workspace-1/mcp-servers/"+created.ID, patchBody, "test-interlock")
	if update.Code != http.StatusOK || !containsCatalogServer(update.Body.Bytes(), created.ID) {
		t.Fatalf("update status/body = %d/%s", update.Code, update.Body.String())
	}

	deleteBody := `{"expected_revision":2,"confirm":true}`
	remove := performCatalogRequest(router, http.MethodDelete, "/api/v1/workspaces/workspace-1/mcp-servers/"+created.ID, deleteBody, "test-interlock")
	if remove.Code != http.StatusOK {
		t.Fatalf("delete status/body = %d/%s", remove.Code, remove.Body.String())
	}
}

func TestMCPCatalogRoutesHideUnauthorizedWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	router := gin.New()
	repo := &catalogHandlerRepository{definitions: make(map[string]*mcpconfig.MCPServerDefinition)}
	RegisterRoutesWithMCPCatalog(router, nil, nil, log, "test-interlock", repo, func(context.Context, string) error {
		return errors.New("not allowed")
	})
	response := performCatalogRequest(router, http.MethodGet, "/api/v1/workspaces/secret-workspace/mcp-servers", "", "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("unauthorized status = %d, body = %s", response.Code, response.Body.String())
	}
}

func allowCatalogWorkspace(_ context.Context, workspaceID string) error {
	if workspaceID != "workspace-1" {
		return errors.New("workspace not found")
	}
	return nil
}

func performCatalogRequest(router http.Handler, method, path, body, interlock string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if interlock != "" {
		request.Header.Set(httpmw.InterimSettingsInterlockHeader, interlock)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func containsCatalogServer(body []byte, id string) bool {
	return strings.Contains(string(body), id)
}
