package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestration/models"
	"strconv"
)

func (h *Handler) attention(c *gin.Context) {
	var b *models.AssistantBinding
	var ok bool
	if _, agent := c.Get("agent_claims"); agent {
		_, b, ok = h.runtimeAssistant(c)
	} else {
		b, ok = h.humanAssistant(c)
	}
	if !ok {
		return
	}
	limit, valid := boundedPageLimit(c)
	if !valid {
		c.AbortWithStatus(400)
		return
	}
	scope := cursorScope("attention-v1", b.OwnerUserID, b.ID, strconv.FormatInt(b.Version, 10))
	after, valid := attentionAfter(c.Query("after"), scope)
	if !valid {
		c.AbortWithStatus(400)
		return
	}
	rows, err := h.Service.Repo.AttentionPage(c.Request.Context(), b.ID, after, limit+1)
	if err != nil {
		c.AbortWithStatus(503)
		return
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		raw, _ := json.Marshal(capabilityCursor{Scope: scope, After: rows[len(rows)-1].ID})
		next = base64.RawURLEncoding.EncodeToString(raw)
	}
	visible := make([]models.Attention, 0, len(rows))
	for _, row := range rows {
		if h.Service.attentionVisible(c.Request.Context(), b, row) {
			visible = append(visible, row)
		}
	}
	c.JSON(200, gin.H{"entries": visible, nextCursorKey: next, "binding_version": b.Version})
}
func attentionAfter(raw, scope string) (string, bool) {
	if raw == "" {
		return "", true
	}
	var cursor capabilityCursor
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if len(raw) > 2048 || err != nil || json.Unmarshal(data, &cursor) != nil || cursor.Scope != scope || cursor.After == "" || len(cursor.After) > 200 {
		return "", false
	}
	return cursor.After, true
}

func (s *Service) attentionVisible(ctx context.Context, b *models.AssistantBinding, row models.Attention) bool {
	task, err := s.Tasks.GetTask(ctx, row.TaskID)
	if err != nil || task.WorkspaceID != b.WorkspaceID {
		return false
	}
	targets, err := s.Repo.AttentionTargets(ctx, row.TaskID, "", 100)
	if err != nil {
		return false
	}
	for _, target := range targets {
		if target.BindingID == b.ID {
			return true
		}
	}
	return false
}
