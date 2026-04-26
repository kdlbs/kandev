package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/orchestrate/dto"
	"github.com/kandev/kandev/internal/orchestrate/models"
)

func (h *Handlers) listProjects(c *gin.Context) {
	projects, err := h.ctrl.Svc.ListProjectsWithCountsFromConfig(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ProjectListResponse{Projects: projects})
}

func (h *Handlers) createProject(c *gin.Context) {
	wsID := c.Param("wsId")
	var req dto.CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	project := &models.Project{
		WorkspaceID:         wsID,
		Name:                req.Name,
		Description:         req.Description,
		Status:              models.ProjectStatusActive,
		LeadAgentInstanceID: req.LeadAgentInstanceID,
		Color:               req.Color,
		BudgetCents:         req.BudgetCents,
		Repositories:        req.Repositories,
		ExecutorConfig:      req.ExecutorConfig,
	}
	if err := h.ctrl.Svc.CreateProject(c.Request.Context(), project); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, dto.ProjectResponse{Project: project})
}

func (h *Handlers) getProject(c *gin.Context) {
	project, err := h.ctrl.Svc.GetProjectFromConfig(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ProjectResponse{Project: project})
}

func (h *Handlers) updateProject(c *gin.Context) {
	project, statusCode, err := h.doUpdateProject(c)
	if err != nil {
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ProjectResponse{Project: project})
}

func (h *Handlers) doUpdateProject(c *gin.Context) (*models.Project, int, error) {
	ctx := c.Request.Context()
	project, err := h.ctrl.Svc.GetProject(ctx, c.Param("id"))
	if err != nil {
		return nil, http.StatusNotFound, err
	}
	var req dto.UpdateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, http.StatusBadRequest, err
	}
	applyProjectUpdates(project, &req)
	if err := h.ctrl.Svc.UpdateProject(ctx, project); err != nil {
		return nil, http.StatusInternalServerError, err
	}
	return project, http.StatusOK, nil
}

func (h *Handlers) deleteProject(c *gin.Context) {
	if err := h.ctrl.Svc.DeleteProject(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func applyProjectUpdates(project *models.Project, req *dto.UpdateProjectRequest) {
	if req.Name != nil {
		project.Name = *req.Name
	}
	if req.Description != nil {
		project.Description = *req.Description
	}
	if req.Status != nil {
		project.Status = models.ProjectStatus(*req.Status)
	}
	if req.LeadAgentInstanceID != nil {
		project.LeadAgentInstanceID = *req.LeadAgentInstanceID
	}
	if req.Color != nil {
		project.Color = *req.Color
	}
	if req.BudgetCents != nil {
		project.BudgetCents = *req.BudgetCents
	}
	if req.Repositories != nil {
		project.Repositories = *req.Repositories
	}
	if req.ExecutorConfig != nil {
		project.ExecutorConfig = *req.ExecutorConfig
	}
}
