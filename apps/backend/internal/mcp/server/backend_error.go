package mcp

import "fmt"

// BackendError preserves the structured websocket error payload (code,
// message, details) returned by a failed BackendClient.RequestPayload call.
// Handlers that surface backend failures to MCP tool callers can use
// errors.As to recover Details — e.g. a related-task-read denial's
// details["reason"] — instead of only matching a flattened error string.
type BackendError struct {
	Code    string
	Message string
	Details map[string]interface{}
}

func (e *BackendError) Error() string {
	return fmt.Sprintf("backend error [%s]: %s", e.Code, e.Message)
}
