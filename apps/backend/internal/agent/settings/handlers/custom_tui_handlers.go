package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/settings/controller"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type createCustomTUIAgentRequest struct {
	DisplayName string `json:"display_name"`
	Model       string `json:"model"`
	Command     string `json:"command"`
	Description string `json:"description"`
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

	resp, err := h.controller.CreateCustomTUIAgent(c.Request.Context(), controller.CreateCustomTUIAgentRequest{
		DisplayName: body.DisplayName,
		Model:       body.Model,
		Command:     body.Command,
		Description: body.Description,
	})
	if err != nil {
		switch err {
		case controller.ErrInvalidSlug, controller.ErrCommandRequired:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case controller.ErrAgentAlreadyExists:
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			h.logger.Error("failed to create custom TUI agent", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// Broadcast updated available agents list
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
	}

	c.JSON(http.StatusOK, resp)
}
