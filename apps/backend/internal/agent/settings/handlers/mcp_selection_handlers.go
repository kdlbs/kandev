package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

type mcpSelectionRequest struct {
	WorkspaceID                   string          `json:"workspace_id"`
	ExpectedDefinitionIDs         json.RawMessage `json:"expected_definition_ids"`
	ExcludedDefinitionIDs         json.RawMessage `json:"excluded_definition_ids"`
	ExpectedRevision              json.RawMessage `json:"expected_revision"`
	ExpectedExcludedDefinitionIDs json.RawMessage `json:"expected_excluded_definition_ids"`
	DefinitionIDs                 []string        `json:"definition_ids"`
	MCPServerIDs                  []string        `json:"mcp_server_ids"`
}

type mcpSelectionResponse struct {
	LegacyImportStatus    string                              `json:"legacy_import_status,omitempty"`
	WorkspaceID           string                              `json:"workspace_id"`
	Scope                 mcpconfig.SelectionScope            `json:"scope"`
	OwnerID               string                              `json:"owner_id"`
	DefinitionIDs         []string                            `json:"definition_ids"`
	ExcludedDefinitionIDs *[]string                           `json:"excluded_definition_ids,omitempty"`
	SelectionRevision     *int64                              `json:"selection_revision,omitempty"`
	MCPState              *mcpconfig.SessionMCPSelectionState `json:"mcp_state,omitempty"`
}

func (h *Handlers) httpListProfileMCPSelections(c *gin.Context) {
	h.listMCPSelections(c, mcpconfig.SelectionScopeProfile)
}

func (h *Handlers) httpReplaceProfileMCPSelections(c *gin.Context) {
	h.replaceMCPSelections(c, mcpconfig.SelectionScopeProfile)
}

func (h *Handlers) httpListRepositoryMCPSelections(c *gin.Context) {
	h.listMCPSelections(c, mcpconfig.SelectionScopeRepository)
}

func (h *Handlers) httpReplaceRepositoryMCPSelections(c *gin.Context) {
	h.replaceMCPSelections(c, mcpconfig.SelectionScopeRepository)
}

func (h *Handlers) httpListTaskMCPSelections(c *gin.Context) {
	h.listMCPSelections(c, mcpconfig.SelectionScopeTask)
}

func (h *Handlers) httpReplaceTaskMCPSelections(c *gin.Context) {
	h.replaceMCPSelections(c, mcpconfig.SelectionScopeTask)
}

func (h *Handlers) httpListTaskSessionMCPSelections(c *gin.Context) {
	h.listMCPSelections(c, mcpconfig.SelectionScopeTaskSession)
}

func (h *Handlers) httpReplaceTaskSessionMCPSelections(c *gin.Context) {
	h.replaceMCPSelections(c, mcpconfig.SelectionScopeTaskSession)
}

func (h *Handlers) listMCPSelections(c *gin.Context, scope mcpconfig.SelectionScope) {
	if !h.requireMCPSelections(c) {
		return
	}
	workspaceID := strings.TrimSpace(c.Query("workspace_id"))
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}
	ownerID := c.Param("id")
	snapshot, err := h.selections.ReadSnapshot(c.Request.Context(), mcpconfig.SelectionOrigin{Scope: scope, WorkspaceID: workspaceID, OwnerID: ownerID})
	if err != nil {
		h.writeMCPSelectionError(c, err)
		return
	}
	h.writeMCPSelectionResponse(c, workspaceID, scope, ownerID, snapshot)
}

func (h *Handlers) replaceMCPSelections(c *gin.Context, scope mcpconfig.SelectionScope) {
	if !h.requireMCPSelections(c) {
		return
	}
	var body mcpSelectionRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid MCP selection request"})
		return
	}
	workspaceID := strings.TrimSpace(body.WorkspaceID)
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(c.Query("workspace_id"))
	}
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}
	definitionIDs, err := selectionIDs(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "definition_ids and mcp_server_ids cannot both be set"})
		return
	}
	ownerID := c.Param("id")
	condition, err := selectionCondition(body, scope)
	if err != nil {
		h.writeMCPSelectionError(c, err)
		return
	}
	snapshot, err := h.selections.ReplaceConditional(c.Request.Context(), mcpconfig.SelectionOrigin{Scope: scope, WorkspaceID: workspaceID, OwnerID: ownerID}, definitionIDs, condition)
	if err != nil {
		h.writeMCPSelectionError(c, err)
		return
	}
	h.writeMCPSelectionResponse(c, workspaceID, scope, ownerID, snapshot)
}

func selectionIDs(body mcpSelectionRequest) ([]string, error) {
	if len(body.DefinitionIDs) > 0 && len(body.MCPServerIDs) > 0 {
		return nil, errors.New("duplicate selection fields")
	}
	if len(body.MCPServerIDs) > 0 {
		return body.MCPServerIDs, nil
	}
	return body.DefinitionIDs, nil
}

func (h *Handlers) requireMCPSelections(c *gin.Context) bool {
	if h.selections != nil {
		return true
	}
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": "MCP selections are not available"})
	return false
}

func selectionCondition(body mcpSelectionRequest, scope mcpconfig.SelectionScope) (mcpconfig.SelectionCondition, error) {
	condition := mcpconfig.SelectionCondition{}
	if body.ExpectedExcludedDefinitionIDs != nil {
		return condition, mcpconfig.ErrMCPInvalidSelection
	}
	if body.ExpectedDefinitionIDs != nil {
		var ids []string
		if err := json.Unmarshal(body.ExpectedDefinitionIDs, &ids); err != nil || ids == nil {
			return condition, mcpconfig.ErrMCPInvalidSelection
		}
		condition.ExpectedDefinitionIDs = &ids
	}
	if body.ExpectedRevision != nil {
		var revision *int64
		if err := json.Unmarshal(body.ExpectedRevision, &revision); err != nil || revision == nil {
			return condition, mcpconfig.ErrMCPInvalidSelection
		}
		condition.ExpectedRevision = revision
	}
	if body.ExcludedDefinitionIDs != nil {
		var ids []string
		if err := json.Unmarshal(body.ExcludedDefinitionIDs, &ids); err != nil || ids == nil {
			return condition, mcpconfig.ErrMCPInvalidSelection
		}
		condition.ExcludedDefinitionIDs = &ids
	}
	return condition, condition.Validate(scope)
}

func (h *Handlers) writeMCPSelectionResponse(c *gin.Context, workspaceID string, scope mcpconfig.SelectionScope, ownerID string, snapshot mcpconfig.SelectionSnapshot) {
	response := mcpSelectionResponse{WorkspaceID: workspaceID, Scope: scope, OwnerID: ownerID, DefinitionIDs: snapshot.DefinitionIDs, MCPState: snapshot.State}
	if scope == mcpconfig.SelectionScopeProfile {
		response.LegacyImportStatus = h.selections.ProfileImportStatus(c.Request.Context(), workspaceID, ownerID)
	}
	if scope == mcpconfig.SelectionScopeTaskSession {
		revision := int64(0)
		if snapshot.State != nil {
			revision = snapshot.State.DesiredRevision
		}
		response.SelectionRevision = &revision
		ids := snapshot.ExcludedDefinitionIDs
		if ids == nil {
			ids = []string{}
		}
		response.ExcludedDefinitionIDs = &ids
	}
	c.JSON(http.StatusOK, response)
}

func (h *Handlers) writeMCPSelectionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, mcpconfig.ErrMCPSelectionConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "MCP selections changed", "code": "mcp_selection_conflict"})
	case errors.Is(err, mcpconfig.ErrMCPWorkspaceAccess), errors.Is(err, mcpconfig.ErrMCPSelectionOwnerAccess):
		c.JSON(http.StatusNotFound, gin.H{"error": "MCP selection owner not found"})
	case errors.Is(err, mcpconfig.ErrMCPSelectionWorkspaceMismatch), errors.Is(err, mcpconfig.ErrMCPDefinitionDisabled), errors.Is(err, mcpconfig.ErrMCPInvalidSelection):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid MCP selection"})
	default:
		h.logger.Error("failed to access MCP selections", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to access MCP selections"})
	}
}
