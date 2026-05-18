package tree_controls

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/office/service"
)

type Handler struct {
	svc *service.Service
}

func NewHandler(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

func RegisterRoutes(api *gin.RouterGroup, h *Handler) {
	if h == nil || h.svc == nil {
		return
	}
	api.POST("/tasks/:id/tree/preview", h.handlePreview)
	api.POST("/tasks/:id/tree/pause", h.handlePause)
	api.POST("/tasks/:id/tree/resume", h.handleResume)
	api.POST("/tasks/:id/tree/cancel", h.handleCancel)
	api.POST("/tasks/:id/tree/restore", h.handleRestore)
	api.GET("/tasks/:id/tree/cost-summary", h.handleCostSummary)
}

func (h *Handler) handlePreview(c *gin.Context) {
	preview, err := h.svc.PreviewTaskTree(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, preview)
}

func (h *Handler) handlePause(c *gin.Context) {
	hold, err := h.svc.PauseTaskTree(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"hold": hold})
}

func (h *Handler) handleResume(c *gin.Context) {
	hold, err := h.svc.ResumeTaskTree(c.Request.Context(), c.Param("id"), "user:local")
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"hold": hold})
}

func (h *Handler) handleCancel(c *gin.Context) {
	hold, err := h.svc.CancelTaskTree(c.Request.Context(), c.Param("id"), "user:local")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"hold": hold})
}

func (h *Handler) handleRestore(c *gin.Context) {
	hold, err := h.svc.RestoreTaskTree(c.Request.Context(), c.Param("id"), "user:local")
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"hold": hold})
}

func (h *Handler) handleCostSummary(c *gin.Context) {
	summary, err := h.svc.GetSubtreeCostSummary(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, summary)
}
