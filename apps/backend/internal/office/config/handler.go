package config

import (
	"context"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
)

// ActiveSourceChecker reports whether a workspace has an Office config sync
// source configured, so Handler can refuse the write routes that would
// otherwise race a second reconciler over the same rows
// (AC-OFFICE-CONFIG-SYNC-005.2/005.2b). Declared locally so this package
// gains no import to internal/office/configsync; the composition root
// (internal/office/routes.go) supplies the real implementation via
// configsync.Service.HasActiveSource.
type ActiveSourceChecker interface {
	HasActiveSource(ctx context.Context, workspaceID string) (bool, error)
}

const errorKey = "error"

// Handler provides HTTP handlers for config routes.
type Handler struct {
	svc    *ConfigService
	guard  ActiveSourceChecker
	logger *logger.Logger
}

// NewHandler constructs a config Handler. guard may be nil (no config sync
// service wired), in which case no workspace is ever treated as having an
// active source.
func NewHandler(svc *ConfigService, guard ActiveSourceChecker, log *logger.Logger) *Handler {
	return &Handler{svc: svc, guard: guard, logger: log}
}

// refuseIfConfigSyncActive writes a 409 and returns true when workspaceID
// has an active Office config sync source. A guard read failure also
// refuses: the guard exists to prevent a second writer, and an unknown
// answer is not evidence that there is none.
func (h *Handler) refuseIfConfigSyncActive(c *gin.Context, workspaceID string) bool {
	if h.guard == nil {
		return false
	}
	active, err := h.guard.HasActiveSource(c.Request.Context(), workspaceID)
	if err != nil || active {
		c.JSON(http.StatusConflict, gin.H{errorKey: "config sync is the active configuration source for this workspace"})
		return true
	}
	return false
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
	workspaceID := c.Param("wsId")
	if h.refuseIfConfigSyncActive(c, workspaceID) {
		return
	}
	var bundle ConfigBundle
	if err := c.ShouldBindJSON(&bundle); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.svc.ApplyImport(c.Request.Context(), workspaceID, &bundle)
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
	workspaceID := c.Param("wsId")
	if h.refuseIfConfigSyncActive(c, workspaceID) {
		return
	}
	result, err := h.svc.ApplyIncoming(c.Request.Context(), workspaceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result})
}

func (h *Handler) syncApplyOutgoing(c *gin.Context) {
	workspaceID := c.Param("wsId")
	if h.refuseIfConfigSyncActive(c, workspaceID) {
		return
	}
	if err := h.svc.ApplyOutgoing(c.Request.Context(), workspaceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
