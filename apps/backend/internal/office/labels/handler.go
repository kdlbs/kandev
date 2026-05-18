package labels

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler provides HTTP handlers for label routes.
type Handler struct {
	svc *LabelService
}

// NewHandler creates a new Handler.
func NewHandler(svc *LabelService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers label routes on the given router group.
func RegisterRoutes(api *gin.RouterGroup, svc *LabelService) {
	h := NewHandler(svc)
	api.GET("/workspaces/:wsId/tasks/:taskId/labels", h.listTaskLabels)
	api.POST("/workspaces/:wsId/tasks/:taskId/labels", h.addLabel)
	api.DELETE("/workspaces/:wsId/tasks/:taskId/labels/:labelName", h.removeLabel)
	api.GET("/workspaces/:wsId/labels", h.listWorkspaceLabels)
	api.PATCH("/workspaces/:wsId/labels/:id", h.updateLabel)
	api.DELETE("/workspaces/:wsId/labels/:id", h.deleteLabel)
}

type addLabelRequest struct {
	Name string `json:"name" binding:"required"`
}

func (h *Handler) listTaskLabels(c *gin.Context) {
	lbls, err := h.svc.ListLabelsForTask(c.Request.Context(), c.Param("taskId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"labels": lbls})
}

func (h *Handler) addLabel(c *gin.Context) {
	var req addLabelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	lbl, err := h.svc.AddLabelToTask(
		c.Request.Context(),
		c.Param("taskId"),
		c.Param("wsId"),
		req.Name,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"label": lbl})
}

func (h *Handler) removeLabel(c *gin.Context) {
	if err := h.svc.RemoveLabelFromTask(
		c.Request.Context(),
		c.Param("taskId"),
		c.Param("wsId"),
		c.Param("labelName"),
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) listWorkspaceLabels(c *gin.Context) {
	lbls, err := h.svc.ListWorkspaceLabels(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"labels": lbls})
}

type updateLabelRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (h *Handler) updateLabel(c *gin.Context) {
	var req updateLabelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.UpdateLabel(c.Request.Context(), c.Param("id"), req.Name, req.Color); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) deleteLabel(c *gin.Context) {
	if err := h.svc.DeleteLabel(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
