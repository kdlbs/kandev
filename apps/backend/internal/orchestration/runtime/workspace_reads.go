package runtime

import (
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/common/redaction"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (h *Handler) workspaceDirectory(c *gin.Context, claims *runtimeauth.AgentClaims) {
	reader, ok := h.workspaceReader(c)
	if !ok {
		return
	}
	limit, ok := boundedPageLimit(c)
	if !ok {
		return
	}
	scope := cursorScope("workspace-directory-v1", claims.TaskID, claims.WorkspaceID, c.Query("workspace_grant_revision"))
	after, err := decodeScopedCursor(c.Query("after"), scope)
	if err != nil {
		c.AbortWithStatus(400)
		return
	}
	rows, err := reader.AssistantWorkspaceDirectory(c.Request.Context(), claims.WorkspaceID)
	if err != nil {
		c.AbortWithStatus(503)
		return
	}
	profiles, err := h.Service.Repo.ExecutionProfileDirectory(c.Request.Context(), claims.WorkspaceID)
	if err != nil {
		c.AbortWithStatus(503)
		return
	}
	for _, p := range profiles {
		rows = append(rows, models.WorkspaceDirectoryEntry{ID: p["id"], Name: clip(redaction.NewRedactor().String(p["name"]), 200), Kind: workspaceProfileKind})
	}
	sort.Slice(rows, func(i, j int) bool { return directoryKey(rows[i]) < directoryKey(rows[j]) })
	entries := []models.WorkspaceDirectoryEntry{}
	for _, row := range rows {
		if directoryKey(row) > after {
			entries = append(entries, row)
			if len(entries) > limit {
				break
			}
		}
	}
	next := ""
	if len(entries) > limit {
		entries = entries[:limit]
		next = encodeScopedCursor(scope, directoryKey(entries[limit-1]))
	}
	h.workspaceResponse(c, gin.H{workspaceIDKey: claims.WorkspaceID, entriesKey: entries, nextCursorKey: next}, "directory")
}

func directoryKey(row models.WorkspaceDirectoryEntry) string { return row.Kind + ":" + row.ID }

func (h *Handler) workspaceTask(c *gin.Context, claims *runtimeauth.AgentClaims) {
	reader, ok := h.workspaceReader(c)
	if !ok {
		return
	}
	result := c.Query("include_result") == "true"
	exports := []string{workspaceTaskSummaryExport}
	if result {
		selected := c.MustGet(workspaceSelectionKey).(workspaceSelection)
		if _, err := h.Service.currentWorkspaceGrant(c.Request.Context(), selected.Binding, claims.WorkspaceID, selected.Grant.Revision, workspaceObserve, "task_result"); err != nil {
			c.AbortWithStatus(403)
			return
		}
		exports = append(exports, "task_result")
	}
	row, err := reader.AssistantWorkspaceTask(c.Request.Context(), claims.WorkspaceID, c.Param("id"), result)
	if err != nil {
		c.AbortWithStatus(404)
		return
	}
	h.workspaceResponse(c, row, exports...)
}

func (h *Handler) workspaceTasks(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	reader, ok := h.workspaceReader(c)
	if !ok {
		return
	}
	limit, ok := boundedPageLimit(c)
	if !ok {
		return
	}
	scope := cursorScope("workspace-tasks-v1", claims.TaskID, claims.WorkspaceID, c.Query("workspace_grant_revision"), strconv.Itoa(limit))
	after, err := decodeScopedCursor(c.Query("after"), scope)
	if err != nil {
		c.AbortWithStatus(400)
		return
	}
	page := 1
	if after != "" {
		page, err = strconv.Atoi(after)
		if err != nil || page < 1 || page > 100000 {
			c.AbortWithStatus(400)
			return
		}
	}
	rows, more, err := reader.AssistantWorkspaceTasks(c.Request.Context(), claims.WorkspaceID, page, limit)
	if err != nil {
		c.AbortWithStatus(503)
		return
	}
	next := ""
	if more {
		next = encodeScopedCursor(scope, strconv.Itoa(page+1))
	}
	h.workspaceResponse(c, gin.H{workspaceIDKey: claims.WorkspaceID, entriesKey: rows, nextCursorKey: next}, workspaceTaskSummaryExport)
}
