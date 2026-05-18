package projects

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/office/models"
)

// Handler provides HTTP handlers for project routes.
type Handler struct {
	svc *ProjectService
}

// NewHandler creates a new Handler backed by the given ProjectService.
func NewHandler(svc *ProjectService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers project HTTP routes on the given router group.
func RegisterRoutes(api *gin.RouterGroup, h *Handler) {
	api.GET("/workspaces/:wsId/projects", h.listProjects)
	api.POST("/workspaces/:wsId/projects", h.createProject)
	api.GET("/projects/:id", h.getProject)
	api.PATCH("/projects/:id", h.updateProject)
	api.DELETE("/projects/:id", h.deleteProject)
}

func (h *Handler) listProjects(c *gin.Context) {
	projects, err := h.svc.ListProjectsWithCountsFromConfig(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ProjectListResponse{Projects: projects})
}

func (h *Handler) createProject(c *gin.Context) {
	wsID := c.Param("wsId")
	var req CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	reposJSON, err := models.EncodeRepositories(req.Repositories)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	project := &Project{
		WorkspaceID:        wsID,
		Name:               req.Name,
		Description:        req.Description,
		Status:             ProjectStatusActive,
		LeadAgentProfileID: req.LeadAgentProfileID,
		Color:              req.Color,
		BudgetCents:        req.BudgetCents,
		Repositories:       reposJSON,
		ExecutorConfig:     req.ExecutorConfig,
	}
	if err := h.svc.CreateProject(c.Request.Context(), project); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, ProjectResponse{Project: project})
}

func (h *Handler) getProject(c *gin.Context) {
	project, err := h.svc.GetProjectFromConfig(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ProjectResponse{Project: project})
}

func (h *Handler) updateProject(c *gin.Context) {
	project, statusCode, err := h.doUpdateProject(c)
	if err != nil {
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ProjectResponse{Project: project})
}

func (h *Handler) doUpdateProject(c *gin.Context) (*Project, int, error) {
	ctx := c.Request.Context()
	project, err := h.svc.GetProjectFromConfig(ctx, c.Param("id"))
	if err != nil {
		return nil, http.StatusNotFound, err
	}
	var req UpdateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, http.StatusBadRequest, err
	}
	if err := applyProjectUpdates(project, &req); err != nil {
		return nil, http.StatusBadRequest, err
	}
	if err := h.svc.UpdateProject(ctx, project); err != nil {
		return nil, http.StatusInternalServerError, err
	}
	return project, http.StatusOK, nil
}

func (h *Handler) deleteProject(c *gin.Context) {
	if err := h.svc.DeleteProject(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func applyProjectUpdates(project *Project, req *UpdateProjectRequest) error {
	if req.Name != nil {
		project.Name = *req.Name
	}
	if req.Description != nil {
		project.Description = *req.Description
	}
	if req.Status != nil {
		project.Status = ProjectStatus(*req.Status)
	}
	if req.LeadAgentProfileID != nil {
		project.LeadAgentProfileID = *req.LeadAgentProfileID
	}
	if req.Color != nil {
		project.Color = *req.Color
	}
	if req.BudgetCents != nil {
		project.BudgetCents = *req.BudgetCents
	}
	if req.Repositories != nil {
		encoded, err := models.EncodeRepositories(*req.Repositories)
		if err != nil {
			return err
		}
		project.Repositories = encoded
	}
	if req.ExecutorConfig != nil {
		project.ExecutorConfig = *req.ExecutorConfig
	}
	return nil
}
