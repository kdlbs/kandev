package dashboard

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
)

// splitParticipantsByRole splits a participant list into reviewer and
// approver agent-profile-id slices. Both returned slices are non-nil so
// the JSON response always renders empty arrays, never `null`.
func splitParticipantsByRole(parts []sqlite.Participant) (reviewers, approvers []string) {
	reviewers = []string{}
	approvers = []string{}
	for _, p := range parts {
		switch p.Role {
		case models.ParticipantRoleReviewer:
			reviewers = append(reviewers, p.AgentProfileID)
		case models.ParticipantRoleApprover:
			approvers = append(approvers, p.AgentProfileID)
		}
	}
	return reviewers, approvers
}

// -- Blockers --

// addBlockerRequest is the request body for POST /tasks/:id/blockers.
type addBlockerRequest struct {
	BlockerTaskID string `json:"blocker_task_id"`
}

func (h *Handler) addTaskBlocker(c *gin.Context) {
	var req addBlockerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.BlockerTaskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "blocker_task_id is required"})
		return
	}
	taskID := c.Param("id")
	if err := h.svc.AddTaskBlocker(c.Request.Context(), taskID, req.BlockerTaskID); err != nil {
		var cycleErr *BlockerCycleError
		if errors.As(err, &cycleErr) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": cycleErr.Error(),
				"cycle": cycleErr.Path,
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"task_id":         taskID,
		"blocker_task_id": req.BlockerTaskID,
	})
}

func (h *Handler) removeTaskBlocker(c *gin.Context) {
	taskID := c.Param("id")
	blockerID := c.Param("blockerId")
	if err := h.svc.RemoveTaskBlocker(c.Request.Context(), taskID, blockerID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// -- Participants (reviewers / approvers) --

// addParticipantRequest is the request body for POST /tasks/:id/{reviewers,approvers}.
type addParticipantRequest struct {
	AgentProfileID string `json:"agent_profile_id"`
}

// participantListResponse is the response body for GET /tasks/:id/{reviewers,approvers}.
type participantListResponse struct {
	AgentProfileIDs []string `json:"agent_profile_ids"`
}

func (h *Handler) listTaskReviewers(c *gin.Context) {
	h.listParticipants(c, models.ParticipantRoleReviewer)
}

func (h *Handler) addTaskReviewer(c *gin.Context) {
	h.addParticipant(c, models.ParticipantRoleReviewer)
}

func (h *Handler) removeTaskReviewer(c *gin.Context) {
	h.removeParticipant(c, models.ParticipantRoleReviewer)
}

func (h *Handler) listTaskApprovers(c *gin.Context) {
	h.listParticipants(c, models.ParticipantRoleApprover)
}

func (h *Handler) addTaskApprover(c *gin.Context) {
	h.addParticipant(c, models.ParticipantRoleApprover)
}

func (h *Handler) removeTaskApprover(c *gin.Context) {
	h.removeParticipant(c, models.ParticipantRoleApprover)
}

// listParticipants is the shared body for the two list endpoints.
func (h *Handler) listParticipants(c *gin.Context, role string) {
	parts, err := h.svc.ListTaskParticipants(c.Request.Context(), c.Param("id"), role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ids := make([]string, 0, len(parts))
	for _, p := range parts {
		ids = append(ids, p.AgentProfileID)
	}
	c.JSON(http.StatusOK, participantListResponse{AgentProfileIDs: ids})
}

// addParticipant is the shared body for the two add endpoints.
func (h *Handler) addParticipant(c *gin.Context, role string) {
	var req addParticipantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.AgentProfileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent_profile_id is required"})
		return
	}
	taskID := c.Param("id")
	caller := agentIDFromCtx(c)
	err := h.dispatchParticipantMutation(c.Request.Context(), caller, taskID, req.AgentProfileID, role, true)
	if err != nil {
		respondParticipantError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"task_id":          taskID,
		"agent_profile_id": req.AgentProfileID,
		"role":             role,
	})
}

// removeParticipant is the shared body for the two delete endpoints.
func (h *Handler) removeParticipant(c *gin.Context, role string) {
	taskID := c.Param("id")
	agentID := c.Param("agentId")
	caller := agentIDFromCtx(c)
	err := h.dispatchParticipantMutation(c.Request.Context(), caller, taskID, agentID, role, false)
	if err != nil {
		respondParticipantError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// dispatchParticipantMutation routes (role, add) to the matching service
// method. Centralised so respondParticipantError can map errors uniformly.
func (h *Handler) dispatchParticipantMutation(
	ctx context.Context, caller, taskID, agentID, role string, add bool,
) error {
	switch {
	case role == models.ParticipantRoleReviewer && add:
		return h.svc.AddTaskReviewer(ctx, caller, taskID, agentID)
	case role == models.ParticipantRoleReviewer && !add:
		return h.svc.RemoveTaskReviewer(ctx, caller, taskID, agentID)
	case role == models.ParticipantRoleApprover && add:
		return h.svc.AddTaskApprover(ctx, caller, taskID, agentID)
	case role == models.ParticipantRoleApprover && !add:
		return h.svc.RemoveTaskApprover(ctx, caller, taskID, agentID)
	}
	return errors.New("invalid participant role")
}

func respondParticipantError(c *gin.Context, err error) {
	if errors.Is(err, shared.ErrForbidden) {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}
