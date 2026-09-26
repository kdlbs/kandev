package dashboard

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/office/agents"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"go.uber.org/zap"
)

// -- Comments --
//
// Split out of handler.go so that file stays under revive's
// file-length-limit. All handlers below hang off *Handler and share its
// service, logger, and DTO shapes (CommentDTO, CreateCommentRequest, etc.,
// declared in dto.go).

func (h *Handler) listComments(c *gin.Context) {
	taskID := c.Param("id")

	if claims := agents.ClaimsFromContext(c); claims != nil {
		h.listCommentsForAgent(c, taskID, claims)
		return
	}

	ctx := c.Request.Context()
	comments, err := h.svc.ListComments(ctx, taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	runByComment := h.fetchRunStatusForComments(ctx, comments)
	dtos := make([]*CommentDTO, len(comments))
	for i, cm := range comments {
		dto := commentToDTO(cm)
		if r, ok := runByComment[cm.ID]; ok {
			dto.RunID = r.RunID
			dto.RunStatus = r.Status
			dto.RunError = r.ErrorMessage
		}
		dtos[i] = dto
	}
	c.JSON(http.StatusOK, CommentListResponse{Comments: dtos})
}

// listCommentsForAgent serves the guarded, bounded, reduced-projection
// comment window for an in-sandbox agent caller
// (REQ-OFFICE-AGENT-COMMENT-READS-001/002/003/004/005). The caller task
// identity comes only from the validated JWT claims, never from the
// request path or query string.
func (h *Handler) listCommentsForAgent(c *gin.Context, taskID string, claims *agents.AgentClaims) {
	if h.handoff == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "comment reads not configured"})
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	window, err := h.handoff.ListCommentsForCaller(c.Request.Context(), claims.TaskID, taskID, limit)
	if err != nil {
		if errors.Is(err, taskservice.ErrAccessDenied) {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, window)
}

// fetchRunStatusForComments batches a per-comment run-status lookup.
// Returns an empty map when no comments are passed or the lookup
// fails — the handler degrades to the legacy run-less DTO shape so
// the comments list never errors on a missing index/table.
func (h *Handler) fetchRunStatusForComments(
	ctx context.Context, comments []*models.TaskComment,
) map[string]sqlite.CommentRunStatus {
	if len(comments) == 0 {
		return map[string]sqlite.CommentRunStatus{}
	}
	ids := make([]string, len(comments))
	for i, cm := range comments {
		ids[i] = cm.ID
	}
	runs, err := h.svc.GetRunsByCommentIDs(ctx, ids)
	if err != nil {
		h.logger.Warn("fetch run status for comments failed", zap.Error(err))
		return map[string]sqlite.CommentRunStatus{}
	}
	return runs
}

func (h *Handler) createComment(c *gin.Context) {
	var req CreateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Body == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body is required"})
		return
	}
	taskID := c.Param("id")
	if agents.CallerFromContext(c) != nil ||
		(req.AuthorType != "" && req.AuthorType != userSentinel) {
		h.logger.Warn("reject agent comment on dashboard endpoint",
			zap.String("task_id", taskID))
		c.JSON(http.StatusForbidden, gin.H{
			"error": "agent comments must use the runtime comments endpoint",
		})
		return
	}
	comment := &models.TaskComment{
		ID:         uuid.New().String(),
		TaskID:     taskID,
		AuthorType: userSentinel,
		AuthorID:   userSentinel,
		Body:       req.Body,
		Source:     userSentinel,
	}
	if err := h.svc.CreateComment(c.Request.Context(), comment); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, CommentResponse{Comment: commentToDTO(comment)})
}

func commentToDTO(cm *models.TaskComment) *CommentDTO {
	return &CommentDTO{
		ID:         cm.ID,
		TaskID:     cm.TaskID,
		AuthorType: cm.AuthorType,
		AuthorID:   cm.AuthorID,
		Body:       cm.Body,
		Source:     cm.Source,
		// RFC3339Nano keeps sub-second precision so per-comment turn windows
		// in the UI can correctly include the agent message that triggered
		// the bridge — both timestamps are written within the same second
		// in office sessions, so seconds-only formatting collapses the
		// agent_message > comment ordering and excludes the reply.
		CreatedAt: cm.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}
