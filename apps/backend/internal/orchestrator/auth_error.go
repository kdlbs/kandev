package orchestrator

import (
	"encoding/json"
	"strings"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

// isAuthError returns true if the error message indicates an authentication failure.
func isAuthError(errorMsg string) bool {
	return routingerr.IsAuthenticationFailureDiagnostic(errorMsg)
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
