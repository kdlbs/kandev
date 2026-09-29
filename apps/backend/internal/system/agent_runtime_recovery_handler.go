package system

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
)

const maxAgentRuntimeRecoveryBodyBytes = 4 * 1024

type agentRuntimeRetryRequest struct {
	BootID       string `json:"boot_id"`
	RuntimeEpoch uint64 `json:"runtime_epoch"`
	Revision     uint64 `json:"revision"`
	RequestID    string `json:"request_id"`
}

func handleAgentRuntimeRetry(target AgentRuntimeRecoveryTarget) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		request, err := decodeAgentRuntimeRetryRequest(ctx)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent runtime retry request", "error_code": "invalid_request"})
			return
		}

		snapshot, err := target.RetryAtRevision(
			ctx.Request.Context(), request.BootID, request.RuntimeEpoch, request.Revision, request.RequestID,
		)
		if err != nil {
			writeAgentRuntimeRecoveryError(ctx, err)
			return
		}
		ctx.JSON(http.StatusAccepted, snapshot)
	}
}

func decodeAgentRuntimeRetryRequest(ctx *gin.Context) (agentRuntimeRetryRequest, error) {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxAgentRuntimeRecoveryBodyBytes)
	decoder := json.NewDecoder(ctx.Request.Body)
	decoder.DisallowUnknownFields()
	var request agentRuntimeRetryRequest
	if err := decoder.Decode(&request); err != nil {
		return agentRuntimeRetryRequest{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return agentRuntimeRetryRequest{}, errors.New("multiple JSON values")
		}
		return agentRuntimeRetryRequest{}, err
	}
	request.BootID = strings.TrimSpace(request.BootID)
	request.RequestID = strings.TrimSpace(request.RequestID)
	if request.BootID == "" || request.RuntimeEpoch == 0 || request.Revision == 0 ||
		request.RequestID == "" || len(request.RequestID) > 128 {
		return agentRuntimeRetryRequest{}, errors.New("required retry fields are missing")
	}
	return request, nil
}

func writeAgentRuntimeRecoveryError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, agentruntime.ErrRecoveryConflict):
		ctx.JSON(http.StatusConflict, gin.H{"error": "agent runtime state changed; refresh and retry", "error_code": "stale_runtime_snapshot"})
	case errors.Is(err, agentruntime.ErrRecoveryNotRetryable):
		ctx.JSON(http.StatusConflict, gin.H{"error": "agent runtime recovery is not currently retryable", "error_code": "runtime_recovery_not_retryable"})
	case errors.Is(err, agentruntime.ErrRecoveryRequestID):
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request id", "error_code": "invalid_request"})
	case errors.Is(err, agentruntime.ErrRuntimeOwnerStopped):
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "agent runtime recovery is unavailable", "error_code": "runtime_recovery_unavailable"})
	default:
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start agent runtime recovery", "error_code": "runtime_recovery_failed"})
	}
}
