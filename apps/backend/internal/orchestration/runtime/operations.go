package runtime

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (h *Handler) performOperation(c *gin.Context, claims *runtimeauth.AgentClaims, req models.OperationRequest, input any, status int, execute func() (any, error)) {
	ctx := c.Request.Context()
	binding, err := h.Service.Repo.AssistantForConversation(ctx, claims.TaskID)
	if errors.Is(err, sql.ErrNoRows) && req.OperationID == "" {
		result, err := execute()
		if err != nil {
			fail(c, err)
			return
		}
		c.JSON(status, result)
		return
	}
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: "assistant_binding_required"})
		return
	}
	if req.OperationID == "" || len(req.OperationID) > 200 || req.ExpectedIntentRevision == nil || *req.ExpectedIntentRevision < 0 {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: "operation_identity_required"})
		return
	}
	raw, err := json.Marshal(input)
	if err != nil {
		fail(c, err)
		return
	}
	operation, created, err := h.Service.Repo.BeginOperation(ctx, models.Operation{
		BindingID: binding.ID, OperationID: req.OperationID, ConversationID: claims.TaskID,
		RunID: claims.RunID, Target: c.Request.URL.Path, RequestHash: fmt.Sprintf("%x", sha256.Sum256(raw)),
		IntentRevision: *req.ExpectedIntentRevision, BindingVersion: binding.Version,
	})
	if err != nil {
		if errors.Is(err, models.ErrConflict) {
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{errorResponseKey: "operation_conflict"})
		} else {
			c.AbortWithStatus(http.StatusServiceUnavailable)
		}
		return
	}
	if !created {
		replayOperation(c, operation)
		return
	}
	h.executeOperation(c, operation, status, execute)
}

func replayOperation(c *gin.Context, operation *models.Operation) {
	if operation.State == "acknowledged" || operation.State == statusFailed {
		c.Data(operation.HTTPStatus, "application/json", []byte(operation.ResponseJSON))
		return
	}
	c.AbortWithStatusJSON(http.StatusConflict, gin.H{errorResponseKey: "operation_outcome_unknown", "operation_id": operation.OperationID})
}

func (h *Handler) executeOperation(c *gin.Context, operation *models.Operation, status int, execute func() (any, error)) {
	result, err := execute()
	state := "acknowledged"
	raw, marshalErr := json.Marshal(result)
	if err != nil || marshalErr != nil || len(raw) > 32000 {
		state, raw, status = statusUnknown, []byte("{}"), http.StatusServiceUnavailable
	}
	var rejected *operationRejection
	if errors.As(err, &rejected) {
		state, status = statusFailed, rejected.status
		raw, _ = json.Marshal(gin.H{errorResponseKey: rejected.message})
	}
	// Receipt persistence must not inherit an upstream request's expired deadline.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 5*time.Second)
	defer cancel()
	if err := h.Service.Repo.FinishOperation(ctx, operation.ID, state, string(raw), status); err != nil {
		state = statusUnknown
	}
	if state == statusUnknown {
		c.JSON(http.StatusServiceUnavailable, gin.H{errorResponseKey: "operation_outcome_unknown", "operation_id": operation.OperationID})
		return
	}
	c.Data(status, "application/json", raw)
}

// Only pre-effect validation may report a known rejection. Dependency/transport
// errors remain unknown because a timeout does not prove non-delivery.
type operationRejection struct {
	status  int
	message string
}

func (e *operationRejection) Error() string { return e.message }
func rejectOperation(status int, message string) error {
	return &operationRejection{status: status, message: message}
}
