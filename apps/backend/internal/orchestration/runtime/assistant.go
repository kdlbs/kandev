package runtime

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func assistantHuman(c *gin.Context) (authn.Identity, bool) {
	if _, agent := c.Get("agent_claims"); agent {
		c.AbortWithStatus(http.StatusForbidden)
		return authn.Identity{}, false
	}
	identity, ok := authn.FromGin(c)
	if !ok || identity.UserID == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return identity, false
	}
	return identity, true
}

func (h *Handler) privateConversationAllowed(c *gin.Context, taskID string) bool {
	owner, err := h.Service.Repo.ConversationUserOwner(c.Request.Context(), taskID)
	if err != nil {
		return false
	}
	if owner == "" {
		return true
	}
	identity, ok := authn.FromGin(c)
	return ok && identity.UserID == owner
}

func (h *Handler) privateRuntimeAllowed(c *gin.Context, claims *runtimeauth.AgentClaims, taskID string) bool {
	owner, err := h.Service.Repo.ConversationUserOwner(c.Request.Context(), taskID)
	if err != nil || (owner != "" && claims.TaskID != taskID) {
		c.AbortWithStatus(http.StatusNotFound)
		return false
	}
	return true
}

func (h *Handler) assistant(c *gin.Context) {
	identity, ok := assistantHuman(c)
	if !ok {
		return
	}
	row, err := h.Service.Repo.AssistantBinding(c.Request.Context(), identity.UserID)
	if err != nil || !h.assistantWorkspaceAllowed(c, row.WorkspaceID) {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.JSON(http.StatusOK, row)
}

func (h *Handler) assistantWorkspaceAllowed(c *gin.Context, workspaceID string) bool {
	return h.Authorize == nil || h.Authorize(c.Request.Context(), workspaceID) == nil
}

func (h *Handler) selectAssistant(c *gin.Context) {
	identity, ok := assistantHuman(c)
	if !ok {
		return
	}
	var req struct {
		OrchestratorID string `json:"orchestrator_id"`
		Expected       int64  `json:"expected_version"`
		ExecutionMode  string `json:"execution_mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.OrchestratorID == "" || req.Expected < 0 {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	ctx := c.Request.Context()
	persona, err := h.Service.Personas.GetAgentInstance(ctx, req.OrchestratorID)
	if err != nil || !h.assistantWorkspaceAllowed(c, persona.WorkspaceID) {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	role, err := h.Service.Repo.OrchestratorRoleID(ctx, persona.ID)
	if err != nil || role == "" {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	conversation, err := h.Service.Repo.EnsureAgentConversation(ctx, persona)
	if err != nil {
		fail(c, err)
		return
	}
	row := &models.AssistantBinding{OwnerUserID: identity.UserID, OrchestratorID: persona.ID,
		WorkspaceID: persona.WorkspaceID, ConversationID: conversation.TaskID, ExecutionMode: req.ExecutionMode}
	if err := h.Service.Repo.SelectAssistant(ctx, row, req.Expected); err != nil {
		if errors.Is(err, models.ErrConflict) {
			c.JSON(http.StatusConflict, gin.H{errorResponseKey: "assistant_binding_conflict"})
			return
		}
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, row)
}
