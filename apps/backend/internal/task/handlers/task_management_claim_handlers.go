package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	"go.uber.org/zap"
)

type taskManagementClaimResponse struct {
	Claim   *models.TaskManagementClaim          `json:"claim"`
	History []*models.TaskManagementClaimHistory `json:"history"`
}

type changeTaskManagementClaimRequest struct {
	Action                       string `json:"action"`
	ExpectedTaskResourceVersion  string `json:"expected_task_resource_version"`
	ExpectedClaimResourceVersion string `json:"expected_claim_resource_version"`
	Reason                       string `json:"reason"`
}

// httpGetTaskManagementClaim exposes the durable owner projection and bounded
// audit history without consulting the owning plugin.
func (h *TaskHandlers) httpGetTaskManagementClaim(c *gin.Context) {
	taskID := c.Param("id")
	claim, err := h.service.GetTaskManagementClaim(c.Request.Context(), taskID)
	if err != nil {
		handleNotFound(c, h.logger, err, "task not found")
		return
	}
	history, err := h.service.ListTaskManagementClaimHistory(c.Request.Context(), taskID)
	if err != nil {
		h.logger.Error("failed to load task management claim history", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load task management claim"})
		return
	}
	c.JSON(http.StatusOK, taskManagementClaimResponse{Claim: claim, History: history})
}

// httpChangeTaskManagementClaim binds takeover identity to the authenticated
// human. A request can transfer a claim to its caller or release it; target
// identities and audit actors are never accepted from the client.
func (h *TaskHandlers) httpChangeTaskManagementClaim(c *gin.Context) {
	identity, ok := authn.FromGin(c)
	if !ok || strings.TrimSpace(identity.UserID) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	var body changeTaskManagementClaimRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task management claim request"})
		return
	}
	reason := strings.TrimSpace(body.Reason)
	if (body.Action != models.TaskManagementClaimTransfer && body.Action != models.TaskManagementClaimRelease) ||
		body.ExpectedTaskResourceVersion == "" || len(body.ExpectedTaskResourceVersion) > 64 ||
		len(body.ExpectedClaimResourceVersion) > 128 || reason == "" || len(reason) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task management claim request"})
		return
	}
	change := models.TaskManagementClaimChange{
		Action: body.Action, TaskID: c.Param("id"),
		ExpectedTaskResourceVersion:  body.ExpectedTaskResourceVersion,
		ExpectedClaimResourceVersion: body.ExpectedClaimResourceVersion,
		ActorID:                      identity.UserID, Reason: reason,
	}
	if body.Action == models.TaskManagementClaimTransfer {
		change.OwnerKind = "human"
		change.OwnerActorID = identity.UserID
	}
	claim, err := h.service.ChangeTaskManagementClaim(c.Request.Context(), c.Param("id"), change)
	if err != nil {
		h.writeTaskManagementClaimError(c, err)
		return
	}
	c.JSON(http.StatusOK, claim)
}

func (h *TaskHandlers) writeTaskManagementClaimError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repoerrors.ErrTaskManagementClaimConflict),
		errors.Is(err, repoerrors.ErrTaskManagementClaimOwned),
		errors.Is(err, repoerrors.ErrTaskVersionConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "task management claim changed; reload and try again"})
	case service.IsForbidden(err):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, repoerrors.ErrTaskNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
	default:
		h.logger.Error("failed to change task management claim", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to change task management claim"})
	}
}
