package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/mcpconfig/registry"
)

type marketplaceInstallRequest struct {
	Identity         string                       `json:"identity"`
	ExpectedRevision int64                        `json:"expected_revision"`
	ChoiceID         string                       `json:"choice_id"`
	RuntimeName      string                       `json:"runtime_name"`
	DisplayName      string                       `json:"display_name"`
	SecretBindings   []mcpconfig.MCPSecretBinding `json:"secret_bindings"`
}

func (h *Handlers) httpSearchMCPMarketplace(c *gin.Context) {
	if !h.requireMarketplace(c) {
		return
	}
	result, err := h.marketplace.Search(c.Request.Context(), c.Query("search"))
	if err != nil {
		h.writeMarketplaceError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handlers) httpGetMCPMarketplaceEntry(c *gin.Context) {
	if !h.requireMarketplace(c) {
		return
	}
	identity := strings.TrimSpace(c.Query("identity"))
	if identity == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "identity is required"})
		return
	}
	entry, err := h.marketplace.Get(c.Request.Context(), identity)
	if err != nil {
		h.writeMarketplaceError(c, err)
		return
	}
	c.JSON(http.StatusOK, entry)
}

func (h *Handlers) httpRefreshMCPMarketplace(c *gin.Context) {
	if !h.requireMarketplace(c) {
		return
	}
	result, err := h.marketplace.Refresh(c.Request.Context())
	if err != nil {
		h.writeMarketplaceError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handlers) httpInstallMCPMarketplace(c *gin.Context) {
	if !h.requireMarketplace(c) {
		return
	}
	var body marketplaceInstallRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid marketplace installation request"})
		return
	}
	bindings := make([]registry.SecretBindingInput, 0, len(body.SecretBindings))
	for _, binding := range body.SecretBindings {
		bindings = append(bindings, registry.SecretBindingInput{InputName: binding.InputName, SecretID: binding.SecretID})
	}
	definition, err := h.marketplace.Install(c.Request.Context(), registry.InstallRequest{
		WorkspaceID: c.Param("id"), Identity: body.Identity,
		ExpectedRevision: body.ExpectedRevision, ChoiceID: body.ChoiceID,
		RuntimeName: body.RuntimeName, DisplayName: body.DisplayName,
		SecretBindings: bindings,
	})
	if err != nil {
		h.writeMarketplaceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, definition)
}

func (h *Handlers) requireMarketplace(c *gin.Context) bool {
	if h.marketplace != nil {
		return true
	}
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": "MCP marketplace is not available"})
	return false
}

func (h *Handlers) writeMarketplaceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, registry.ErrRegistryEntryNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "marketplace entry not found"})
	case errors.Is(err, registry.ErrMarketplaceRevisionRequired), errors.Is(err, registry.ErrMarketplaceChoiceNotFound):
		c.JSON(http.StatusBadRequest, gin.H{"error": "marketplace review is required"})
	case errors.Is(err, registry.ErrMarketplaceEntryUnavailable), errors.Is(err, registry.ErrMarketplaceChoiceUnsupported):
		c.JSON(http.StatusConflict, gin.H{"error": "marketplace choice is no longer available"})
	case errors.Is(err, registry.ErrMarketplaceCatalogUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "MCP marketplace is not available"})
	default:
		var upstream *registry.RegistryHTTPError
		if errors.As(err, &upstream) {
			c.JSON(http.StatusBadGateway, gin.H{"error": "MCP Registry is unavailable"})
			return
		}
		h.logger.Error("failed to access MCP marketplace", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to access MCP marketplace"})
	}
}
