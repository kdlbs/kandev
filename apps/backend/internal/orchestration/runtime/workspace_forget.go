package runtime

import "github.com/gin-gonic/gin"

func (h *Handler) forgetWorkspaceContext(c *gin.Context) {
	b, ok := h.humanAssistant(c)
	if !ok {
		return
	}
	var req workspaceGrantRequest
	if c.ShouldBindJSON(&req) != nil {
		c.AbortWithStatus(400)
		return
	}
	if req.ExpectedBindingVersion != b.Version {
		c.AbortWithStatus(409)
		return
	}
	if err := h.Service.Repo.ForgetWorkspaceContext(c.Request.Context(), b, c.Param("workspaceId"), req.ExpectedRevision); err != nil {
		workspaceGrantFailure(c, err)
		return
	}
	h.Service.notifyAssistantUpdated(c.Request.Context(), b.ID)
	c.JSON(200, gin.H{workspaceForgotten: true, revisionResponseKey: req.ExpectedRevision + 1})
}

func (h *Handler) workspaceExports(c *gin.Context) {
	b, ok := h.humanAssistant(c)
	if !ok {
		return
	}
	scope, after, limit, ok := workspaceGrantPage(c, b, "workspace-exports-v1")
	if !ok {
		return
	}
	rows, err := h.Service.Repo.WorkspaceExports(c.Request.Context(), b.ID, b.ConversationID, after, limit+1)
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
