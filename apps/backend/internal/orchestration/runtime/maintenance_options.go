package runtime

import (
	"cmp"
	"slices"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (h *Handler) maintenanceOptions(c *gin.Context) {
	b, ok := h.humanAssistant(c)
	if !ok {
		return
	}
	limit, ok := boundedPageLimit(c)
	scope := cursorScope("maintenance-options-v1", b.ID, b.WorkspaceID, strconv.FormatInt(b.Version, 10))
	after, valid := attentionAfter(c.Query("after"), scope)
	if !ok || !valid {
		c.AbortWithStatus(400)
		return
	}
	reader, ok := h.Service.Manager.(MaintenanceOptionsReader)
	if !ok {
		c.AbortWithStatus(503)
		return
	}
	rows, err := reader.MaintenanceOptions(c.Request.Context(), b.WorkspaceID)
	if err != nil {
		c.AbortWithStatus(503)
		return
	}
	profiles, err := h.Service.Repo.ExecutionProfileDirectory(c.Request.Context(), b.WorkspaceID)
	if err != nil {
		c.AbortWithStatus(503)
		return
	}
	for _, p := range profiles {
		rows = append(rows, models.MaintenanceOption{ID: "profile:" + p["id"], ResourceID: p["id"], Name: p["name"], Kind: "profile"})
	}
	slices.SortFunc(rows, func(a, b models.MaintenanceOption) int { return cmp.Compare(a.ID, b.ID) })
	result := []models.MaintenanceOption{}
	for _, row := range rows {
		if row.ID > after {
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
	c.JSON(200, gin.H{entriesKey: result, nextCursorKey: next})
}
