package runtime

import (
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (h *Handler) assistantSnapshot(c *gin.Context, b *models.AssistantBinding) {
	ctx := c.Request.Context()
	revision, err := h.Service.Repo.IntentRevision(ctx, b.ConversationID)
	if err != nil {
		c.AbortWithStatus(503)
		return
	}
	persona, err := h.Service.Personas.GetAgentInstance(ctx, b.OrchestratorID)
	if err != nil {
		c.AbortWithStatus(503)
		return
	}
	authority, authorityErr := h.Service.assistantAuthority(ctx, b.ConversationID)
	reason := ""
	if authorityErr != nil {
		reason = "authority_unavailable"
	}
	c.JSON(200, struct {
		*models.AssistantBinding
		IntentRevision  int64                      `json:"intent_revision"`
		Paused          bool                       `json:"paused"`
		Authority       *models.AssistantAuthority `json:"authority,omitempty"`
		AuthorityReason string                     `json:"authority_reason,omitempty"`
	}{AssistantBinding: b, IntentRevision: revision, Paused: paused(persona), Authority: authority, AuthorityReason: reason})
}
