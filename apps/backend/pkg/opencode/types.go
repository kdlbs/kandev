// Package opencode provides types and client for the OpenCode CLI protocol.
// OpenCode uses a REST API + Server-Sent Events (SSE) pattern for communication.
package opencode

import (
	"encoding/json"
)

// ExecutorEvent types written to adapter output
const (
	EventTypeStartupLog       = "startup_log"
	EventTypeSessionStart     = "session_start"
	EventTypeSDKEvent         = "sdk_event"
	EventTypeTokenUsage       = "token_usage"
	EventTypeApprovalResponse = "approval_response"
	EventTypeError            = "error"
	EventTypeDone             = "done"
)

// SDK Event types from /event SSE stream
const (
	SDKEventMessageUpdated     = "message.updated"
	SDKEventMessagePartUpdated = "message.part.updated"
	SDKEventMessageRemoved     = "message.removed"
	SDKEventMessagePartRemoved = "message.part.removed"
	SDKEventPermissionAsked    = "permission.asked"
	SDKEventPermissionReplied  = "permission.replied"
	SDKEventSessionIdle        = "session.idle"
	SDKEventSessionStatus      = "session.status"
	SDKEventSessionDiff        = "session.diff"
	SDKEventSessionCompacted   = "session.compacted"
	SDKEventSessionError       = "session.error"
	SDKEventTodoUpdated        = "todo.updated"
)

// Part types
const (
	PartTypeText      = "text"
	PartTypeReasoning = "reasoning"
	PartTypeTool      = "tool"
)

// Tool status values
const (
	ToolStatusPending   = "pending"
	ToolStatusRunning   = "running"
	ToolStatusCompleted = "completed"
	ToolStatusError     = "error"
)

// Permission reply values
const (
	PermissionReplyOnce   = "once"
	PermissionReplyReject = "reject"
)

// ExecutorEvent represents events written to stdout by the adapter
type ExecutorEvent struct {
	Type string `json:"type"`
	// Variant fields based on type
	Message            string          `json:"message,omitempty"`
	SessionID          string          `json:"session_id,omitempty"`
	Event              json.RawMessage `json:"event,omitempty"`
	TotalTokens        int             `json:"total_tokens,omitempty"`
	ModelContextWindow int             `json:"model_context_window,omitempty"`
	ToolCallID         string          `json:"tool_call_id,omitempty"`
	Status             string          `json:"status,omitempty"`
}

// HealthResponse from GET /global/health
type HealthResponse struct {
	Healthy bool   `json:"healthy"`
	Version string `json:"version"`
}

// SessionResponse from POST /session or POST /session/{id}/fork
type SessionResponse struct {
	ID string `json:"id"`
}

// ModelSpec for prompt requests
type ModelSpec struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
}

// TextPartInput for prompt request parts
type TextPartInput struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// PromptRequest for POST /session/{id}/message
type PromptRequest struct {
	Model   *ModelSpec      `json:"model,omitempty"`
	Agent   string          `json:"agent,omitempty"`
	Variant string          `json:"variant,omitempty"`
	Parts   []TextPartInput `json:"parts"`
}

// PermissionReplyRequest for POST /permission/{id}/reply
type PermissionReplyRequest struct {
	Reply   string `json:"reply"`
	Message string `json:"message,omitempty"`
}

// SDKEventEnvelope is the base structure for all SSE events
type SDKEventEnvelope struct {
	Type       string          `json:"type"`
	Properties json.RawMessage `json:"properties,omitempty"`
}

// MessageUpdatedProperties for message.updated events
type MessageUpdatedProperties struct {
	Info MessageInfo `json:"info"`
}

// MessageInfo contains message metadata
type MessageInfo struct {
	ID         string             `json:"id"`
	SessionID  string             `json:"sessionID"`
	Role       string             `json:"role"` // "user", "assistant"
	Model      *MessageModelInfo  `json:"model,omitempty"`
	ProviderID string             `json:"providerID,omitempty"`
	ModelID    string             `json:"modelID,omitempty"`
	Tokens     *MessageTokensInfo `json:"tokens,omitempty"`
}

// MessageModelInfo contains model information
type MessageModelInfo struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
}

// MessageTokensInfo contains token usage information
type MessageTokensInfo struct {
	Input  int                     `json:"input"`
	Output int                     `json:"output"`
	Cache  *MessageTokensCacheInfo `json:"cache,omitempty"`
}

// MessageTokensCacheInfo contains cache token information
type MessageTokensCacheInfo struct {
	Read int `json:"read"`
}

// MessagePartUpdatedProperties for message.part.updated events
type MessagePartUpdatedProperties struct {
	Part  Part   `json:"part"`
	Delta string `json:"delta,omitempty"`
}

// Part represents a message part (text, reasoning, or tool)
type Part struct {
	ID        string           `json:"id"`   // Unique part identifier for tracking updates
	Type      string           `json:"type"` // "text", "reasoning", "tool"
	MessageID string           `json:"messageID"`
	SessionID string           `json:"sessionID"`
	Text      string           `json:"text,omitempty"`   // For text/reasoning
	CallID    string           `json:"callID,omitempty"` // For tool
	Tool      string           `json:"tool,omitempty"`   // For tool
	State     *ToolStateUpdate `json:"state,omitempty"`  // For tool
}

// ToolStateUpdate represents tool execution state
type ToolStateUpdate struct {
	Status   string          `json:"status"` // "pending", "running", "completed", "error"
	Input    json.RawMessage `json:"input,omitempty"`
	Output   string          `json:"output,omitempty"`
	Title    string          `json:"title,omitempty"`
	Error    string          `json:"error,omitempty"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// PermissionAskedProperties for permission.asked events
type PermissionAskedProperties struct {
	ID         string              `json:"id"`
	SessionID  string              `json:"sessionID"`
	Permission string              `json:"permission"`
	Patterns   []string            `json:"patterns,omitempty"`
	Metadata   map[string]any      `json:"metadata,omitempty"`
	Tool       *PermissionToolInfo `json:"tool,omitempty"`
}

// PermissionToolInfo contains tool information for permission requests
type PermissionToolInfo struct {
	CallID string `json:"callID"`
}

// SessionIdleProperties for session.idle events
type SessionIdleProperties struct {
	SessionID string `json:"sessionID"`
}

// SessionErrorProperties for session.error events
type SessionErrorProperties struct {
	SessionID string    `json:"sessionID"`
	Error     *SDKError `json:"error,omitempty"`
}

// SDKError represents an error from the SDK
type SDKError struct {
	Name    string `json:"name,omitempty"`
	Type    string `json:"type,omitempty"`
	Message string `json:"message,omitempty"`
	Data    *struct {
		Message string `json:"message,omitempty"`
	} `json:"data,omitempty"`
}

// GetMessage returns the error message
func (e *SDKError) GetMessage() string {
	if e.Data != nil && e.Data.Message != "" {
		return e.Data.Message
	}
	return e.Message
}

// GetKind returns the error kind/type
func (e *SDKError) GetKind() string {
	if e.Name != "" {
		return e.Name
	}
	if e.Type != "" {
		return e.Type
	}
	return "unknown"
}

// SessionStatusProperties for session.status events
type SessionStatusProperties struct {
	Status SessionStatus `json:"status"`
}

// SessionStatus represents session status
type SessionStatus struct {
	Type    string `json:"type"` // "idle", "busy", "retry"
	Attempt int    `json:"attempt,omitempty"`
	Message string `json:"message,omitempty"`
	Next    int64  `json:"next,omitempty"`
}

// TodoUpdatedProperties for todo.updated events
type TodoUpdatedProperties struct {
	Todos []SDKTodo `json:"todos"`
}

// SDKTodo represents a todo item
type SDKTodo struct {
	ID       string `json:"id"`
	Content  string `json:"content"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
}

// ProviderListResponse from GET /provider
type ProviderListResponse struct {
	All []ProviderInfo `json:"all"`
}

// ProviderInfo contains provider information
type ProviderInfo struct {
	ID     string                       `json:"id"`
	Models map[string]ProviderModelInfo `json:"models,omitempty"`
}

// ProviderModelInfo contains model information from provider
type ProviderModelInfo struct {
	Limit ProviderModelLimit `json:"limit"`
}

// ProviderModelLimit contains model limits
type ProviderModelLimit struct {
	Context int `json:"context"`
}

// ParseSDKEvent parses an SDK event from JSON
func ParseSDKEvent(data []byte) (*SDKEventEnvelope, error) {
	var env SDKEventEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	return &env, nil
}

// ParseMessageUpdated parses message.updated properties
func ParseMessageUpdated(data json.RawMessage) (*MessageUpdatedProperties, error) {
	var props MessageUpdatedProperties
	if err := json.Unmarshal(data, &props); err != nil {
		return nil, err
	}
	return &props, nil
}

// ParseMessagePartUpdated parses message.part.updated properties
func ParseMessagePartUpdated(data json.RawMessage) (*MessagePartUpdatedProperties, error) {
	var props MessagePartUpdatedProperties
	if err := json.Unmarshal(data, &props); err != nil {
		return nil, err
	}
	return &props, nil
}

// ParsePermissionAsked parses permission.asked properties
func ParsePermissionAsked(data json.RawMessage) (*PermissionAskedProperties, error) {
	var props PermissionAskedProperties
	if err := json.Unmarshal(data, &props); err != nil {
		return nil, err
	}
	return &props, nil
}

// ParseSessionIdle parses session.idle properties
func ParseSessionIdle(data json.RawMessage) (*SessionIdleProperties, error) {
	var props SessionIdleProperties
	if err := json.Unmarshal(data, &props); err != nil {
		return nil, err
	}
	return &props, nil
}

// ParseSessionError parses session.error properties
func ParseSessionError(data json.RawMessage) (*SessionErrorProperties, error) {
	var props SessionErrorProperties
	if err := json.Unmarshal(data, &props); err != nil {
		return nil, err
	}
	return &props, nil
}
