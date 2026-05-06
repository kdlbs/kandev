package config

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
)

// Handler provides HTTP handlers for config routes.
type Handler struct {
	svc    *ConfigService
	logger *logger.Logger
}

// NewHandler constructs a config Handler.
func NewHandler(svc *ConfigService, log *logger.Logger) *Handler {
	return &Handler{svc: svc, logger: log}
}

// RegisterRoutes registers all config HTTP routes on the given router group.
func RegisterRoutes(api *gin.RouterGroup, h *Handler) {
	api.GET("/workspaces/:wsId/config/export", h.exportConfig)
	api.GET("/workspaces/:wsId/config/export/zip", h.exportConfigZip)
	api.POST("/workspaces/:wsId/config/preview", h.previewImport)
	api.POST("/workspaces/:wsId/config/import", h.applyImport)
	api.GET("/workspaces/:wsId/config/sync/incoming", h.syncIncomingDiff)
	api.GET("/workspaces/:wsId/config/sync/outgoing", h.syncOutgoingDiff)
	api.POST("/workspaces/:wsId/config/sync/import-fs", h.syncApplyIncoming)
	api.POST("/workspaces/:wsId/config/sync/export-fs", h.syncApplyOutgoing)
}

func (h *Handler) exportConfig(c *gin.Context) {
	bundle, err := h.svc.ExportBundle(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"bundle": bundle})
}

func (h *Handler) exportConfigZip(c *gin.Context) {
	reader, err := h.svc.ExportZip(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", "attachment; filename=kandev-config.zip")
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, reader)
}

func (h *Handler) previewImport(c *gin.Context) {
	var bundle ConfigBundle
	if err := c.ShouldBindJSON(&bundle); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	preview, err := h.svc.PreviewImport(c.Request.Context(), c.Param("wsId"), &bundle)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"preview": preview})
}

func (h *Handler) applyImport(c *gin.Context) {
	var bundle ConfigBundle
	if err := c.ShouldBindJSON(&bundle); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.svc.ApplyImport(c.Request.Context(), c.Param("wsId"), &bundle)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result})
}

func (h *Handler) syncIncomingDiff(c *gin.Context) {
	diff, err := h.svc.IncomingDiff(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"diff": diff})
}

func (h *Handler) syncOutgoingDiff(c *gin.Context) {
	diff, err := h.svc.OutgoingDiff(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"diff": diff})
}

func (h *Handler) syncApplyIncoming(c *gin.Context) {
	result, err := h.svc.ApplyIncoming(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result})
}

func (h *Handler) syncApplyOutgoing(c *gin.Context) {
	if err := h.svc.ApplyOutgoing(c.Request.Context(), c.Param("wsId")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
