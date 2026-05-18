package skills

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

// Handler provides HTTP handlers for skill routes.
type Handler struct {
	svc *SkillService
}

// NewHandler creates a new Handler.
func NewHandler(svc *SkillService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers skill HTTP routes on the given router group.
func (h *Handler) RegisterRoutes(api *gin.RouterGroup) {
	api.GET("/workspaces/:wsId/skills", h.listSkills)
	api.POST("/workspaces/:wsId/skills", h.createSkill)
	api.GET("/workspaces/:wsId/skills/discover", h.discoverUserSkills)
	api.POST("/workspaces/:wsId/skills/import", h.importSkill)
	api.GET("/skills/:id", h.getSkill)
	api.GET("/skills/:id/files", h.getSkillFile)
	api.PATCH("/skills/:id", h.updateSkill)
	api.DELETE("/skills/:id", h.deleteSkill)
}

func (h *Handler) listSkills(c *gin.Context) {
	skills, err := h.svc.ListSkillsFromConfig(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, SkillListResponse{Skills: skills})
}

func (h *Handler) createSkill(c *gin.Context) {
	if !checkSkillPerm(c, shared.PermCanManageOwnSkills) {
		return
	}
	var req CreateSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	skill := &models.Skill{
		WorkspaceID:             c.Param("wsId"),
		Name:                    req.Name,
		Slug:                    req.Slug,
		Description:             req.Description,
		SourceType:              models.SkillSourceType(req.SourceType),
		SourceLocator:           req.SourceLocator,
		Content:                 req.Content,
		FileInventory:           req.FileInventory,
		CreatedByAgentProfileID: req.CreatedByAgentProfileID,
	}
	ctx := c.Request.Context()
	if err := h.svc.ValidateAndPrepareSkill(ctx, skill); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.CreateSkill(ctx, skill); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, SkillResponse{Skill: skill})
}

func (h *Handler) getSkill(c *gin.Context) {
	skill, err := h.svc.GetSkillFromConfig(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, SkillResponse{Skill: skill})
}

func (h *Handler) updateSkill(c *gin.Context) {
	if !checkSkillPerm(c, shared.PermCanManageOwnSkills) {
		return
	}
	var req UpdateSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	skill, err := h.svc.GetSkillFromConfig(ctx, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	applySkillUpdates(skill, &req)
	if err := h.svc.ValidateSkillUpdate(ctx, skill); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.UpdateSkill(ctx, skill); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, SkillResponse{Skill: skill})
}

func (h *Handler) deleteSkill(c *gin.Context) {
	if !checkSkillPerm(c, shared.PermCanManageOwnSkills) {
		return
	}
	if err := h.svc.DeleteSkill(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) importSkill(c *gin.Context) {
	if !checkSkillPerm(c, shared.PermCanManageOwnSkills) {
		return
	}
	var req ImportSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.SourceType == SkillSourceTypeUserHome {
		h.importUserHomeSkill(c, req)
		return
	}
	if req.Source == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source is required"})
		return
	}
	result, err := h.svc.ImportFromSource(c.Request.Context(), c.Param("wsId"), req.Source, nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, ImportSkillResponse{
		Skills:   result.Skills,
		Warnings: result.Warnings,
	})
}

func (h *Handler) discoverUserSkills(c *gin.Context) {
	if !checkSkillPerm(c, shared.PermCanManageOwnSkills) {
		return
	}
	provider := c.Query("provider")
	skills, err := h.svc.DiscoverUserSkills(c.Request.Context(), provider)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, DiscoveredUserSkillListResponse{Skills: skills})
}

func (h *Handler) importUserHomeSkill(c *gin.Context, req ImportSkillRequest) {
	if req.Provider == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider is required"})
		return
	}
	if req.Key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
		return
	}
	skill, err := h.svc.ImportUserHomeSkill(c.Request.Context(), c.Param("wsId"), req.Provider, req.Key)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, ImportSkillResponse{Skills: []*models.Skill{skill}})
}

func (h *Handler) getSkillFile(c *gin.Context) {
	path := c.DefaultQuery("path", "SKILL.md")
	content, err := h.svc.GetSkillFile(c.Request.Context(), c.Param("id"), path)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, SkillFileResponse{Path: path, Content: content})
}

func applySkillUpdates(skill *models.Skill, req *UpdateSkillRequest) {
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
		skill.SourceType = models.SkillSourceType(*req.SourceType)
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

// checkSkillPerm verifies the agent caller holds the given permission.
// UI (non-agent) requests always pass. Returns false and writes a 403 if denied.
func checkSkillPerm(c *gin.Context, permKey string) bool {
	val, ok := c.Get("agent_caller")
	if !ok {
		return true
	}
	caller, ok := val.(*models.AgentInstance)
	if !ok {
		return true
	}
	perms := shared.ResolvePermissions(shared.AgentRole(caller.Role), caller.Permissions)
	if !shared.HasPermission(perms, permKey) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: missing " + permKey})
		return false
	}
	return true
}
