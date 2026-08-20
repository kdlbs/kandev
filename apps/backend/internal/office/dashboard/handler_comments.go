package dashboard

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/office/agents"
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
	taskID := c.Param("id")
	caller := agents.CallerFromContext(c)

	authorType := req.AuthorType
	if authorType == "" {
		authorType = userSentinel
	}
	// An authenticated agent JWT is authoritative over the request body: a
	// non-nil caller means the request came through agent-JWT auth, so it is
	// always attributed as an agent comment, even when author_type is
	// omitted. An explicit "user" claim from that same caller is rejected
	// rather than silently downgraded — trusting it would mislabel the
	// comment as user-authored and let it evade the scheduler's
	// self-comment guard (scheduler/reactivity.go), which only suppresses a
	// wake when author_type=="agent".
	if caller != nil {
		if req.AuthorType != "" && req.AuthorType != activityActorTypeAgent {
			h.logger.Warn("reject agent comment claiming non-agent author type",
				zap.String("task_id", taskID),
				zap.String("claimed_author_type", req.AuthorType),
				zap.String("caller_id", caller.ID))
			c.JSON(http.StatusForbidden, gin.H{"error": "author_type must be \"agent\" for an authenticated agent caller"})
			return
		}
		authorType = activityActorTypeAgent
	}
	authorID := userSentinel
	source := userSentinel
	if authorType == activityActorTypeAgent {
		// req.AuthorID is never trusted as the identity to persist — see
		// ResolveCommentAgentAuthor. It is only cross-checked here as a
		// cheap diagnostic: a mismatch means the CLI's KANDEV_AGENT_ID
		// disagrees with its own JWT, which should never happen and is
		// worth rejecting loudly rather than silently using the JWT's
		// (correct) identity.
		if caller != nil && req.AuthorID != "" && req.AuthorID != caller.ID {
			h.logger.Warn("reject agent comment with mismatched author",
				zap.String("task_id", taskID),
				zap.String("claimed_author_id", req.AuthorID),
				zap.String("caller_id", caller.ID))
			c.JSON(http.StatusForbidden, gin.H{"error": "author_id does not match the authenticated agent"})
			return
		}
		resolvedID, err := h.svc.ResolveCommentAgentAuthor(c.Request.Context(), taskID, caller)
		if err != nil {
			h.logger.Warn("reject agent comment with unauthorized author",
				zap.String("task_id", taskID),
				zap.Error(err))
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		authorID = resolvedID
		source = activityActorTypeAgent
	}
	comment := &models.TaskComment{
		ID:         uuid.New().String(),
		TaskID:     taskID,
		AuthorType: authorType,
		AuthorID:   authorID,
		Body:       req.Body,
		Source:     source,
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
