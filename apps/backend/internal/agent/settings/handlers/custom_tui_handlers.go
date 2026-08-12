package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/settings/controller"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type createCustomTUIAgentRequest struct {
	DisplayName string `json:"display_name"`
	Model       string `json:"model"`
	Command     string `json:"command"`
	Description string `json:"description"`
	// CommandArgs are appended to the argv built from Command. Unlike Command —
	// which the registry splits on whitespace — each entry is one argv element,
	// so an argument containing a space can only be expressed here.
	CommandArgs []string `json:"command_args"`
	// MCPStrategy names how the wrapped CLI loads MCP servers, so kandev can
	// point it at the per-session server. Empty (the default) = no MCP tools.
	MCPStrategy string `json:"mcp_strategy"`
}

func (r createCustomTUIAgentRequest) toControllerRequest() controller.CreateCustomTUIAgentRequest {
	return controller.CreateCustomTUIAgentRequest{
		DisplayName: r.DisplayName,
		Model:       r.Model,
		Command:     r.Command,
		Description: r.Description,
		CommandArgs: r.CommandArgs,
		MCPStrategy: r.MCPStrategy,
	}
}

func (h *Handlers) httpCreateCustomTUIAgent(c *gin.Context) {
	var body createCustomTUIAgentRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if body.DisplayName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "display_name is required"})
		return
	}
	if body.Command == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "command is required"})
		return
	}

	resp, err := h.controller.CreateCustomTUIAgent(c.Request.Context(), body.toControllerRequest())
	if err != nil {
		switch err {
		case controller.ErrInvalidSlug, controller.ErrCommandRequired, controller.ErrUnknownMCPStrategy:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case controller.ErrAgentAlreadyExists:
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			h.logger.Error("failed to create custom TUI agent", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// Broadcast updated available agents list and profile created event
	if h.hub != nil {
		availableResp, availErr := h.controller.ListAvailableAgents(c.Request.Context())
		if availErr != nil {
			h.logger.Error("failed to list available agents", zap.Error(availErr))
		} else {
			notification, _ := ws.NewNotification(ws.ActionAgentAvailableUpdated, gin.H{
				"agents": availableResp.Agents,
			})
			h.hub.Broadcast(notification)
		}
		// Broadcast profile created so the create-task dialog picks up the new profile
		if len(resp.Profiles) > 0 {
			notification, _ := ws.NewNotification(ws.ActionAgentProfileCreated, gin.H{
				"profile": resp.Profiles[0],
			})
			h.hub.Broadcast(notification)
		}
	}

	c.JSON(http.StatusOK, resp)
}

type updateCustomTUIAgentMCPRequest struct {
	MCPStrategy string `json:"mcp_strategy"`
}

// httpListMCPStrategies serves the selectable MCP injection strategies. The
// list is served rather than hardcoded in the client so a strategy added in Go
// becomes selectable without a matching frontend edit, and so the UI can show
// each strategy's own description of the mechanism it uses.
func (h *Handlers) httpListMCPStrategies(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"strategies": mcpconfig.StrategyOptions()})
}

// httpUpdateCustomTUIAgentMCP changes an existing custom TUI agent's MCP
// injection strategy. The rest of a tui_config stays immutable; this exists so
// agents created before the field existed can opt in without being deleted and
// rebuilt.
func (h *Handlers) httpUpdateCustomTUIAgentMCP(c *gin.Context) {
	var body updateCustomTUIAgentMCPRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	resp, err := h.controller.SetCustomTUIAgentMCPStrategy(c.Request.Context(), c.Param("id"), body.MCPStrategy)
	if err != nil {
		switch err {
		case controller.ErrUnknownMCPStrategy, controller.ErrNotCustomTUIAgent:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case controller.ErrAgentNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			h.logger.Error("failed to update custom TUI agent MCP strategy", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// The agent's supports_mcp and tui_config both changed; refresh the agent
	// list so open settings tabs show the MCP editor appearing or disappearing
	// without a reload.
	if h.hub != nil {
		notification, _ := ws.NewNotification(ws.ActionAgentUpdated, gin.H{"agent": resp})
		h.hub.Broadcast(notification)
	}

	c.JSON(http.StatusOK, resp)
}
