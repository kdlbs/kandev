package runtime

import (
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/redaction"
	"github.com/kandev/kandev/internal/orchestration/models"
)

type workspaceLink struct {
	ID       string                     `json:"workspace_id"`
	Name     string                     `json:"name"`
	Revision int64                      `json:"workspace_grant_revision"`
	Scope    models.WorkspaceGrantScope `json:"scope"`
}

func (h *Handler) runtimeWorkspaceLinks(c *gin.Context) {
	_, b, ok := h.runtimeAssistant(c)
	if !ok {
		return
	}
	scope, after, limit, ok := workspaceGrantPage(c, b, "workspace-links-v1")
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
	entries := []workspaceLink{}
	for _, row := range rows {
		g, err := h.Service.currentWorkspaceGrant(c.Request.Context(), b, row.WorkspaceID, row.Revision, workspaceObserve, "")
		if err != nil {
			continue
		}
		option, err := reader.WorkspaceGrantAccess(c.Request.Context(), row.WorkspaceID, false)
		if err != nil {
			continue
		}
		if _, err = h.Service.currentWorkspaceGrant(c.Request.Context(), b, row.WorkspaceID, row.Revision, workspaceObserve, ""); err != nil {
			continue
		}
		if err = h.Service.Repo.RecordWorkspaceExport(c.Request.Context(), b, g, "workspace_link"); err != nil {
			continue
		}
		entries = append(entries, workspaceLink{ID: row.WorkspaceID, Name: clip(redaction.NewRedactor().String(option.Name), 200), Revision: row.Revision, Scope: row.Scope})
	}
	if _, ok = h.caller(c); !ok {
		return
	}
	c.JSON(200, gin.H{entriesKey: entries, nextCursorKey: next})
}
