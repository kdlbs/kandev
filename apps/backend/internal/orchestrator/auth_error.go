package orchestrator

import (
	"encoding/json"
	"strings"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

// isAuthError returns true if the error message indicates an authentication failure.
func isAuthError(errorMsg string) bool {
	return routingerr.IsAuthenticationFailureDiagnostic(errorMsg)
}

// isAuthenticationRecoveryFailure uses typed evidence whenever it exists.
// Text classification remains for older records that have no structured
// cause or routing code.
func isAuthenticationRecoveryFailure(data watcher.AgentEventData) bool {
	if data.FailureCode == string(routingerr.CodeAuthRequired) {
		return true
	}
	causes := models.NormalizeAgentErrorCauses(data.Causes)
	if len(causes) > 0 {
		for _, cause := range causes {
			if cause.Code == models.AgentErrorCauseCodeAuthenticationRequired {
				return true
			}
		}
		return false
	}
	return isAuthError(data.ErrorMessage)
}

func normalizeRecoveryAuthMethods(methods []streams.AuthMethodInfo) []streams.AuthMethodInfo {
	if len(methods) == 0 {
		return nil
	}
	projected := agentctltypes.NormalizeSessionOperationFailure(&agentctltypes.SessionOperationFailure{
		Code:        agentctltypes.SessionOperationFailureCodeAuthenticationRequired,
		AuthMethods: methods,
	})
	if projected == nil {
		return nil
	}
	return projected.AuthMethods
}

// extractReadableAuthError attempts to extract a human-readable authentication
// error message from the raw (often nested JSON-RPC) error string.
//
// Example input:
//
//	{"code":-32603,"message":"Internal error: Failed to authenticate. API Error: 401 {\"type\":\"error\",\"error\":{\"type\":\"authentication_error\",\"message\":\"OAuth token has expired...\"}}"}
//
// Returns the innermost message if found, otherwise a cleaned-up version of the raw error.
func extractReadableAuthError(rawError string) string {
	// Try to parse as JSON-RPC error envelope.
	var rpcErr struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(rawError), &rpcErr); err == nil && rpcErr.Message != "" {
		// Try to find an embedded JSON error body in the message.
		if msg := extractInnerAPIError(rpcErr.Message); msg != "" {
			return msg
		}
		return rpcErr.Message
	}

	// Not JSON — try to extract from raw string.
	if msg := extractInnerAPIError(rawError); msg != "" {
		return msg
	}

	return rawError
}

// extractInnerAPIError looks for an embedded JSON error object within a string
// (e.g., after "API Error: 401 ") and returns the inner message field.
func extractInnerAPIError(s string) string {
	// Find the first '{' that might start a JSON object.
	idx := strings.Index(s, "{")
	if idx < 0 {
		return ""
	}
	jsonStr := s[idx:]

	var apiErr struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &apiErr); err == nil && apiErr.Error.Message != "" {
		return apiErr.Error.Message
	}
	return ""
}
