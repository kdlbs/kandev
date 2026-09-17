package runtime

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (h *Handler) acceptComment(c *gin.Context, agentID string) {
	var req struct {
		Body            string `json:"body"`
		ClientMessageID string `json:"client_message_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	identity, ok := authn.FromGin(c)
	if !ok || identity.UserID == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	row := &models.TaskComment{TaskID: c.Param("id"), AuthorID: identity.UserID, Body: req.Body}
	receipt, created, err := h.Service.Repo.AcceptComment(c.Request.Context(), agentID, req.ClientMessageID, row)
	if errors.Is(err, models.ErrConflict) {
		c.JSON(http.StatusConflict, gin.H{errorResponseKey: "message_id_conflict"})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	// Acceptance is durable even while the queue is unavailable. The dispatch
	// tick repairs pending outbox rows; a transport retry uses the same receipt.
	_ = h.Service.dispatchIntake(c.Request.Context(), *receipt)
	row, err = h.Service.Repo.GetCommentByID(c.Request.Context(), receipt.TaskID, receipt.CommentID)
	if err != nil {
		fail(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, row)
}

// DispatchIntake is called by the existing run scheduler, with no additional
// goroutine. Queue idempotency closes the crash gap before outbox acknowledgement.
func (s *Service) DispatchIntake(ctx context.Context) error {
	rows, err := s.Repo.PendingIntake(ctx)
	if err != nil {
		return err
	}
	var problems []error
	for _, row := range rows {
		if err := s.dispatchIntake(ctx, row); err != nil && !errors.Is(err, ErrAssistantDisabled) {
			problems = append(problems, err)
		}
	}
	return errors.Join(problems...)
}

func (s *Service) dispatchIntake(ctx context.Context, row models.Intake) error {
	if row.Status != "accepted" {
		return nil
	}
	// Check durable run identity without a time window before enqueueing again.
	runs, err := s.Runs.GetRunsByCommentIDs(ctx, []string{row.CommentID})
	if err != nil {
		return err
	}
	if run, ok := runs[row.CommentID]; ok {
		return s.Repo.AcknowledgeIntake(ctx, row.CommentID, run.RunID)
	}
	if err := s.QueueTurn(ctx, row.AgentID, row.TaskID, "task_comment", "task_comment:"+row.CommentID,
		map[string]any{"comment_id": row.CommentID, "intent_revision": row.Sequence}); err != nil {
		return err
	}
	runs, err = s.Runs.GetRunsByCommentIDs(ctx, []string{row.CommentID})
	if err != nil {
		return err
	}
	if run, ok := runs[row.CommentID]; ok {
		return s.Repo.AcknowledgeIntake(ctx, row.CommentID, run.RunID)
	}
	return nil
}
