package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

func (h *TaskHandlers) registerWorkspaceAdoptionHTTP(router *gin.Engine) {
	group := router.Group("/api/v1/tasks/:id/workspace-adoption", authn.RequireAdmin(), h.rejectAdoptionAgent)
	group.POST("/preview", h.httpPreviewWorkspaceAdoption)
	group.POST("/apply", h.httpApplyWorkspaceAdoption)
	group.GET("/:operation_id", h.httpValidateWorkspaceAdoption)
}

func (h *TaskHandlers) rejectAdoptionAgent(c *gin.Context) {
	if h.workspaceAdoptionCallerGuard == nil || !h.workspaceAdoptionCallerGuard(c) {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error_code": "WORKSPACE_ADOPTION_FORBIDDEN"})
		return
	}
	c.Next()
}

func adoptionDecode(c *gin.Context, value any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 256*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error_code": "WORKSPACE_ADOPTION_INVALID_REQUEST"})
		return false
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error_code": "WORKSPACE_ADOPTION_INVALID_REQUEST"})
		return false
	}
	return true
}

func adoptionResponse(c *gin.Context, value any, err error) {
	if err == nil {
		c.JSON(http.StatusOK, value)
		return
	}
	if errors.Is(err, service.ErrForbidden) {
		c.JSON(http.StatusForbidden, gin.H{"error_code": "WORKSPACE_ADOPTION_FORBIDDEN"})
		return
	}
	c.JSON(http.StatusConflict, gin.H{"error_code": "WORKSPACE_ADOPTION_REFUSED"})
}

func (h *TaskHandlers) httpPreviewWorkspaceAdoption(c *gin.Context) {
	var req service.WorkspaceAdoptionRequest
	if !adoptionDecode(c, &req) {
		return
	}
	result, err := h.service.PreviewWorkspaceAdoption(c.Request.Context(), c.Param("id"), req)
	adoptionResponse(c, result, err)
}

func (h *TaskHandlers) httpApplyWorkspaceAdoption(c *gin.Context) {
	var req struct {
		Preview     models.WorkspaceAdoptionPreview `json:"preview"`
		OperationID string                          `json:"operation_id"`
	}
	if !adoptionDecode(c, &req) {
		return
	}
	result, err := h.service.ApplyWorkspaceAdoption(c.Request.Context(), c.Param("id"), req.Preview, req.OperationID)
	adoptionResponse(c, result, err)
}

func (h *TaskHandlers) httpValidateWorkspaceAdoption(c *gin.Context) {
	result, err := h.service.ValidateWorkspaceAdoption(c.Request.Context(), c.Param("id"), c.Param("operation_id"))
	adoptionResponse(c, result, err)
}

// SetWorkspaceAdoptionCallerGuard installs the composition-owned HTTP caller check.
// An unwired handler fails closed rather than treating an unknown caller as an operator.
func (h *TaskHandlers) SetWorkspaceAdoptionCallerGuard(guard func(*gin.Context) bool) {
	h.workspaceAdoptionCallerGuard = guard
}
