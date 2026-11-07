package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/common/logger"
)

type setupHandlerRepo struct {
	catalogHandlerRepository
	ids             []string
	state           mcpconfig.SessionMCPSelectionState
	projectionError bool
}

func (r *setupHandlerRepo) ListMCPSelections(context.Context, mcpconfig.SelectionScope, string, string) ([]string, error) {
	return r.ids, nil
}
func (r *setupHandlerRepo) ReplaceMCPSelections(_ context.Context, _ mcpconfig.SelectionScope, _, _ string, ids []string) error {
	r.ids = ids
	return nil
}
func (r *setupHandlerRepo) SelectionImpact(context.Context, string, string) (mcpconfig.SelectionImpact, error) {
	return mcpconfig.SelectionImpact{}, nil
}
func (r *setupHandlerRepo) DeleteMCPSelectionsForDefinition(context.Context, string, string) error {
	return nil
}
func (r *setupHandlerRepo) GetMCPSelectionState(context.Context, string) (mcpconfig.SessionMCPSelectionState, error) {
	return r.state, nil
}
func (r *setupHandlerRepo) SaveMCPSelectionState(_ context.Context, _ string, state mcpconfig.SessionMCPSelectionState) error {
	r.state = state
	return nil
}
func (r *setupHandlerRepo) ReadMCPResolutionInputs(context.Context, mcpconfig.ResolutionContext) (*mcpconfig.ResolutionInputs, error) {
	if r.projectionError {
		return nil, errors.New("private storage detail")
	}
	defs := map[string]*mcpconfig.MCPServerDefinition{}
	for _, id := range r.ids {
		defs[id] = r.definitions[id]
	}
	return &mcpconfig.ResolutionInputs{Definitions: defs, DesiredRevision: r.state.DesiredRevision, SessionSelectionIDs: r.ids}, nil
}

func TestMCPSessionRoutesExposeSafeComparisonAndKeepSuccessfulSave(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	definition := &mcpconfig.MCPServerDefinition{ID: "docs", WorkspaceID: "workspace-1", RuntimeName: "docs", DisplayName: "Documentation", Enabled: true, Revision: 1, Configuration: mcpconfig.MCPServerConfiguration{URL: "https://private.example", Headers: map[string]string{"Authorization": "secret"}}}
	repo := &setupHandlerRepo{catalogHandlerRepository: catalogHandlerRepository{definitions: map[string]*mcpconfig.MCPServerDefinition{"docs": definition}}, ids: []string{}, state: mcpconfig.SessionMCPSelectionState{ApplyState: mcpconfig.SessionMCPApplyStateApplied, AcceptedSetup: &mcpconfig.MCPSetupSnapshot{Version: 1, Servers: []mcpconfig.MCPSetupServer{}}}}
	service := mcpconfig.NewSelectionService(repo, repo)
	service.SetSessionMCPStateRepository(repo)
	service.SetOwnerValidator(func(_ context.Context, _ mcpconfig.SelectionScope, workspace, owner string) error {
		if workspace != "workspace-1" || owner != "session" {
			return errors.New("denied")
		}
		return nil
	})
	service.SetSessionResolutionContext(func(_ context.Context, w, s string) (mcpconfig.ResolutionContext, error) {
		return mcpconfig.ResolutionContext{WorkspaceID: w, SessionID: s}, nil
	})
	h := NewHandlers(nil, nil, log, "")
	h.selections = service
	router := gin.New()
	router.GET("/sessions/:id", h.httpListTaskSessionMCPSelections)
	router.PUT("/sessions/:id", h.httpReplaceTaskSessionMCPSelections)
	for _, path := range []string{"/sessions/sibling?workspace_id=workspace-1", "/sessions/session?workspace_id=other"} {
		if response := performCatalogRequest(router, http.MethodGet, path, "", ""); response.Code != http.StatusNotFound {
			t.Fatalf("unauthorized = %d", response.Code)
		}
	}
	response := performCatalogRequest(router, http.MethodPut, "/sessions/session", `{"workspace_id":"workspace-1","definition_ids":["docs"]}`, "")
	if response.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s", response.Code, response.Body.String())
	}
	var body mcpSelectionResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.MCPState.Comparison.Status != "known" || len(body.MCPState.Comparison.Changes) != 1 {
		t.Fatalf("comparison = %#v", body.MCPState)
	}
	for _, private := range []string{"accepted_setup", "private.example", "secret", "Authorization"} {
		if strings.Contains(response.Body.String(), private) {
			t.Fatalf("unsafe response: %s", response.Body.String())
		}
	}
	repo.projectionError = true
	response = performCatalogRequest(router, http.MethodPut, "/sessions/session", `{"workspace_id":"workspace-1","definition_ids":["docs"]}`, "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"unavailable"`) || strings.Contains(response.Body.String(), "private storage") {
		t.Fatalf("projection failure rejected committed write: %d %s", response.Code, response.Body.String())
	}
}
