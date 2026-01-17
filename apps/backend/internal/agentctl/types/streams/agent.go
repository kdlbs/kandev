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

	// ToolName is the name of the tool being invoked.
	ToolName string `json:"tool_name,omitempty"`

	// ToolTitle is a human-readable title for the tool call.
	ToolTitle string `json:"tool_title,omitempty"`

	// ToolStatus indicates the current status: "started", "running", "completed", "error".
	ToolStatus string `json:"tool_status,omitempty"`

	// ToolArgs contains the arguments passed to the tool.
	ToolArgs map[string]interface{} `json:"tool_args,omitempty"`

	// ToolResult contains the result from the tool execution.
	ToolResult interface{} `json:"tool_result,omitempty"`

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
	ActionDetails map[string]interface{} `json:"action_details,omitempty"`

	// --- Extension fields ---

	// Data contains raw protocol-specific extensions.
	Data map[string]interface{} `json:"data,omitempty"`
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
