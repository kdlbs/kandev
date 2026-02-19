package streams

// AgentEvent type constants define the types of events streamed from the agent.
const (
	// EventTypeMessageChunk indicates streaming text content from the agent.
	EventTypeMessageChunk = "message_chunk"

	// EventTypeReasoning indicates chain-of-thought or thinking content.
	EventTypeReasoning = "reasoning"

	// EventTypeToolCall indicates a tool invocation has started.
	EventTypeToolCall = "tool_call"

	// EventTypeToolUpdate indicates a tool status update (running, completed, etc.).
	EventTypeToolUpdate = "tool_update"

	// EventTypePlan indicates agent plan/task list updates.
	EventTypePlan = "plan"

	// EventTypeComplete indicates the turn or session has completed.
	EventTypeComplete = "complete"

	// EventTypeError indicates an error occurred.
	EventTypeError = "error"

	// EventTypePermissionRequest indicates the agent is requesting permission.
	EventTypePermissionRequest = "permission_request"

	// EventTypePermissionCancelled indicates a permission request was cancelled.
	// This happens when the agent completes or the context is cancelled before
	// the user responds to the permission request.
	EventTypePermissionCancelled = "permission_cancelled"

	// EventTypeSessionStatus indicates a session status update (resumed or new).
	EventTypeSessionStatus = "session_status"

	// EventTypeContextWindow indicates a context window update.
	EventTypeContextWindow = "context_window"

	// EventTypeAvailableCommands indicates available slash commands from the agent.
	EventTypeAvailableCommands = "available_commands"

	// EventTypeSessionMode indicates the agent's session mode has changed.
	EventTypeSessionMode = "session_mode"
)

// Session status constants for EventTypeSessionStatus events.
const (
	SessionStatusResumed = "resumed"
	SessionStatusNew     = "new"
)

// AgentEvent is the message type streamed from the agent process.
// This represents protocol-agnostic events from the agent, normalized from
// various underlying protocols (ACP, Codex, Claude Code, etc.).
//
// Stream endpoint: ws://.../api/v1/agent/events
type AgentEvent struct {
	// Type identifies the event type. Use EventType* constants:
	// "message_chunk", "reasoning", "tool_call", "tool_update", "plan", "complete", "error"
	Type string `json:"type"`

	// SessionID is the current session identifier.
	SessionID string `json:"session_id,omitempty"`

	// OperationID identifies the current in-flight operation (turn, prompt, etc.).
	// Used to target specific operations for cancellation or status updates.
	// For Codex this is the turn ID, for other protocols it may be empty.
	OperationID string `json:"operation_id,omitempty"`

	// --- Message fields (for "message_chunk" type) ---

	// Text contains streaming text content from the agent.
	Text string `json:"text,omitempty"`

	// --- Reasoning fields (for "reasoning" type) ---

	// ReasoningText contains full reasoning/chain-of-thought content.
	ReasoningText string `json:"reasoning_text,omitempty"`

	// ReasoningSummary contains a summarized version of reasoning (if available).
	ReasoningSummary string `json:"reasoning_summary,omitempty"`

	// --- Tool call fields (for "tool_call" and "tool_update" types) ---

	// ToolCallID uniquely identifies the tool invocation.
	ToolCallID string `json:"tool_call_id,omitempty"`

	// ParentToolCallID identifies the parent Task tool call when this event
	// comes from a subagent. Used for visual nesting in the UI.
	ParentToolCallID string `json:"parent_tool_call_id,omitempty"`

	// ToolName is the name of the tool being invoked.
	ToolName string `json:"tool_name,omitempty"`

	// ToolTitle is a human-readable title for the tool call.
	ToolTitle string `json:"tool_title,omitempty"`

	// ToolStatus indicates the current status: "started", "running", "completed", "error".
	ToolStatus string `json:"tool_status,omitempty"`

	// NormalizedPayload contains the normalized tool data as a typed discriminated union.
	// Provides typed access to tool parameters based on the Kind field.
	NormalizedPayload *NormalizedPayload `json:"normalized,omitempty"`

	// Diff contains unified diff content for file changes.
	// Populated when tools modify files, providing the aggregated diff.
	Diff string `json:"diff,omitempty"`

	// --- Plan fields (for "plan" type) ---

	// PlanEntries contains the agent's execution plan/task list.
	PlanEntries []PlanEntry `json:"plan_entries,omitempty"`

	// --- Error fields (for "error" type) ---

	// Error contains error message when Type is "error".
	Error string `json:"error,omitempty"`

	// --- Permission request fields (for "permission_request" type) ---

	// PendingID uniquely identifies this pending permission request.
	PendingID string `json:"pending_id,omitempty"`

	// PermissionTitle is a human-readable description of the action requiring permission.
	PermissionTitle string `json:"permission_title,omitempty"`

	// PermissionOptions contains the available permission choices.
	PermissionOptions []PermissionOption `json:"permission_options,omitempty"`

	// ActionType categorizes the action requiring approval.
	// Use ActionType* constants: "command", "file_write", "file_read", "network", "mcp_tool", "other".
	ActionType string `json:"action_type,omitempty"`

	// ActionDetails contains structured details about the action.
	ActionDetails map[string]any `json:"action_details,omitempty"`

	// --- Session status fields (for "session_status" type) ---

	// SessionStatus indicates whether the session was resumed or new.
	// Use SessionStatus* constants: "resumed", "new".
	SessionStatus string `json:"session_status,omitempty"`

	// --- Extension fields ---

	// Data contains raw protocol-specific extensions.
	Data map[string]any `json:"data,omitempty"`

	// --- Context window fields ---

	// ContextWindowSize is the total available tokens in the context window.
	ContextWindowSize int64 `json:"context_window_size,omitempty"`

	// ContextWindowUsed is the number of tokens currently consumed.
	ContextWindowUsed int64 `json:"context_window_used,omitempty"`

	// ContextWindowRemaining is the number of available tokens left.
	ContextWindowRemaining int64 `json:"context_window_remaining,omitempty"`

	// ContextEfficiency is the percentage utilization (0-100).
	ContextEfficiency float64 `json:"context_efficiency,omitempty"`

	// --- Available commands fields (for "available_commands" type) ---

	// AvailableCommands contains the slash commands available from the agent.
	AvailableCommands []AvailableCommand `json:"available_commands,omitempty"`

	// --- Multimodal content fields ---

	// ContentBlocks contains multimodal content (images, audio, resource links, etc.).
	// Text-only messages still use the Text field for backward compatibility.
	ContentBlocks []ContentBlock `json:"content_blocks,omitempty"`

	// Role distinguishes user vs assistant messages (e.g., during session/load replay).
	// Values: "user", "assistant". Empty defaults to "assistant".
	Role string `json:"role,omitempty"`

	// --- Tool call content fields (for "tool_call" and "tool_update" types) ---

	// ToolCallContents contains rich content produced by a tool call (diffs, text, terminals).
	ToolCallContents []ToolCallContentItem `json:"tool_call_contents,omitempty"`

	// --- Session mode fields (for "session_mode" type) ---

	// CurrentModeID is the active session mode identifier (e.g., "code", "ask", "architect").
	CurrentModeID string `json:"current_mode_id,omitempty"`
}

// PlanEntry represents an entry in the agent's execution plan.
type PlanEntry struct {
	// Description is the content/description of the task.
	Description string `json:"description,omitempty"`

	// Status indicates task status: "pending", "in_progress", "completed", "failed".
	Status string `json:"status,omitempty"`

	// Priority indicates relative importance.
	Priority string `json:"priority,omitempty"`
}

// AvailableCommand represents a slash command available from the agent.
type AvailableCommand struct {
	// Name is the command name (e.g., "draftpr", "commit").
	Name string `json:"name"`

	// Description is a human-readable description of the command.
	Description string `json:"description,omitempty"`

	// InputHint is a hint displayed when the command expects additional input.
	InputHint string `json:"input_hint,omitempty"`
}

// ContentBlock represents a multimodal content block from the agent.
// Supports text, image, audio, resource_link, and resource types.
type ContentBlock struct {
	// Type identifies the content block type: "text", "image", "audio", "resource_link", "resource".
	Type string `json:"type"`

	// Text contains text content (for "text" type).
	Text string `json:"text,omitempty"`

	// Data contains base64-encoded binary data (for "image" and "audio" types).
	Data string `json:"data,omitempty"`

	// MimeType identifies the media type (for "image", "audio", "resource_link" types).
	MimeType string `json:"mime_type,omitempty"`

	// URI is the resource location (for "image", "resource_link" types).
	URI string `json:"uri,omitempty"`

	// Name identifies the resource (for "resource_link" type).
	Name string `json:"name,omitempty"`

	// Title is a human-readable title (for "resource_link" type).
	Title string `json:"title,omitempty"`

	// Description provides additional context (for "resource_link" type).
	Description string `json:"description,omitempty"`

	// Size is the resource size in bytes (for "resource_link" type).
	Size *int `json:"size,omitempty"`
}

// ToolCallContentItem represents a content item produced by a tool call.
// This is a discriminated union: exactly one of Content, Diff, or Terminal fields will be set.
type ToolCallContentItem struct {
	// Type identifies the variant: "content", "diff", or "terminal".
	Type string `json:"type"`

	// Content is a standard content block (text, image, resource). Set when Type is "content".
	Content *ContentBlock `json:"content,omitempty"`

	// Diff fields are set when Type is "diff".
	Path    string  `json:"path,omitempty"`
	OldText *string `json:"old_text,omitempty"` // nil for new files
	NewText string  `json:"new_text,omitempty"`

	// TerminalID is set when Type is "terminal".
	TerminalID string `json:"terminal_id,omitempty"`
}
