package runtime

import (
	"net/url"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/redaction"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func contextScopeQuery(c *gin.Context) models.ContextScope {
	return models.ContextScope{ProfileID: c.Query("profile_id"), ProjectID: c.Query("project_id"), EnvironmentID: c.Query("environment_id"), TaskID: c.Query(taskIDKey)}
}

func contextMemoryReference(objective string, scope models.ContextScope) string {
	values := url.Values{"profile_id": {scope.ProfileID}}
	for key, value := range map[string]string{"project_id": scope.ProjectID, "environment_id": scope.EnvironmentID, taskIDKey: scope.TaskID} {
		if value != "" {
			values.Set(key, value)
		}
	}
	return "/api/v1/orchestration/runtime/context/" + url.PathEscape(objective) + "/memory?" + values.Encode()
}

func (h *Handler) contextMemory(c *gin.Context) {
	claims, b, ok := h.runtimeAssistant(c)
	if !ok {
		return
	}
	if !h.contextObjectiveTarget(c, b, claims.WorkspaceID) {
		return
	}
	packet, err := h.Service.buildContext(c.Request.Context(), b, c.Param("id"), contextScopeQuery(c))
	if err != nil {
		c.AbortWithStatus(422)
		return
	}
	if packet.WorkspaceID != claims.WorkspaceID {
		c.AbortWithStatus(403)
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
		if !contextMemoryMatches(row, b, packet, after) || !h.Service.memoryVisible(c.Request.Context(), b, row) {
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
	h.workspaceResponse(c, gin.H{memoryResponseKey: entries, nextCursorKey: next, "context_ref": packet.ID}, "handoff")
}

func contextMemoryMatches(row *models.AgentMemory, b *models.AssistantBinding, packet *models.ContextPacket, after string) bool {
	if packet.WorkspaceID != b.WorkspaceID && row.Scope != authorTypeUser {
		return false
	}
	return row.ID > after && row.MatchesContext(b, packet.ContextScope)
}
