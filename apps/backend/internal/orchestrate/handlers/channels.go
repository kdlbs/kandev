package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/orchestrate/dto"
	"github.com/kandev/kandev/internal/orchestrate/models"
)

func (h *Handlers) listChannels(c *gin.Context) {
	channels, err := h.ctrl.Svc.ListChannelsByAgent(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ChannelListResponse{Channels: channels})
}

func (h *Handlers) setupChannel(c *gin.Context) {
	var req dto.CreateChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	channel := &models.Channel{
		WorkspaceID:     req.WorkspaceID,
		AgentInstanceID: c.Param("id"),
		Platform:        req.Platform,
		Config:          req.Config,
		Status:          req.Status,
	}
	if err := h.ctrl.Svc.SetupChannel(c.Request.Context(), channel); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"channel": channel})
}

func (h *Handlers) deleteChannel(c *gin.Context) {
	if err := h.ctrl.Svc.DeleteChannel(c.Request.Context(), c.Param("channelId")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handlers) deleteAllMemory(c *gin.Context) {
	if err := h.ctrl.Svc.DeleteAllMemory(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handlers) exportMemory(c *gin.Context) {
	entries, err := h.ctrl.Svc.ExportMemory(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.MemoryListResponse{Memory: entries})
}
