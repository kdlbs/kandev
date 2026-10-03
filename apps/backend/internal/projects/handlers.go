package projects

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type Handler struct {
	service *Service
}

func RegisterRoutes(router *gin.Engine, service *Service) {
	h := &Handler{service: service}
	router.POST("/api/v1/workspaces/:id/agent-projects", h.create)
	router.GET("/api/v1/workspaces/:id/agent-projects", h.list)
	router.GET("/api/v1/workspaces/:id/agent-projects/:projectID", h.get)
	router.PATCH("/api/v1/workspaces/:id/agent-projects/:projectID", h.update)
	router.POST("/api/v1/workspaces/:id/agent-projects/:projectID/archive", h.archive)
	router.POST("/api/v1/workspaces/:id/agent-projects/:projectID/restore", h.restore)
	router.DELETE("/api/v1/workspaces/:id/agent-projects/:projectID", h.delete)
	router.GET("/api/v1/workspaces/:id/agent-projects/:projectID/context/tree", h.contextTree)
	router.GET("/api/v1/workspaces/:id/agent-projects/:projectID/context/content", h.readContextFile)
	router.PUT("/api/v1/workspaces/:id/agent-projects/:projectID/context/content", h.writeContextFile)
}

func (h *Handler) contextTree(c *gin.Context) {
	entries, err := h.service.ListContext(c.Request.Context(), c.Param("id"), c.Param("projectID"), c.Query("path"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"entries": entries})
}

func (h *Handler) readContextFile(c *gin.Context) {
	content, hash, err := h.service.ReadContextFile(c.Request.Context(), c.Param("id"), c.Param("projectID"), c.Query("path"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"path": c.Query("path"), "content": content, "hash": hash})
}

func (h *Handler) writeContextFile(c *gin.Context) {
	var req struct {
		Path         string `json:"path"`
		Content      string `json:"content"`
		ExpectedHash string `json:"expected_hash"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid context file request"})
		return
	}
	hash, err := h.service.WriteContextFile(c.Request.Context(), c.Param("id"), c.Param("projectID"), req.Path, req.ExpectedHash, []byte(req.Content))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"path": req.Path, "hash": hash})
}

func (h *Handler) create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project request"})
		return
	}
	req.WorkspaceID = c.Param("id")
	project, err := h.service.Create(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, project)
}

func (h *Handler) list(c *gin.Context) {
	projects, err := h.service.List(c.Request.Context(), c.Param("id"), strings.EqualFold(c.Query("archived"), "true"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"projects": projects})
}

func (h *Handler) get(c *gin.Context) {
	project, err := h.service.Get(c.Request.Context(), c.Param("id"), c.Param("projectID"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, project)
}

func (h *Handler) update(c *gin.Context) {
	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project request"})
		return
	}
	project, err := h.service.Update(c.Request.Context(), c.Param("id"), c.Param("projectID"), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, project)
}

func (h *Handler) archive(c *gin.Context) {
	project, err := h.service.Archive(c.Request.Context(), c.Param("id"), c.Param("projectID"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, project)
}

func (h *Handler) restore(c *gin.Context) {
	project, err := h.service.Restore(c.Request.Context(), c.Param("id"), c.Param("projectID"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, project)
}

func (h *Handler) delete(c *gin.Context) {
	err := h.service.Delete(
		c.Request.Context(), c.Param("id"), c.Param("projectID"),
		strings.EqualFold(c.Query("discard_worktree_changes"), "true"),
		strings.EqualFold(c.Query("delete_context"), "true"),
	)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) writeError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrDisabled):
		status = http.StatusNotFound
	case errors.Is(err, ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, ErrInvalidProject):
		status = http.StatusBadRequest
	case errors.Is(err, ErrRevisionConflict), errors.Is(err, ErrCoordinatorConflict), errors.Is(err, ErrRepositoryInUse), errors.Is(err, ErrProjectRunning), errors.Is(err, taskservice.ErrAgentProjectCoordinatorActionRequired):
		status = http.StatusConflict
	case errors.Is(err, ErrContextConflict):
		status = http.StatusConflict
	case errors.Is(err, ErrInvalidContextPath):
		status = http.StatusBadRequest
	case errors.Is(err, ErrContextNotFound):
		status = http.StatusNotFound
	case errors.Is(err, ErrExecutorIncompatible), errors.Is(err, ErrDependenciesMissing):
		status = http.StatusUnprocessableEntity
	default:
		if taskservice.IsForbidden(err) {
			status = http.StatusForbidden
		}
	}
	c.JSON(status, gin.H{"error": err.Error()})
}
