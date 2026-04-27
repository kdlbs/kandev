package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/orchestrate/dto"
)

func (h *Handlers) listInstructions(c *gin.Context) {
	files, err := h.ctrl.Svc.ListInstructions(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.InstructionListResponse{Files: files})
}

func (h *Handlers) getInstruction(c *gin.Context) {
	file, err := h.ctrl.Svc.GetInstruction(c.Request.Context(), c.Param("id"), c.Param("filename"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.InstructionFileResponse{File: file})
}

func (h *Handlers) upsertInstruction(c *gin.Context) {
	var req dto.UpsertInstructionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	agentID := c.Param("id")
	filename := c.Param("filename")
	isEntry := filename == "AGENTS.md"
	if err := h.ctrl.Svc.UpsertInstruction(c.Request.Context(), agentID, filename, req.Content, isEntry); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	file, err := h.ctrl.Svc.GetInstruction(c.Request.Context(), agentID, filename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.InstructionFileResponse{File: file})
}

func (h *Handlers) deleteInstruction(c *gin.Context) {
	if err := h.ctrl.Svc.DeleteInstruction(c.Request.Context(), c.Param("id"), c.Param("filename")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
