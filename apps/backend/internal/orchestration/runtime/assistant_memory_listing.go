package runtime

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/redaction"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (h *Handler) assistantMemoryPage(c *gin.Context, b *models.AssistantBinding) {
	scope := c.Query("scope")
	if scope != "" && !validMemoryScopeName(scope) {
		c.AbortWithStatus(400)
		return
	}
	scopeID := c.Query("scope_id")
	if scopeID != "" {
		var err error
		scopeID, err = h.Service.validateMemoryScope(c.Request.Context(), b, scope, scopeID)
		if err != nil {
			c.AbortWithStatus(422)
			return
		}
	}
	limit, ok := boundedPageLimit(c)
	if !ok {
		return
	}
	ownerScope := cursorScope("owner-memory-v1", b.OwnerUserID, b.ID, strconv.FormatInt(b.Version, 10), scope, scopeID, "id-asc")
	after, err := decodeScopedCursor(c.Query("after"), ownerScope)
	if err != nil {
		c.AbortWithStatus(400)
		return
	}
	rows, err := h.Service.Repo.AssistantMemoryPage(c.Request.Context(), b.OrchestratorID, b.OwnerUserID, scope, scopeID, after, limit)
	if err != nil {
		c.AbortWithStatus(503)
		return
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = encodeScopedCursor(ownerScope, rows[limit-1].ID)
	}
	visible := make([]*models.AgentMemory, 0, len(rows))
	for _, row := range rows {
		if h.Service.memoryVisible(c.Request.Context(), b, row) {
			visible = append(visible, row)
		}
	}
	if _, runtime := c.Get("agent_claims"); runtime {
		projected := make([]models.ContextMemory, 0, len(visible))
		for _, row := range visible {
			projected = append(projected, models.ContextMemory{ID: row.ID, Revision: row.Revision, Scope: row.Scope, ScopeID: row.ScopeID, SourceCommentID: row.SourceCommentID, Confirmed: row.Confirmed, Content: redaction.NewRedactor().String(row.Content)})
		}
		c.JSON(200, gin.H{memoryResponseKey: projected, nextCursorKey: next})
		return
	}
	c.JSON(200, gin.H{memoryResponseKey: visible, nextCursorKey: next, "forget_notice": "Forgetting affects future context, not text already sent to a provider or historical backups."})
}
