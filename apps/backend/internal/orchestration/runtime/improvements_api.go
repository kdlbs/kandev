package runtime

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (h *Handler) improvementBinding(c *gin.Context) (*models.AssistantBinding, bool) {
	if _, runtime := c.Get("agent_claims"); runtime {
		_, b, ok := h.runtimeAssistant(c)
		return b, ok
	}
	return h.humanAssistant(c)
}

func (h *Handler) improvements(c *gin.Context) {
	b, ok := h.improvementBinding(c)
	if !ok {
		return
	}
	limit, ok := boundedPageLimit(c)
	scope := cursorScope("improvements-v1", b.ID, strconv.FormatInt(b.Version, 10))
	after, valid := attentionAfter(c.Query("after"), scope)
	if !ok || !valid {
		c.AbortWithStatus(400)
		return
	}
	rows, err := h.Service.Repo.ImprovementCandidates(c.Request.Context(), b.ID, after, limit+1)
	if err != nil {
		c.AbortWithStatus(503)
		return
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = encodeScopedCursor(scope, rows[len(rows)-1].ID)
	}
	c.JSON(200, gin.H{entriesKey: rows, nextCursorKey: next})
}
