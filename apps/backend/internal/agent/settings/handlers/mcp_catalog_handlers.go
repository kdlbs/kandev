package handlers

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

type mcpDefinitionRequest struct {
	RuntimeName      string                           `json:"runtime_name"`
	DisplayName      string                           `json:"display_name"`
	Description      string                           `json:"description"`
	Enabled          *bool                            `json:"enabled"`
	ExecutionMode    mcpconfig.ExecutionMode          `json:"execution_mode"`
	Transport        mcpconfig.ServerType             `json:"transport"`
	Configuration    mcpconfig.MCPServerConfiguration `json:"configuration"`
	SecretBindings   []mcpconfig.MCPSecretBinding     `json:"secret_bindings"`
	Source           mcpconfig.DefinitionSource       `json:"source"`
	SourceIdentity   string                           `json:"source_identity"`
	ExpectedRevision int64                            `json:"expected_revision"`
	Confirm          bool                             `json:"confirm"`
}

type mcpDefinitionUpdateRequest struct {
	RuntimeName      *string                           `json:"runtime_name"`
	DisplayName      *string                           `json:"display_name"`
	Description      *string                           `json:"description"`
	Enabled          *bool                             `json:"enabled"`
	ExecutionMode    *mcpconfig.ExecutionMode          `json:"execution_mode"`
	Transport        *mcpconfig.ServerType             `json:"transport"`
	Configuration    *mcpconfig.MCPServerConfiguration `json:"configuration"`
	SecretBindings   *[]mcpconfig.MCPSecretBinding     `json:"secret_bindings"`
	Source           *mcpconfig.DefinitionSource       `json:"source"`
	SourceIdentity   *string                           `json:"source_identity"`
	ExpectedRevision int64                             `json:"expected_revision"`
}

type mcpDefinitionListResponse struct {
	Servers []mcpDefinitionListItem `json:"servers"`
}

type mcpDefinitionListItem struct {
	*mcpconfig.MCPServerDefinition
	SelectionImpact mcpconfig.SelectionImpact `json:"selection_impact"`
}

func (h *Handlers) httpListMCPCatalog(c *gin.Context) {
	if !h.requireMCPCatalog(c) {
		return
	}
	servers, err := h.catalog.List(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeMCPCatalogError(c, err)
		return
	}
	items := make([]mcpDefinitionListItem, 0, len(servers))
	for _, server := range servers {
		item := mcpDefinitionListItem{MCPServerDefinition: server}
		if h.selections != nil && server != nil {
			item.SelectionImpact, err = h.selections.SelectionImpact(c.Request.Context(), c.Param("id"), server.ID)
			if err != nil {
				h.writeMCPCatalogError(c, err)
				return
			}
		}
		items = append(items, item)
	}
	c.JSON(http.StatusOK, mcpDefinitionListResponse{Servers: items})
}

func (h *Handlers) httpGetMCPCatalog(c *gin.Context) {
	if !h.requireMCPCatalog(c) {
		return
	}
	server, err := h.catalog.Get(c.Request.Context(), c.Param("id"), c.Param("serverID"))
	if err != nil {
		h.writeMCPCatalogError(c, err)
		return
	}
	c.JSON(http.StatusOK, server)
}

func (h *Handlers) httpCreateMCPCatalog(c *gin.Context) {
	if !h.requireMCPCatalog(c) {
		return
	}
	var body mcpDefinitionRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid MCP server definition"})
		return
	}
	server, err := h.catalog.Create(c.Request.Context(), mcpconfig.CreateDefinitionInput{
		WorkspaceID: c.Param("id"), RuntimeName: body.RuntimeName,
		DisplayName: body.DisplayName, Description: body.Description, Enabled: body.Enabled,
		ExecutionMode: body.ExecutionMode, Transport: body.Transport,
		Configuration: body.Configuration, SecretBindings: body.SecretBindings,
		Source: body.Source, SourceIdentity: body.SourceIdentity,
	})
	if err != nil {
		h.writeMCPCatalogError(c, err)
		return
	}
	c.JSON(http.StatusCreated, server)
}

func (h *Handlers) httpUpdateMCPCatalog(c *gin.Context) {
	if !h.requireMCPCatalog(c) {
		return
	}
	var body mcpDefinitionUpdateRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid MCP server definition"})
		return
	}
	if body.ExpectedRevision <= 0 {
		body.ExpectedRevision = queryRevision(c)
	}
	if body.ExpectedRevision <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "expected_revision is required"})
		return
	}
	server, err := h.catalog.Update(c.Request.Context(), mcpconfig.UpdateDefinitionInput{
		WorkspaceID: c.Param("id"), ID: c.Param("serverID"),
		ExpectedRevision: body.ExpectedRevision, RuntimeName: body.RuntimeName,
		DisplayName: body.DisplayName, Description: body.Description, Enabled: body.Enabled,
		ExecutionMode: body.ExecutionMode, Transport: body.Transport,
		Configuration: body.Configuration, SecretBindings: body.SecretBindings,
		Source: body.Source, SourceIdentity: body.SourceIdentity,
	})
	if err != nil {
		h.writeMCPCatalogError(c, err)
		return
	}
	c.JSON(http.StatusOK, server)
}

func (h *Handlers) httpDeleteMCPCatalog(c *gin.Context) {
	if !h.requireMCPCatalog(c) {
		return
	}
	body, err := decodeDeleteRequest(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deletion request"})
		return
	}
	if body.ExpectedRevision <= 0 {
		body.ExpectedRevision = queryRevision(c)
	}
	if !body.Confirm && strings.EqualFold(c.Query("confirm"), "true") {
		body.Confirm = true
	}
	if body.ExpectedRevision <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "expected_revision is required"})
		return
	}
	if err := h.catalog.Delete(c.Request.Context(), c.Param("id"), c.Param("serverID"), body.ExpectedRevision, body.Confirm); err != nil {
		h.writeMCPCatalogError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *Handlers) requireMCPCatalog(c *gin.Context) bool {
	if h.catalog != nil {
		return true
	}
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": "MCP catalog is not available"})
	return false
}

func (h *Handlers) writeMCPCatalogError(c *gin.Context, err error) {
	var conflict *mcpconfig.MCPRevisionConflictError
	var impact *mcpconfig.MCPSelectionImpactError
	switch {
	case errors.As(err, &conflict):
		c.JSON(http.StatusConflict, gin.H{"error": "MCP server definition changed", "current": conflict.Current})
	case errors.As(err, &impact):
		c.JSON(http.StatusConflict, gin.H{
			"error":  "MCP server definition is selected",
			"impact": impact.Impact,
		})
	case errors.Is(err, mcpconfig.ErrMCPWorkspaceAccess), errors.Is(err, mcpconfig.ErrMCPServerDefinitionNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "MCP server definition not found"})
	case errors.Is(err, mcpconfig.ErrMCPRuntimeNameConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "MCP runtime name already exists"})
	case errors.Is(err, mcpconfig.ErrMCPRuntimeNameReserved), errors.Is(err, mcpconfig.ErrMCPInvalidDefinition), errors.Is(err, mcpconfig.ErrMCPDeleteConfirmation):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid MCP server definition"})
	default:
		h.logger.Error("failed to access MCP catalog", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to access MCP catalog"})
	}
}

func decodeDeleteRequest(c *gin.Context) (mcpDefinitionRequest, error) {
	var body mcpDefinitionRequest
	if c.Request.ContentLength == 0 {
		return body, nil
	}
	if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		return body, err
	}
	return body, nil
}

func queryRevision(c *gin.Context) int64 {
	revision, _ := strconv.ParseInt(strings.TrimSpace(c.Query("expected_revision")), 10, 64)
	return revision
}
