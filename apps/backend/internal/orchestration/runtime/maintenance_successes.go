package runtime

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (h *Handler) maintenanceSuccesses(c *gin.Context) {
	b, ok := h.humanAssistant(c)
	if !ok {
		return
	}
	row, err := h.Service.Repo.ImprovementCandidate(c.Request.Context(), b.ID, c.Param("id"))
	if err != nil || row.WorkspaceID != b.WorkspaceID {
		c.AbortWithStatus(404)
		return
	}
	scope := cursorScope("maintenance-successes-v1", b.ID, row.ID, strconv.FormatInt(b.Version, 10), strconv.FormatInt(row.Revision, 10))
	after, ok := attentionAfter(c.Query("after"), scope)
	if !ok {
		c.AbortWithStatus(400)
		return
	}
	results := []models.MaintenanceSuccess{}
	if row.State != "prepared" || row.PreparedAt == nil || !h.Service.maintenanceAccountCurrent(c.Request.Context(), row) {
		c.JSON(200, gin.H{entriesKey: results, nextCursorKey: ""})
		return
	}
	reader, ok := h.Service.Manager.(MaintenanceSuccessFinder)
	if !ok {
		c.AbortWithStatus(503)
		return
	}
	tasks, err := h.Service.Repo.ImprovementAffectedTasks(c.Request.Context(), b.ID, row.ID, after, 26)
	if err != nil {
		c.AbortWithStatus(503)
		return
	}
	next := ""
	if len(tasks) > 25 {
		tasks = tasks[:25]
		next = encodeScopedCursor(scope, tasks[len(tasks)-1])
	}
	for _, id := range tasks {
		result, err := reader.FindMaintenanceSuccess(c.Request.Context(), b.WorkspaceID, id, row.ProfileID, *row.PreparedAt)
		if err != nil {
			c.AbortWithStatus(503)
			return
		}
		if result != nil {
			results = append(results, *result)
		}
	}
	c.JSON(200, gin.H{entriesKey: results, nextCursorKey: next})
}
