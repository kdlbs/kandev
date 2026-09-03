package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

type mcpSelectionRequest struct {
	WorkspaceID   string   `json:"workspace_id"`
	DefinitionIDs []string `json:"definition_ids"`
	MCPServerIDs  []string `json:"mcp_server_ids"`
}

type mcpSelectionResponse struct {
	WorkspaceID   string                              `json:"workspace_id"`
	Scope         mcpconfig.SelectionScope            `json:"scope"`
	OwnerID       string                              `json:"owner_id"`
	DefinitionIDs []string                            `json:"definition_ids"`
	MCPState      *mcpconfig.SessionMCPSelectionState `json:"mcp_state,omitempty"`
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
	definitionIDs, err := h.selections.List(c.Request.Context(), scope, workspaceID, ownerID)
	if err != nil {
		h.writeMCPSelectionError(c, err)
		return
	}
	h.writeMCPSelectionResponse(c, workspaceID, scope, ownerID, definitionIDs)
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
	if err := h.selections.Replace(c.Request.Context(), scope, workspaceID, ownerID, definitionIDs); err != nil {
		h.writeMCPSelectionError(c, err)
		return
	}
	definitionIDs, err = h.selections.List(c.Request.Context(), scope, workspaceID, ownerID)
	if err != nil {
		h.writeMCPSelectionError(c, err)
		return
	}
	h.writeMCPSelectionResponse(c, workspaceID, scope, ownerID, definitionIDs)
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

func (h *Handlers) writeMCPSelectionResponse(c *gin.Context, workspaceID string, scope mcpconfig.SelectionScope, ownerID string, definitionIDs []string) {
	response := mcpSelectionResponse{
		WorkspaceID: workspaceID, Scope: scope, OwnerID: ownerID,
		DefinitionIDs: definitionIDs,
	}
	if scope == mcpconfig.SelectionScopeTaskSession {
		state, err := h.selections.SessionState(c.Request.Context(), workspaceID, ownerID)
		if err == nil {
			response.MCPState = &state
		} else if !errors.Is(err, mcpconfig.ErrMCPSelectionStateNotFound) {
			h.writeMCPSelectionError(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, response)
}

func (h *Handlers) writeMCPSelectionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, mcpconfig.ErrMCPWorkspaceAccess), errors.Is(err, mcpconfig.ErrMCPSelectionOwnerAccess):
		c.JSON(http.StatusNotFound, gin.H{"error": "MCP selection owner not found"})
	case errors.Is(err, mcpconfig.ErrMCPSelectionWorkspaceMismatch), errors.Is(err, mcpconfig.ErrMCPDefinitionDisabled), errors.Is(err, mcpconfig.ErrMCPInvalidSelection):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid MCP selection"})
	default:
		h.logger.Error("failed to access MCP selections", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to access MCP selections"})
	}
}
