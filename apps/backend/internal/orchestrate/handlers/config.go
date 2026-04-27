package handlers

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/orchestrate/service"
)

func (h *Handlers) exportConfig(c *gin.Context) {
	bundle, err := h.ctrl.Svc.ExportBundle(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"bundle": bundle})
}

func (h *Handlers) exportConfigZip(c *gin.Context) {
	reader, err := h.ctrl.Svc.ExportZip(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", "attachment; filename=kandev-config.zip")
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, reader)
}

func (h *Handlers) previewImport(c *gin.Context) {
	var bundle service.ConfigBundle
	if err := c.ShouldBindJSON(&bundle); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	preview, err := h.ctrl.Svc.PreviewImport(c.Request.Context(), c.Param("wsId"), &bundle)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"preview": preview})
}

func (h *Handlers) applyImport(c *gin.Context) {
	var bundle service.ConfigBundle
	if err := c.ShouldBindJSON(&bundle); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.ctrl.Svc.ApplyImport(c.Request.Context(), c.Param("wsId"), &bundle)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result})
}

func (h *Handlers) syncIncomingDiff(c *gin.Context) {
	diff, err := h.ctrl.Svc.IncomingDiff(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"diff": diff})
}

func (h *Handlers) syncOutgoingDiff(c *gin.Context) {
	diff, err := h.ctrl.Svc.OutgoingDiff(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"diff": diff})
}

func (h *Handlers) syncApplyIncoming(c *gin.Context) {
	result, err := h.ctrl.Svc.ApplyIncoming(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result})
}

func (h *Handlers) syncApplyOutgoing(c *gin.Context) {
	if err := h.ctrl.Svc.ApplyOutgoing(c.Request.Context(), c.Param("wsId")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
