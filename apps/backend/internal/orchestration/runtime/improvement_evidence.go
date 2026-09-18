package runtime

import (
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestration/models"
	"strconv"
)

func (h *Handler) improvementEvidence(c *gin.Context) {
	b, ok := h.improvementBinding(c)
	if !ok {
		return
	}
	row, err := h.Service.Repo.ImprovementCandidate(c.Request.Context(), b.ID, c.Param("id"))
	if err != nil || row.WorkspaceID != b.WorkspaceID {
		c.AbortWithStatus(404)
		return
	}
	rows, next, ok := h.improvementEvidencePage(c, b, row)
	if ok {
		c.JSON(200, gin.H{entriesKey: rows, nextCursorKey: next})
	}
}

func (h *Handler) improvementEvidencePage(c *gin.Context, b *models.AssistantBinding, row *models.ImprovementCandidate) ([]models.Friction, string, bool) {
	limit, valid := boundedPageLimit(c)
	scope := cursorScope("improvement-evidence-v1", b.ID, row.ID, strconv.FormatInt(b.Version, 10))
	after, ok := attentionAfter(c.Query("after"), scope)
	if !valid || !ok {
		c.AbortWithStatus(400)
		return nil, "", false
	}
	rows, err := h.Service.Repo.CandidateEvidence(c.Request.Context(), row, after, limit+1)
	if err != nil {
		c.AbortWithStatus(503)
		return nil, "", false
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = encodeScopedCursor(scope, rows[len(rows)-1].ID)
	}
	visible := []models.Friction{}
	for _, entry := range rows {
		task, err := h.Service.Tasks.GetTask(c.Request.Context(), entry.TaskID)
		if err == nil && task.WorkspaceID == b.WorkspaceID {
			visible = append(visible, entry)
		}
	}
	return visible, next, true
}
