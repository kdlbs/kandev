package runtime

import (
	"cmp"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestration/models"
	"slices"
	"strconv"
)

func workspaceGrantPage(c *gin.Context, b *models.AssistantBinding, kind string) (string, string, int, bool) {
	scope := cursorScope(kind, b.ID, strconv.FormatInt(b.Version, 10), c.Param("workspaceId"))
	after, ok := attentionAfter(c.Query("after"), scope)
	limit, valid := boundedPageLimit(c)
	if !ok || !valid {
		c.AbortWithStatus(400)
		return "", "", 0, false
	}
	return scope, after, limit, true
}

func (h *Handler) workspaceGrants(c *gin.Context) {
	b, ok := h.humanAssistant(c)
	if !ok {
		return
	}
	scope, after, limit, ok := workspaceGrantPage(c, b, "workspace-grants-v1")
	if !ok {
		return
	}
	rows, err := h.Service.Repo.WorkspaceGrants(c.Request.Context(), b.ID, after, limit+1)
	if err != nil {
		c.AbortWithStatus(503)
		return
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = encodeScopedCursor(scope, rows[len(rows)-1].ID)
	}
	reader, ok := h.Service.Manager.(WorkspaceGrantReader)
	if !ok {
		c.AbortWithStatus(503)
		return
	}
	result := []models.WorkspaceGrantView{}
	for _, row := range rows {
		entry := models.WorkspaceGrantView{WorkspaceGrant: row, Reason: h.Service.workspaceGrantReason(c.Request.Context(), b, &row)}
		workspace, err := reader.WorkspaceGrantAccess(c.Request.Context(), row.WorkspaceID, false)
		if err != nil {
			entry.Reason = "workspace_unavailable"
		} else {
			entry.WorkspaceName = workspace.Name
		}
		entry.Active = entry.Reason == ""
		result = append(result, entry)
	}
	c.JSON(200, gin.H{entriesKey: result, nextCursorKey: next})
}

func (h *Handler) workspaceGrantOptions(c *gin.Context) {
	b, ok := h.humanAssistant(c)
	if !ok {
		return
	}
	scope, after, limit, ok := workspaceGrantPage(c, b, "workspace-options-v1")
	if !ok {
		return
	}
	receiver, err := h.Service.workspaceGrantReceiver(c.Request.Context(), b)
	if err != nil {
		c.AbortWithStatus(503)
		return
	}
	reader, ok := h.Service.Manager.(WorkspaceGrantReader)
	if !ok {
		c.AbortWithStatus(503)
		return
	}
	rows, err := reader.WorkspaceGrantOptions(c.Request.Context())
	if err != nil {
		c.AbortWithStatus(503)
		return
	}
	slices.SortFunc(rows, func(a, b models.WorkspaceGrantOption) int { return cmp.Compare(a.ID, b.ID) })
	result := []models.WorkspaceGrantOption{}
	for _, row := range rows {
		if row.ID > after && row.ID != b.WorkspaceID {
			result = append(result, row)
			if len(result) > limit {
				break
			}
		}
	}
	next := ""
	if len(result) > limit {
		result = result[:limit]
		next = encodeScopedCursor(scope, result[len(result)-1].ID)
	}
	c.JSON(200, gin.H{entriesKey: result, nextCursorKey: next, "receiver": receiver})
}

func (h *Handler) workspaceGrantEvents(c *gin.Context) {
	b, ok := h.humanAssistant(c)
	if !ok {
		return
	}
	scope, after, limit, ok := workspaceGrantPage(c, b, "workspace-grant-events-v1")
	if !ok {
		return
	}
	rows, err := h.Service.Repo.WorkspaceGrantEvents(c.Request.Context(), b.ID, c.Param("workspaceId"), after, limit+1)
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
