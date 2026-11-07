package utility

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
)

// InferenceClosureRequest contains identities only. Proof is produced locally.
type InferenceClosureRequest struct {
	OperationID         string `json:"operation_id"`
	CandidateInstanceID string `json:"candidate_instance_id"`
	ExpectedOwnerID     string `json:"expected_owner_id"`
	ExpectedState       []byte `json:"expected_state,omitempty"`
	Action              string `json:"action"`
}

type InferenceClosureResponse struct {
	State    []byte          `json:"state"`
	Snapshot AttemptSnapshot `json:"snapshot"`
}

func (h *Handler) handleInferenceClosure(c *gin.Context) {
	var req InferenceClosureRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid closure request"})
		return
	}
	if req.Action == "probe" && h.attempts != nil && h.attempts.cfg != nil && h.attempts.cfg.DurableJournalPath == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "durable inference closure unsupported"})
		return
	}
	result, err := h.inferenceClosure(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) inferenceClosure(ctx context.Context, req InferenceClosureRequest) (*InferenceClosureResponse, error) {
	if h.attempts == nil || req.OperationID == "" {
		return nil, ErrInferenceClosureIncomplete
	}
	owner := &InferenceOwner{attempts: h.attempts}
	if err := h.advanceInferenceClosureOwner(ctx, owner, req); err != nil {
		return nil, err
	}
	state, err := owner.InferenceClosureState()
	if err != nil {
		return nil, err
	}
	return &InferenceClosureResponse{State: state, Snapshot: h.attempts.snapshot()}, nil
}

func (h *Handler) advanceInferenceClosureOwner(ctx context.Context, owner *InferenceOwner, req InferenceClosureRequest) error {
	if req.Action == "probe" {
		if h.closureStop == nil || !h.attempts.snapshot().Known {
			return ErrInferenceClosureIncomplete
		}
		return nil
	}
	if req.Action == "prepare" {
		return h.prepareInferenceClosure(ctx, owner, req)
	}
	snapshot := h.attempts.snapshot()
	if snapshot.Closure == nil || snapshot.Closure.OperationID != req.OperationID {
		return ErrInferenceClosureIncomplete
	}
	switch req.Action {
	case "commit":
		_, err := owner.CommitInferenceClosure(ctx, req.OperationID, req.ExpectedState)
		return err
	case "status":
		return nil
	default:
		return ErrInferenceClosureIncomplete
	}
}

func (h *Handler) prepareInferenceClosure(ctx context.Context, owner *InferenceOwner, req InferenceClosureRequest) error {
	if h.closureStop == nil {
		return ErrInferenceClosureIncomplete
	}
	if _, err := owner.BeginInferenceClosure(ctx, req.OperationID, req.CandidateInstanceID, req.ExpectedOwnerID); err != nil {
		return err
	}
	if h.attempts.snapshot().Closure.Stage != closureFenced {
		return nil
	}
	proof, err := h.closureStop(ctx)
	if err != nil {
		return err
	}
	_, err = owner.ObserveInferenceClosure(ctx, req.OperationID, proof)
	return err
}
