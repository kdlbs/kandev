package utility

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"go.uber.org/zap"
)

type inferenceProcessOwner interface {
	DeliveryJournal() (*journal.Journal, error)
	BeginOwnedOperation(context.Context) (context.Context, func(), error)
}

// NewOwnedHandler uses the existing instance's storage and teardown admission.
func NewOwnedHandler(cfg *config.InstanceConfig, owner inferenceProcessOwner, log *zap.Logger) *Handler {
	h := NewHandler(cfg.WorkDir, log)
	retained, journalErr := owner.DeliveryJournal()
	if source, ok := owner.(interface{ InferenceOwner() *InferenceOwner }); ok && source.InferenceOwner() != nil {
		h.attempts = source.InferenceOwner().attempts
	} else {
		h.attempts = newAttemptOwner(cfg, retained)
	}
	if cfg.DurableJournalPath != "" && retained == nil {
		h.attempts.diagnostics.JournalErrorClass = attemptJournalErrorClass(journalErr)
	}
	h.beginOperation = owner.BeginOwnedOperation
	if finalOwner, ok := owner.(interface{ SetInferenceSnapshotter(func() AttemptSnapshot) }); ok {
		finalOwner.SetInferenceSnapshotter(h.attempts.finalSnapshot)
	}
	return h
}

func (h *Handler) executePrompt(ctx context.Context, req *PromptRequest) (*PromptResponse, error) {
	if h.attempts == nil {
		return h.promptExecutor(*req).Execute(ctx, req)
	}
	if req.AttemptID != "" {
		if _, err := uuid.Parse(req.AttemptID); err != nil {
			return &PromptResponse{Error: "attempt_id must be a UUID"}, nil
		}
	}
	if req.CallID != "" {
		if _, err := uuid.Parse(req.CallID); err != nil {
			return &PromptResponse{Error: "call_id must be a UUID"}, nil
		}
	}
	if h.beginOperation != nil {
		operation, release, err := h.beginOperation(ctx)
		if err != nil {
			return nil, err
		}
		defer release()
		ctx = operation
	}
	if req.AttemptID == "" {
		prepared, err := h.attempts.prepare("", req.CallID)
		if err != nil {
			return nil, err
		}
		req.AttemptID = prepared.AttemptID
	}
	status, err := h.attempts.consume(req.AttemptID)
	if err != nil {
		return &PromptResponse{Error: err.Error(), Attempt: &status}, err
	}
	ctx = withAttempt(ctx, h.attempts, req.AttemptID)
	resp, err := h.promptExecutor(*req).Execute(ctx, req)
	changeAttempt(ctx, settleNotStarted)
	status, _ = h.attempts.get(req.AttemptID)
	if resp == nil {
		resp = &PromptResponse{Error: "inference execution failed"}
	}
	resp.Attempt = &status
	return resp, err
}

func (h *Handler) handleAttempts(c *gin.Context) {
	if h.attempts == nil {
		c.JSON(http.StatusServiceUnavailable, AttemptSnapshot{Attempts: []AttemptStatus{}})
		return
	}
	c.JSON(http.StatusOK, h.attempts.snapshot())
}

func (h *Handler) handleAttempt(c *gin.Context) {
	if h.attempts == nil {
		c.JSON(http.StatusServiceUnavailable, AttemptStatus{AttemptID: c.Param("id"), State: AttemptUnknown})
		return
	}
	status, found := h.attempts.get(c.Param("id"))
	code := http.StatusOK
	if !found {
		code = http.StatusNotFound
	}
	c.JSON(code, status)
}

func promptErrorStatus(err error) int {
	if errors.Is(err, errAttemptExists) || errors.Is(err, errAttemptUnavailable) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}
