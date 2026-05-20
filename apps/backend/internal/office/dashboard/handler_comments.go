package dashboard

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"go.uber.org/zap"
)

// -- Comments --
//
// Split out of handler.go so that file stays under revive's
// file-length-limit. All handlers below hang off *Handler and share its
// service, logger, and DTO shapes (CommentDTO, CreateCommentRequest, etc.,
// declared in dto.go).

func (h *Handler) listComments(c *gin.Context) {
	ctx := c.Request.Context()
	comments, err := h.svc.ListComments(ctx, c.Param("id"))
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
	authorType := req.AuthorType
	if authorType == "" {
		authorType = userSentinel
	}
	comment := &models.TaskComment{
		ID:         uuid.New().String(),
		TaskID:     c.Param("id"),
		AuthorType: authorType,
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
