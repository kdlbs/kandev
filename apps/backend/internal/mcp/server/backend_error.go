package mcp

import "fmt"

// BackendError preserves a structured WebSocket error for MCP tool handlers.
// Details can carry a committed partial result or a stable denial reason and
// remain available to callers that can recover from an independent failure.
type BackendError struct {
	Code    string
	Message string
	Details map[string]interface{}
}

func (e *BackendError) Error() string {
	if e == nil {
		return "backend error"
	}
	if e.Code == "" {
		return fmt.Sprintf("backend error: %s", e.Message)
	}
	return fmt.Sprintf("backend error [%s]: %s", e.Code, e.Message)
}
