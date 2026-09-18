package runtime

import "github.com/gin-gonic/gin"

func (h *Handler) assistantMemorySource(c *gin.Context) {
	b, ok := h.humanAssistant(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	memory, err := h.Service.Repo.AssistantMemory(ctx, b.OrchestratorID, c.Param("id"))
	if err != nil || !h.Service.memoryVisible(ctx, b, memory) {
		c.AbortWithStatus(404)
		return
	}
	source, err := h.Service.Repo.GetCommentByID(ctx, b.ConversationID, memory.SourceCommentID)
	if err != nil || source.AuthorType != authorTypeUser || source.AuthorID != b.OwnerUserID {
		c.AbortWithStatus(404)
		return
	}
	body := []rune(source.Body)
	c.JSON(200, gin.H{"id": source.ID, "body": string(body[:min(len(body), 6000)]), "created_at": source.CreatedAt, "truncated": len(body) > 6000})
}
