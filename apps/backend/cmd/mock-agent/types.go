package main

import "encoding/json"

// Message types
const (
	TypeSystem          = "system"
	TypeAssistant       = "assistant"
	TypeUser            = "user"
	TypeResult          = "result"
	TypeControlRequest  = "control_request"
	TypeControlResponse = "control_response"
)

// Content block types
const (
	BlockText       = "text"
	BlockThinking   = "thinking"
	BlockToolUse    = "tool_use"
	BlockToolResult = "tool_result"
)

// Tool names matching Claude Code conventions
const (
	ToolBash = "Bash"
	ToolEdit = "Edit"
	ToolRead      = "Read"
	ToolGlob      = "Glob"
	ToolGrep      = "Grep"
	ToolTask      = "Task"
	ToolTodoWrite = "TodoWrite"
	ToolWebFetch  = "WebFetch"
)

// IncomingMessage is a minimal struct for parsing stdin messages.
type IncomingMessage struct {
	Type      string           `json:"type"`
	RequestID string           `json:"request_id,omitempty"`
	Message   *IncomingBody    `json:"message,omitempty"`
	Response  *IncomingControl `json:"response,omitempty"`
}

// IncomingBody is the message body for user messages.
type IncomingBody struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// IncomingControl is the response body for control_response messages.
type IncomingControl struct {
	Subtype   string           `json:"subtype"`
	RequestID string           `json:"request_id,omitempty"`
	Result    *PermissionReply `json:"result,omitempty"`
}

// PermissionReply is the result from a permission response.
type PermissionReply struct {
	Behavior string `json:"behavior"`
}

// --- Outgoing message types (written to stdout) ---

// SystemMsg is the system message emitted at the start of each turn.
type SystemMsg struct {
	Type          string `json:"type"`
	SessionID     string `json:"session_id"`
	SessionStatus string `json:"session_status"`
}

// AssistantMsg is an assistant message with content blocks.
type AssistantMsg struct {
	Type            string       `json:"type"`
	ParentToolUseID string       `json:"parent_tool_use_id,omitempty"`
	Message         AssistantBody `json:"message"`
}

// AssistantBody is the body of an assistant message.
type AssistantBody struct {
	Role       string         `json:"role"`
	Content    []ContentBlock `json:"content"`
	Model      string         `json:"model"`
	StopReason string         `json:"stop_reason,omitempty"`
	Usage      *Usage         `json:"usage,omitempty"`
}

// ContentBlock represents a content block in an assistant or user message.
type ContentBlock struct {
	Type string `json:"type"`

	// text block
	Text string `json:"text,omitempty"`

	// thinking block
	Thinking string `json:"thinking,omitempty"`

	// tool_use block
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`

	// tool_result block
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// Usage contains token usage information.
type Usage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens,omitempty"`
}

// UserMsg is a user message (used for tool results).
type UserMsg struct {
	Type            string       `json:"type"`
	ParentToolUseID string       `json:"parent_tool_use_id,omitempty"`
	Message         UserMsgBody  `json:"message"`
}

// UserMsgBody is the body of a user message with tool results.
type UserMsgBody struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

// ResultMsg is the final result message for a turn.
type ResultMsg struct {
	Type              string                    `json:"type"`
	Result            json.RawMessage           `json:"result"`
	CostUSD           float64                   `json:"cost_usd"`
	DurationMS        int64                     `json:"duration_ms"`
	DurationAPIMS     int64                     `json:"duration_api_ms"`
	IsError           bool                      `json:"is_error"`
	NumTurns          int                       `json:"num_turns"`
	TotalInputTokens  int64                     `json:"total_input_tokens"`
	TotalOutputTokens int64                     `json:"total_output_tokens"`
	ModelUsage        map[string]ModelUsageStats `json:"model_usage,omitempty"`
}

// ModelUsageStats per-model usage statistics.
type ModelUsageStats struct {
	ContextWindow int64 `json:"context_window"`
}

// ResultData is the result object for successful completions.
type ResultData struct {
	Text      string `json:"text"`
	SessionID string `json:"session_id"`
}

// ControlRequestMsg is a control request emitted to stdout (permission requests).
type ControlRequestMsg struct {
	Type      string              `json:"type"`
	RequestID string              `json:"request_id"`
	Request   ControlRequestBody  `json:"request"`
}

// ControlRequestBody is the body of a control request.
type ControlRequestBody struct {
	Subtype   string         `json:"subtype"`
	ToolName  string         `json:"tool_name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
}

// ControlResponseMsg is a control response emitted to stdout (for initialize).
type ControlResponseMsg struct {
	Type     string               `json:"type"`
	Response ControlResponseBody  `json:"response"`
}

// ControlResponseBody is the body of a control response.
type ControlResponseBody struct {
	Subtype   string               `json:"subtype"`
	RequestID string               `json:"request_id"`
	Response  *InitializeResponse  `json:"response,omitempty"`
}

// InitializeResponse is the response to an initialize control request.
type InitializeResponse struct {
	Commands []Command `json:"commands"`
	Agents   []string  `json:"agents"`
}

// Command is an available slash command.
type Command struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	ArgumentHint string `json:"argumentHint,omitempty"`
}
