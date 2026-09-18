package runtime

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (h *Handler) assistantMemory(c *gin.Context) {
	b, ok := h.humanAssistant(c)
	if !ok {
		return
	}
	if c.Param("id") != "" {
		row, err := h.Service.Repo.AssistantMemory(c.Request.Context(), b.OrchestratorID, c.Param("id"))
		if err != nil || !h.Service.memoryVisible(c.Request.Context(), b, row) {
			c.AbortWithStatus(404)
			return
		}
		c.JSON(200, row)
		return
	}
	h.assistantMemoryPage(c, b)
}

type memoryEdit struct {
	Key       string     `json:"key"`
	Content   string     `json:"content"`
	Scope     string     `json:"scope"`
	ScopeID   string     `json:"scope_id"`
	Source    string     `json:"source_comment_id"`
	Confirmed bool       `json:"confirmed"`
	Priority  int        `json:"priority"`
	ExpiresAt *time.Time `json:"expires_at"`
	Expected  int64      `json:"expected_revision"`
}

func (r memoryEdit) valid() bool {
	if strings.TrimSpace(r.Key) == "" || len(r.Key) > 200 || strings.TrimSpace(r.Content) == "" || len(r.Content) > 16000 || !utf8.ValidString(r.Content) {
		return false
	}
	if r.Expected < 0 || r.Priority < 0 || r.Priority > 100 || len(r.ScopeID) > 200 {
		return false
	}
	switch r.Scope {
	case authorTypeUser, scopeWorkspace:
		return true
	case "project", "environment", scopeTask:
		return r.ScopeID != ""
	default:
		return false
	}
}

func (h *Handler) editAssistantMemory(c *gin.Context) {
	b, ok := h.humanAssistant(c)
	if !ok {
		return
	}
	var req memoryEdit
	if !strictAssistantJSON(c, &req) {
		return
	}
	if !req.valid() {
		c.AbortWithStatus(422)
		return
	}
	if len(c.Param("id")) > 200 {
		c.AbortWithStatus(422)
		return
	}
	source, err := h.Service.Repo.GetCommentByID(c.Request.Context(), b.ConversationID, req.Source)
	if err != nil || source.AuthorType != authorTypeUser || source.AuthorID != b.OwnerUserID {
		c.AbortWithStatus(422)
		return
	}
	scopeID, err := h.Service.validateMemoryScope(c.Request.Context(), b, req.Scope, req.ScopeID)
	if err != nil {
		c.AbortWithStatus(422)
		return
	}
	req.ScopeID = scopeID
	row := &models.AgentMemory{ID: c.Param("id"), AgentProfileID: b.OrchestratorID, OwnerUserID: b.OwnerUserID, Layer: authorTypeUser, Key: req.Key, Content: req.Content, Metadata: "{}", Scope: req.Scope, ScopeID: req.ScopeID, SourceCommentID: req.Source, Confirmed: req.Confirmed, Priority: req.Priority, ExpiresAt: req.ExpiresAt}
	if err := h.Service.Repo.SaveAssistantMemory(c.Request.Context(), row, req.Expected); err != nil {
		memoryFailure(c, err)
		return
	}
	saved, err := h.Service.Repo.AssistantMemory(c.Request.Context(), b.OrchestratorID, row.ID)
	if err != nil {
		c.AbortWithStatus(503)
		return
	}
	h.Service.notifyAssistantUpdated(c.Request.Context(), b.ID)
	c.JSON(200, saved)
}

func (h *Handler) forgetAssistantMemory(c *gin.Context) {
	b, ok := h.humanAssistant(c)
	if !ok {
		return
	}
	var req struct {
		Expected int64 `json:"expected_revision"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Expected < 1 {
		c.AbortWithStatus(422)
		return
	}
	if err := h.Service.Repo.ForgetAssistantMemory(c.Request.Context(), b.OrchestratorID, c.Param("id"), req.Expected); err != nil {
		memoryFailure(c, err)
		return
	}
	h.Service.notifyAssistantUpdated(c.Request.Context(), b.ID)
	c.JSON(200, gin.H{"forgotten": true})
}

func memoryFailure(c *gin.Context, err error) {
	if errors.Is(err, models.ErrConflict) {
		c.AbortWithStatus(409)
		return
	}
	c.AbortWithStatus(503)
}
