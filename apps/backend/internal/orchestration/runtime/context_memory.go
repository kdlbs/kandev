package runtime

import (
	"net/url"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/redaction"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func contextScopeQuery(c *gin.Context) models.ContextScope {
	return models.ContextScope{ProfileID: c.Query("profile_id"), ProjectID: c.Query("project_id"), EnvironmentID: c.Query("environment_id"), TaskID: c.Query("task_id")}
}

func contextMemoryReference(objective string, scope models.ContextScope) string {
	values := url.Values{"profile_id": {scope.ProfileID}}
	for key, value := range map[string]string{"project_id": scope.ProjectID, "environment_id": scope.EnvironmentID, "task_id": scope.TaskID} {
		if value != "" {
			values.Set(key, value)
		}
	}
	return "/api/v1/orchestration/runtime/context/" + url.PathEscape(objective) + "/memory?" + values.Encode()
}

func (h *Handler) contextMemory(c *gin.Context) {
	_, b, ok := h.runtimeAssistant(c)
	if !ok {
		return
	}
	packet, err := h.Service.buildContext(c.Request.Context(), b, c.Param("id"), contextScopeQuery(c))
	if err != nil {
		c.AbortWithStatus(422)
		return
	}
	limit, ok := boundedPageLimit(c)
	if !ok {
		return
	}
	scope := cursorScope("context-memory-v1", b.OwnerUserID, packet.ID, "id-asc")
	after, err := decodeScopedCursor(c.Query("after"), scope)
	if err != nil {
		c.AbortWithStatus(400)
		return
	}
	rows, err := h.Service.Repo.ListAgentMemory(c.Request.Context(), b.OrchestratorID)
	if err != nil {
		c.AbortWithStatus(503)
		return
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	entries := make([]models.ContextMemory, 0, limit+1)
	for _, row := range rows {
		if row.ID <= after || !row.MatchesContext(b, packet.ContextScope) || !h.Service.memoryVisible(c.Request.Context(), b, row) {
			continue
		}
		entries = append(entries, models.ContextMemory{ID: row.ID, Revision: row.Revision, Scope: row.Scope, ScopeID: row.ScopeID, SourceCommentID: row.SourceCommentID, Confirmed: row.Confirmed, Content: redaction.NewRedactor().String(row.Content)})
		if len(entries) > limit {
			break
		}
	}
	next := ""
	if len(entries) > limit {
		entries = entries[:limit]
		next = encodeScopedCursor(scope, entries[limit-1].ID)
	}
	c.JSON(200, gin.H{memoryResponseKey: entries, nextCursorKey: next, "context_ref": packet.ID})
}
