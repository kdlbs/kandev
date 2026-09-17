package runtime

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (h *Handler) memory(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	if c.Param("id") != claims.AgentProfileID {
		c.AbortWithStatus(403)
		return
	}
	rows, err := h.Service.Repo.ListAgentMemory(c.Request.Context(), claims.AgentProfileID)
	if err != nil {
		fail(c, err)
		return
	}
	filtered := make([]*models.AgentMemory, 0, len(rows))
	for _, row := range rows {
		if id := c.Query("memory_id"); id != "" && row.ID != id {
			continue
		}
		if layer := c.Query("layer"); layer != "" && row.Layer != layer {
			continue
		}
		if key := c.Query("key"); key != "" && row.Key != key {
			continue
		}
		filtered = append(filtered, row)
	}
	c.JSON(200, gin.H{"entries": filtered, "memory": filtered, "count": len(filtered)})
}
func (h *Handler) setMemory(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	if c.Param("id") != claims.AgentProfileID {
		c.AbortWithStatus(403)
		return
	}
	var req struct {
		Entries []models.AgentMemory `json:"entries"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	if len(req.Entries) > 20 {
		fail(c, fmt.Errorf("too many memory entries"))
		return
	}
	for _, entry := range req.Entries {
		if entry.Layer == "" || entry.Key == "" || len(entry.Key) > 200 || len(entry.Content) > 16000 {
			fail(c, fmt.Errorf("invalid memory entry"))
			return
		}
	}
	for _, entry := range req.Entries {
		entry.AgentProfileID = claims.AgentProfileID
		entry.ID = ""
		entry.Metadata = "{}"
		if err := h.Service.Repo.UpsertAgentMemory(c.Request.Context(), &entry); err != nil {
			fail(c, err)
			return
		}
	}
	c.JSON(200, gin.H{"ok": true})
}
