// Package claudecode provides types and client for the Claude Code CLI stream-json protocol.
// Claude Code uses a streaming JSON format over stdin/stdout with control requests for permissions.
package claudecode

import "encoding/json"

// Message types from Claude Code CLI
const (
	// MessageTypeSystem is the initial system message with session info
	MessageTypeSystem = "system"
	// MessageTypeAssistant contains text or thinking from the assistant
	MessageTypeAssistant = "assistant"
	// MessageTypeResult is the final result message
	MessageTypeResult = "result"
	// MessageTypeControlRequest is a control request (permission, hook)
	MessageTypeControlRequest = "control_request"
	// MessageTypeControlResponse is a response to a control request
	MessageTypeControlResponse = "control_response"
	// MessageTypeUser is a user message (prompt)
	MessageTypeUser = "user"
)

// Control request subtypes
const (
	// SubtypeCanUseTool is a permission request for tool use
	SubtypeCanUseTool = "can_use_tool"
	// SubtypeHookCallback is a hook callback request
	SubtypeHookCallback = "hook_callback"
	// SubtypeInitialize initializes the session
	SubtypeInitialize = "initialize"
	// SubtypeInterrupt interrupts the current operation
	SubtypeInterrupt = "interrupt"
	// SubtypeSetPermissionMode sets the permission mode
	SubtypeSetPermissionMode = "set_permission_mode"
)

// Permission behaviors
const (
	// BehaviorAllow allows the tool use
	BehaviorAllow = "allow"
	// BehaviorDeny denies the tool use
	BehaviorDeny = "deny"
)

// CLIMessage represents messages from Claude Code CLI stdout.
// The message type determines which fields are populated.
type CLIMessage struct {
	// Type is the message type (system, assistant, result, control_request, control_response, etc.)
	Type string `json:"type"`

	// For control_request messages (from Claude to us)
	RequestID string          `json:"request_id,omitempty"`
	Request   *ControlRequest `json:"request,omitempty"`

	// For control_response messages (from Claude back to us after we send a control_request)
	Response *IncomingControlResponse `json:"response,omitempty"`

	// For system messages
	SessionID     string   `json:"session_id,omitempty"`
	SessionStatus string   `json:"session_status,omitempty"`
	SlashCommands []string `json:"slash_commands,omitempty"` // System message uses string array

	// For subagent context (when running inside a Task tool call)
	ParentToolUseID string `json:"parent_tool_use_id,omitempty"`

	// For assistant messages
	Message *AssistantMessage `json:"message,omitempty"`

	// For result messages
	// Result can be either a string (error message) or an object (ResultData)
	Result            json.RawMessage            `json:"result,omitempty"`
	Subtype           string                     `json:"subtype,omitempty"`
	CostUSD           float64                    `json:"cost_usd,omitempty"`
	DurationMS        int64                      `json:"duration_ms,omitempty"`
	DurationAPIMS     int64                      `json:"duration_api_ms,omitempty"`
	IsError           bool                       `json:"is_error,omitempty"`
	Errors            []string                   `json:"errors,omitempty"`
	NumTurns          int                        `json:"num_turns,omitempty"`
	TotalInputTokens  int64                      `json:"total_input_tokens,omitempty"`
	TotalOutputTokens int64                      `json:"total_output_tokens,omitempty"`
	ModelUsage        map[string]ModelUsageStats `json:"model_usage,omitempty"`

	// Raw message for parsing content blocks
	RawContent json.RawMessage `json:"-"`
}

// AssistantMessage contains the assistant's response content.
// Note: For user messages (type="user"), Content may be a plain string instead of []ContentBlock.
// Use RawContent and GetContentBlocks()/GetContentString() for flexible parsing.
type AssistantMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"` // Can be string or []ContentBlock
	Model      string          `json:"model,omitempty"`
	StopReason string          `json:"stop_reason,omitempty"`
	Usage      *Usage          `json:"usage,omitempty"`
}

// GetContentBlocks attempts to parse Content as []ContentBlock.
// Returns nil if Content is a string or cannot be parsed.
func (m *AssistantMessage) GetContentBlocks() []ContentBlock {
	if len(m.Content) == 0 {
		return nil
	}
	var blocks []ContentBlock
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return nil
	}
	return blocks
}

// GetContentString attempts to parse Content as a plain string.
// Returns empty string if Content is []ContentBlock or cannot be parsed.
func (m *AssistantMessage) GetContentString() string {
	if len(m.Content) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(m.Content, &s); err != nil {
		return ""
	}
	return s
}

// ContentBlock represents a block of content in an assistant message.
type ContentBlock struct {
	Type string `json:"type"`

	// For text blocks
	Text string `json:"text,omitempty"`

	// For thinking blocks
	Thinking string `json:"thinking,omitempty"`

	// For tool_use blocks
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`

	// For tool_result blocks
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

// ResultData contains the final result information.
type ResultData struct {
	Text      string `json:"text,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// GetResultData attempts to parse the Result field as a ResultData object.
// Returns nil if Result is empty, a string, or cannot be parsed as ResultData.
func (m *CLIMessage) GetResultData() *ResultData {
	if len(m.Result) == 0 {
		return nil
	}
	var data ResultData
	if err := json.Unmarshal(m.Result, &data); err != nil {
		return nil
	}
	return &data
}

// GetResultString returns the Result field as a string.
// This is used when the result is an error message string.
func (m *CLIMessage) GetResultString() string {
	if len(m.Result) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(m.Result, &s); err != nil {
		// Not a string, return empty
		return ""
	}
	return s
}

// ModelUsageStats contains per-model usage statistics from result message.
// The context_window field provides the actual model context window size.
type ModelUsageStats struct {
	ContextWindow *int64 `json:"context_window,omitempty"`
}

// ControlRequest represents a control request from Claude Code CLI.
// This is used for permission requests (can_use_tool) and hook callbacks.
type ControlRequest struct {
	// Subtype identifies the type of control request
	Subtype string `json:"subtype"`

	// For can_use_tool requests
	ToolName  string         `json:"tool_name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`

	// For hook_callback requests
	CallbackID string         `json:"callback_id,omitempty"`
	HookName   string         `json:"hook_name,omitempty"`
	HookInput  map[string]any `json:"hook_input,omitempty"`

	// Permission suggestions from Claude
	PermissionSuggestions []PermissionUpdate `json:"permission_suggestions,omitempty"`
}

// PermissionUpdate represents a permission rule update.
type PermissionUpdate struct {
	Tool    string `json:"tool"`
	Pattern string `json:"pattern,omitempty"`
	Allow   bool   `json:"allow"`
}

// ControlResponseMessage is the message sent to respond to control requests.
type ControlResponseMessage struct {
	Type      string           `json:"type"` // "control_response"
	RequestID string           `json:"request_id"`
	Response  *ControlResponse `json:"response"`
}

// IncomingControlResponse represents a control_response message from Claude Code CLI
// (response to a control_request we sent, like initialize).
type IncomingControlResponse struct {
	// Subtype is the response type (success, error)
	Subtype string `json:"subtype"`

	// RequestID is the ID of the control_request this responds to
	RequestID string `json:"request_id"`

	// Response contains the response data (e.g., slash_commands for initialize)
	Response *InitializeResponseData `json:"response,omitempty"`

	// Error message if subtype is "error"
	Error string `json:"error,omitempty"`
}

// InitializeResponseData contains the response data from an initialize control request.
type InitializeResponseData struct {
	// Commands are the available commands (Claude Code uses "commands" not "slash_commands")
	Commands []Command `json:"commands,omitempty"`

	// Agents are the available agent names
	Agents []string `json:"agents,omitempty"`
}

// Command represents an available command from the initialize response.
type Command struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	ArgumentHint string `json:"argumentHint,omitempty"`
}

// ControlResponse is the response to a control request.
type ControlResponse struct {
	// Subtype is the response type (success, error)
	Subtype string `json:"subtype"`

	// For success responses
	Result *PermissionResult `json:"result,omitempty"`

	// For error responses
	Error string `json:"error,omitempty"`
}

// PermissionResult is the result for tool approval responses.
type PermissionResult struct {
	// Behavior is "allow" or "deny"
	Behavior string `json:"behavior"`

	// UpdatedInput allows modifying the tool input
	UpdatedInput any `json:"updatedInput,omitempty"`

	// UpdatedPermissions adds permission rules for future requests
	UpdatedPermissions []PermissionUpdate `json:"updatedPermissions,omitempty"`

	// Message provides feedback to the model
	Message string `json:"message,omitempty"`

	// Interrupt stops the current operation (for deny)
	Interrupt *bool `json:"interrupt,omitempty"`
}

// SDKControlRequest is a control request sent to Claude Code CLI.
// Used for initialize, interrupt, and other control operations.
type SDKControlRequest struct {
	Type      string                `json:"type"` // "control_request"
	RequestID string                `json:"request_id"`
	Request   SDKControlRequestBody `json:"request"`
}

// SDKControlRequestBody contains the body of an SDK control request.
type SDKControlRequestBody struct {
	// Subtype identifies the operation (initialize, interrupt, set_permission_mode)
	Subtype string `json:"subtype"`

	// For initialize requests
	Hooks map[string]any `json:"hooks,omitempty"`

	// For set_permission_mode requests
	Mode string `json:"mode,omitempty"`
}

// UserMessage is sent to provide a prompt to Claude Code.
type UserMessage struct {
	Type    string          `json:"type"` // "user"
	Message UserMessageBody `json:"message"`
}

// UserMessageBody contains the user message content.
type UserMessageBody struct {
	Role    string `json:"role"` // "user"
	Content string `json:"content"`
}

// StreamEvent represents a streaming event from Claude Code.
// These are partial content updates during processing.
type StreamEvent struct {
	Type string `json:"type"`

	// Content stream events
	Index       int    `json:"index,omitempty"`
	ContentType string `json:"content_type,omitempty"`

	// For text_delta events
	Delta *TextDelta `json:"delta,omitempty"`

	// For thinking_delta events
	ThinkingDelta string `json:"thinking_delta,omitempty"`
}

// TextDelta contains a partial text update.
type TextDelta struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ToolApprovalOptions represents the available options for tool approval.
type ToolApprovalOptions struct {
	Allow       bool `json:"allow"`
	AlwaysAllow bool `json:"always_allow"`
	Deny        bool `json:"deny"`
}

// Common tool names used by Claude Code
const (
	ToolBash         = "Bash"
	ToolWrite        = "Write"
	ToolEdit         = "Edit"
	ToolNotebookEdit = "NotebookEdit"
	ToolRead         = "Read"
	ToolGlob         = "Glob"
	ToolGrep         = "Grep"
	ToolTask         = "Task"
	ToolTaskCreate   = "TaskCreate"
	ToolTaskUpdate   = "TaskUpdate"
	ToolTaskList     = "TaskList"
	ToolTaskGet      = "TaskGet"
	ToolTodoWrite    = "TodoWrite"
	ToolWebFetch     = "WebFetch"
	ToolWebSearch    = "WebSearch"
)
