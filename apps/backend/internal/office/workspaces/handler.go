package workspaces

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/office/service"
)

// Handler provides workspace-level office operations.
type Handler struct {
	svc *service.Service
}

// NewHandler creates a workspace handler.
func NewHandler(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers workspace deletion routes.
func RegisterRoutes(api *gin.RouterGroup, h *Handler) {
	api.GET("/workspaces/:wsId/deletion-summary", h.getDeletionSummary)
	api.DELETE("/workspaces/:wsId", h.deleteWorkspace)
}

func (h *Handler) getDeletionSummary(c *gin.Context) {
	summary, err := h.svc.GetWorkspaceDeletionSummary(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, summary)
}

func (h *Handler) deleteWorkspace(c *gin.Context) {
	var req struct {
		ConfirmName string `json:"confirm_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ConfirmName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "confirm_name is required"})
		return
	}

	// Verify the confirmation name matches the actual workspace name.
	summary, err := h.svc.GetWorkspaceDeletionSummary(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if req.ConfirmName != summary.WorkspaceName {
		c.JSON(http.StatusBadRequest, gin.H{"error": "confirm_name does not match workspace name"})
		return
	}

	if err := h.svc.DeleteWorkspace(c.Request.Context(), c.Param("wsId")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
