package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/orchestrate/dto"
	"github.com/kandev/kandev/internal/orchestrate/models"
)

func (h *Handlers) listSkills(c *gin.Context) {
	skills, err := h.ctrl.Svc.ListSkills(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.SkillListResponse{Skills: skills})
}

func (h *Handlers) createSkill(c *gin.Context) {
	var req dto.CreateSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	skill := &models.Skill{
		WorkspaceID:              c.Param("wsId"),
		Name:                     req.Name,
		Slug:                     req.Slug,
		Description:              req.Description,
		SourceType:               req.SourceType,
		SourceLocator:            req.SourceLocator,
		Content:                  req.Content,
		FileInventory:            req.FileInventory,
		CreatedByAgentInstanceID: req.CreatedByAgentInstanceID,
	}
	ctx := c.Request.Context()
	if err := h.ctrl.Svc.ValidateAndPrepareSkill(ctx, skill); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.ctrl.Svc.CreateSkill(ctx, skill); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, dto.SkillResponse{Skill: skill})
}

func (h *Handlers) getSkill(c *gin.Context) {
	skill, err := h.ctrl.Svc.GetSkill(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.SkillResponse{Skill: skill})
}

func (h *Handlers) updateSkill(c *gin.Context) {
	// Bind first to fail fast on malformed input before hitting the database.
	var req dto.UpdateSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	skill, err := h.ctrl.Svc.GetSkill(ctx, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	applySkillUpdates(skill, &req)
	if err := h.ctrl.Svc.ValidateSkillUpdate(ctx, skill); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.ctrl.Svc.UpdateSkill(ctx, skill); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.SkillResponse{Skill: skill})
}

func (h *Handlers) deleteSkill(c *gin.Context) {
	if err := h.ctrl.Svc.DeleteSkill(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handlers) importSkill(c *gin.Context) {
	var req dto.ImportSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Source == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source is required"})
		return
	}
	result, err := h.ctrl.Svc.ImportFromSource(c.Request.Context(), c.Param("wsId"), req.Source, nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, dto.ImportSkillResponse{
		Skills:   result.Skills,
		Warnings: result.Warnings,
	})
}

func (h *Handlers) getSkillFile(c *gin.Context) {
	path := c.DefaultQuery("path", "SKILL.md")
	content, err := h.ctrl.Svc.GetSkillFile(c.Request.Context(), c.Param("id"), path)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.SkillFileResponse{Path: path, Content: content})
}

func applySkillUpdates(skill *models.Skill, req *dto.UpdateSkillRequest) {
	if req.Name != nil {
		skill.Name = *req.Name
	}
	if req.Slug != nil {
		skill.Slug = *req.Slug
	}
	if req.Description != nil {
		skill.Description = *req.Description
	}
	if req.SourceType != nil {
		skill.SourceType = *req.SourceType
	}
	if req.SourceLocator != nil {
		skill.SourceLocator = *req.SourceLocator
	}
	if req.Content != nil {
		skill.Content = *req.Content
	}
	if req.FileInventory != nil {
		skill.FileInventory = *req.FileInventory
	}
}
