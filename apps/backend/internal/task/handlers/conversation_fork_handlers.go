package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
)

type createConversationForkDraftBody struct {
	CutoffMessageID     string   `json:"cutoff_message_id"`
	StartMessageID      string   `json:"start_message_id"`
	DraftRequestID      string   `json:"draft_request_id"`
	IncludeToolEvidence bool     `json:"include_tool_evidence"`
	ModelID             string   `json:"model_id"`
	AttachmentIDs       []string `json:"attachment_ids"`
}

type estimateConversationForkDraftBody struct {
	ModelID string `json:"model_id"`
}

type conversationForkCandidatesResponse struct {
	TaskID             string                              `json:"task_id"`
	SessionID          string                              `json:"session_id"`
	TaskTitle          string                              `json:"task_title"`
	Revision           int64                               `json:"revision"`
	CutoffMessageID    string                              `json:"cutoff_message_id"`
	CutoffTurnComplete bool                                `json:"cutoff_turn_complete"`
	Attachments        []models.ConversationForkAttachment `json:"attachments"`
	AttachmentsHasMore bool                                `json:"attachments_has_more"`
	AttachmentCursor   string                              `json:"attachment_cursor,omitempty"`
}

func (h *TaskHandlers) httpListConversationForkCandidates(c *gin.Context) {
	req := models.ConversationForkSourceRequest{
		SessionID:        c.Param("id"),
		CutoffMessageID:  strings.TrimSpace(c.Query("cutoff_message_id")),
		StartMessageID:   strings.TrimSpace(c.Query("start_message_id")),
		AttachmentCursor: strings.TrimSpace(c.Query("attachment_cursor")),
	}
	if req.CutoffMessageID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cutoff_message_id is required"})
		return
	}
	source, err := h.service.ListConversationForkCandidates(c.Request.Context(), req)
	if err != nil {
		handleConversationForkError(c, err)
		return
	}
	c.JSON(http.StatusOK, conversationForkCandidatesResponse{
		TaskID: source.TaskID, SessionID: source.SessionID, TaskTitle: source.TaskTitle,
		Revision: source.Revision, CutoffMessageID: req.CutoffMessageID,
		CutoffTurnComplete: source.CutoffTurnComplete, Attachments: source.AttachmentCandidates,
		AttachmentsHasMore: source.AttachmentsHasMore, AttachmentCursor: source.AttachmentCursor,
	})
}

func (h *TaskHandlers) httpCreateConversationForkDraft(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
	var body createConversationForkDraftBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation fork request"})
		return
	}
	request := models.ConversationForkCreateRequest{
		Source: models.ConversationForkSourceRequest{
			SessionID: c.Param("id"), CutoffMessageID: strings.TrimSpace(body.CutoffMessageID),
			StartMessageID: strings.TrimSpace(body.StartMessageID),
		},
		DraftRequestID: body.DraftRequestID, IncludeToolEvidence: body.IncludeToolEvidence,
		ModelID: body.ModelID, AttachmentIDs: body.AttachmentIDs,
	}
	draft, err := h.service.CreateConversationForkDraft(c.Request.Context(), request)
	if err != nil {
		handleConversationForkError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusCreated, draft.Descriptor)
}

func (h *TaskHandlers) httpGetConversationForkDraft(c *gin.Context) {
	draft, err := h.service.GetConversationForkDraft(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, models.ErrConversationForkExpired) {
			c.Header("Cache-Control", "private, no-store")
			c.JSON(http.StatusGone, gin.H{"code": "conversation_fork_expired", "draft": draft.Descriptor})
			return
		}
		handleConversationForkError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, draft.Descriptor)
}

func (h *TaskHandlers) httpGetConversationForkContent(c *gin.Context) {
	draft, err := h.service.GetConversationForkDraft(c.Request.Context(), c.Param("id"))
	if err != nil {
		handleConversationForkError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{
		"content": draft.CompiledText, "content_hash": draft.Descriptor.ContentHash,
		"compiler_version": draft.Descriptor.CompilerVersion,
	})
}

func (h *TaskHandlers) httpEstimateConversationForkDraft(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<10)
	var body estimateConversationForkDraftBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation fork estimate request"})
		return
	}
	estimate, err := h.service.EstimateConversationForkDraft(c.Request.Context(), c.Param("id"), body.ModelID)
	if err != nil {
		handleConversationForkError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, estimate)
}

func (h *TaskHandlers) httpDiscardConversationForkDraft(c *gin.Context) {
	if err := h.service.DiscardConversationForkDraft(c.Request.Context(), c.Param("id")); err != nil {
		handleConversationForkError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func handleConversationForkError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, models.ErrConversationForkCutoffUnavailable):
		c.JSON(http.StatusBadRequest, gin.H{"code": "conversation_fork_cutoff_unavailable"})
	case errors.Is(err, models.ErrConversationForkLimitExceeded):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"code": "conversation_fork_limit_exceeded"})
	case errors.Is(err, models.ErrConversationForkQuotaExceeded):
		c.JSON(http.StatusTooManyRequests, gin.H{"code": "conversation_fork_quota_exceeded"})
	case errors.Is(err, models.ErrConversationForkExpired):
		c.JSON(http.StatusGone, gin.H{"code": "conversation_fork_expired"})
	case errors.Is(err, models.ErrConversationForkNotFound), errors.Is(err, repoerrors.ErrTaskNotFound), errors.Is(err, repoerrors.ErrWorkspaceNotFound):
		c.JSON(http.StatusNotFound, gin.H{"code": "conversation_fork_not_found"})
	case errors.Is(err, service.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"code": "conversation_fork_forbidden"})
	case errors.Is(err, models.ErrConversationForkSourceUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": "conversation_fork_source_unavailable"})
	case errors.Is(err, models.ErrConversationForkAttachmentMissing):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"code": "conversation_fork_attachment_unavailable"})
	case errors.Is(err, models.ErrConversationForkToolEvidence):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"code": "conversation_fork_tool_evidence_unavailable"})
	case errors.Is(err, models.ErrConversationForkConflict):
		c.JSON(http.StatusConflict, gin.H{"code": "conversation_fork_request_conflict"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"code": "conversation_fork_failed"})
	}
}
