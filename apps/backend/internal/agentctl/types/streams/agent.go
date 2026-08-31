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

	// EventTypeAgentPlan indicates rich plan content (markdown) from the agent.
	EventTypeAgentPlan = "agent_plan"

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

	// EventTypeForegroundIdle indicates the provider finished its foreground
	// model cycle even when the prompt RPC remains open for background work.
	EventTypeForegroundIdle = "foreground_idle"

	// EventTypeBackgroundComplete indicates one provider-reported background
	// workload completed. ToolCallID carries a stable provider work identity when
	// available. Some providers expose no identity on the terminal frame; consumers
	// must fail closed rather than guessing among multiple registrations.
	EventTypeBackgroundComplete = "background_complete"

	// EventTypeAvailableCommands indicates available slash commands from the agent.
	EventTypeAvailableCommands = "available_commands"

	// EventTypeSessionMode indicates the agent's session mode has changed.
	EventTypeSessionMode = "session_mode"

	// EventTypeRateLimit indicates the agent is being rate-limited by the API.
	EventTypeRateLimit = "rate_limit"

	// EventTypeAgentCapabilities indicates agent capabilities from ACP initialize.
	EventTypeAgentCapabilities = "agent_capabilities"

	// EventTypeSessionModels indicates available models from ACP session/new.
	EventTypeSessionModels = "session_models"

	// EventTypeSessionModelFallback indicates the session started on the
	// profile's fallback model because the configured start model was
	// unavailable. Data carries {"fallback_model": <model id>}.
	EventTypeSessionModelFallback = "session_model_fallback"

	// EventTypeSessionModelSelectionWarning explains an executor-authoritative
	// default or explicit fallback decision.
	EventTypeSessionModelSelectionWarning = "session_model_selection_warning"

	// EventTypeSessionInfo indicates ACP session metadata such as title changed.
	EventTypeSessionInfo = "session_info"

	// EventTypeAuthRequired indicates the agent rejected session/new with an
	// authentication-required error. The event carries the available auth
	// methods (from ACP initialize); the client picks one and replays the
	// authenticate → session/new round-trip.
	EventTypeAuthRequired = "auth_required"

	// EventTypeMCPAttachment carries safe, session-scoped evidence about MCP
	// configuration delivery and observed use. It deliberately excludes raw
	// protocol frames, tool payloads, credentials, and full endpoints.
	EventTypeMCPAttachment = "mcp_attachment"
)

// AgentEventDataPromptHandoff marks a generation-bearing foreground-idle event
// whose provider and transport have attested that the next human prompt may
// take ownership before the held session/prompt RPC returns.
const AgentEventDataPromptHandoff = "prompt_handoff"

// Session status constants for EventTypeSessionStatus events.
const (
	SessionStatusResumed = "resumed"
	SessionStatusNew     = "new"
)

// AgentEvent is the message type streamed from the agent process.
// This represents agent-agnostic events. ACP session updates are normalized
// into this shape; agentctl also synthesizes lifecycle and diagnostic events
// (process exit, MCP attachment, permission cancellation) that never came from
// the agent.
//
// Stream endpoint: ws://.../api/v1/agent/events
type AgentEvent struct {
	// Type identifies the event type. Use the EventType* constants for supported
	// values.
	Type string `json:"type"`

	// SessionID is the current session identifier.
	SessionID string `json:"session_id,omitempty"`

	// OperationID identifies an operation when the agent exposes an operation ID.
	// It may be empty when no operation ID is available.
	OperationID string `json:"operation_id,omitempty"`

	// PromptGeneration is the lifecycle-owned identity assigned when this
	// prompt was accepted. Terminal events and ownership-sensitive lifecycle
	// boundaries such as foreground-idle echo it so delayed events cannot be
	// attributed to a newer prompt on the same execution.
	PromptGeneration uint64 `json:"prompt_generation,omitempty"`

	// TurnID is the durable Kandev turn identity captured for terminal events.
	// It travels with the completion frame so consumers do not have to infer
	// ownership from cross-subject event ordering.
	TurnID string `json:"turn_id,omitempty"`

	// --- Message fields (for "message_chunk" type) ---

	// Text contains streaming text content from the agent.
	Text string `json:"text,omitempty"`

	// ProtocolMessageID identifies the source-protocol message this chunk belongs to.
	// It is distinct from the Kandev message record ID used by downstream streaming.
	ProtocolMessageID string `json:"protocol_message_id,omitempty"`

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

	// --- Plan fields (for "plan" and "agent_plan" types) ---

	// PlanEntries contains the agent's execution plan/task list.
	PlanEntries []PlanEntry `json:"plan_entries,omitempty"`

	// PlanContent contains rich markdown plan content (for "agent_plan" type).
	PlanContent string `json:"plan_content,omitempty"`

	// --- Error fields (for "error" type) ---

	// Error contains error message when Type is "error".
	Error string `json:"error,omitempty"`

	// ProviderError contains a validated, sanitized provider diagnostic for a
	// terminal error. Raw stderr never crosses this boundary.
	ProviderError *ProviderError `json:"provider_error,omitempty"`

	// MCPAttachment contains one safe observation from the MCP attachment
	// lifecycle when Type is EventTypeMCPAttachment.
	MCPAttachment *MCPAttachmentEvidence `json:"mcp_attachment,omitempty"`

	// MCPAttachmentAttempt starts a new backend-owned evidence timeline. It is
	// present only on the first EventTypeMCPAttachment event for an attempt.
	MCPAttachmentAttempt *MCPAttachmentAttempt `json:"mcp_attachment_attempt,omitempty"`

	// --- Permission request fields (for "permission_request" type) ---
	// RequestID is the Kandev-generated identity for this exact request
	// generation. It is distinct from the provider-controlled PendingID.
	RequestID string `json:"request_id,omitempty"`

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

	// --- Rate limit fields (for "rate_limit" type) ---

	// RateLimitMessage contains a human-readable rate limit message from the API.
	RateLimitMessage string `json:"rate_limit_message,omitempty"`

	// LastMessageUUID is the UUID of the last committed message, used for --resume-session-at.
	LastMessageUUID string `json:"last_message_uuid,omitempty"`

	// --- Session mode fields (for "session_mode" type) ---

	// CurrentModeID is the active session mode identifier (e.g., "code", "ask", "architect").
	CurrentModeID string `json:"current_mode_id,omitempty"`

	// AvailableModes lists the modes the agent supports.
	AvailableModes []SessionModeInfo `json:"available_modes,omitempty"`

	// --- Agent capabilities fields (for "agent_capabilities" type) ---

	// SupportsImage indicates the agent natively supports image content blocks.
	SupportsImage bool `json:"supports_image"`

	// SupportsAudio indicates the agent natively supports audio content blocks.
	SupportsAudio bool `json:"supports_audio"`

	// SupportsEmbeddedContext indicates the agent supports embedded context.
	SupportsEmbeddedContext bool `json:"supports_embedded_context"`

	// SupportsPromptQueueing indicates the agent advertised that it accepts a
	// prompt while another prompt for the same session is still in flight. It is
	// the negotiated precondition for prompt handoff and mid-turn steering, and
	// does not by itself promise the agent will fold that prompt into the running
	// turn. See docs/specs/platform/requirements/mid-turn-steering.md.
	SupportsPromptQueueing bool `json:"supports_prompt_queueing"`

	// AuthMethods lists authentication methods from ACP initialize.
	AuthMethods []AuthMethodInfo `json:"auth_methods,omitempty"`

	// --- Session models fields (for "session_models" type) ---

	// CurrentModelID is the active model identifier.
	CurrentModelID string `json:"current_model_id,omitempty"`

	// FallbackModel carries the fallback model a session started on when
	// the configured start model was unavailable (session_model_fallback
	// events).
	FallbackModel string `json:"fallback_model,omitempty"`

	// ModelSelectionWarning carries the provider-neutral model decision made
	// before the session's first prompt.
	ModelSelectionWarning *ModelSelectionWarning `json:"model_selection_warning,omitempty"`

	// SessionModels lists models available in the ACP session.
	SessionModels []SessionModelInfo `json:"session_models,omitempty"`

	// ConfigOptions lists session configuration options from ACP _meta.
	ConfigOptions []ConfigOption `json:"config_options,omitempty"`

	// ConfigBaselineCandidate carries an authoritative response snapshot for
	// lifecycle settlement without replacing the event's newer live options.
	ConfigBaselineCandidate []ConfigOption `json:"config_baseline_candidate,omitempty"`

	// OriginalConfigCandidate is the profile-settled model-adjacent state used
	// once to capture a task session's immutable original configuration. It is
	// emitted before runtime/workflow overrides are applied.
	OriginalConfigCandidate []ConfigOption `json:"original_config_candidate,omitempty"`

	// --- Session info fields ---

	// SessionTitle is the agent-provided human-readable ACP session title.
	SessionTitle string `json:"session_title,omitempty"`

	// SessionUpdatedAt is the provider-reported ISO 8601 last activity timestamp.
	SessionUpdatedAt string `json:"session_updated_at,omitempty"`

	// SessionMeta contains opaque ACP _meta from session_info_update.
	SessionMeta map[string]any `json:"session_meta,omitempty"`

	// --- Usage fields (attached to "complete" event) ---

	// Usage contains token usage stats from the prompt response.
	Usage *PromptUsage `json:"usage,omitempty"`
}

// ModelSelectionWarning is the structured, provider-neutral explanation for
// continuing with an executor model other than the saved profile request.
type ModelSelectionWarning struct {
	Kind              string `json:"kind"`
	DecisionID        string `json:"decision_id,omitempty"`
	Reason            string `json:"reason"`
	RequestedModel    string `json:"requested_model,omitempty"`
	EffectiveModel    string `json:"effective_model,omitempty"`
	FallbackModel     string `json:"fallback_model,omitempty"`
	AgentID           string `json:"agent_id,omitempty"`
	ExecutorType      string `json:"executor_type,omitempty"`
	ExecutorProfileID string `json:"executor_profile_id,omitempty"`
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

// SessionModeInfo represents an available session mode from ACP.
type SessionModeInfo struct {
	// ID is the mode identifier (e.g., "code", "ask", "architect").
	ID string `json:"id"`

	// Name is a human-readable name for the mode.
	Name string `json:"name"`

	// Description provides additional context about the mode.
	Description string `json:"description,omitempty"`
}

// SessionModelInfo represents a model available in an ACP session.
type SessionModelInfo struct {
	// ModelID is the model identifier.
	ModelID string `json:"model_id"`

	// Name is a human-readable model name.
	Name string `json:"name"`

	// Description provides additional context about the model.
	Description string `json:"description,omitempty"`

	// UsageMultiplier is the normalized pricing multiplier (e.g., "1x", "3x").
	UsageMultiplier string `json:"usage_multiplier,omitempty"`

	// Meta contains raw _meta from the agent for agent-specific rendering.
	Meta map[string]any `json:"meta,omitempty"`
}

// SessionModelState is the synchronous model snapshot returned with a newly
// created or loaded session. It lets lifecycle callers apply a saved model
// before the asynchronous session_models stream event is dispatched.
type SessionModelState struct {
	CurrentModelID string             `json:"current_model_id,omitempty"`
	Models         []SessionModelInfo `json:"models,omitempty"`
	ConfigOptions  []ConfigOption     `json:"config_options,omitempty"`
}

// AuthMethodInfo represents an authentication method from ACP initialize.
type AuthMethodInfo struct {
	// ID is the auth method identifier.
	ID string `json:"id"`

	// Name is a human-readable name for the auth method.
	Name string `json:"name"`

	// Description provides additional context.
	Description string `json:"description,omitempty"`

	// TerminalAuth contains normalized terminal-based auth info from _meta.
	TerminalAuth *TerminalAuth `json:"terminal_auth,omitempty"`

	// Meta contains raw _meta from the agent.
	Meta map[string]any `json:"meta,omitempty"`
}

// TerminalAuth contains normalized terminal-based authentication info.
type TerminalAuth struct {
	// Command is the CLI command to run (e.g., "copilot").
	Command string `json:"command"`

	// Args are the command arguments (e.g., ["auth", "login"]).
	Args []string `json:"args,omitempty"`

	// Label is a human-readable description of the auth action.
	Label string `json:"label,omitempty"`
}

// ConfigOption represents a session configuration option from ACP _meta.
type ConfigOption struct {
	// Type is the option type: "select", "toggle", etc.
	Type string `json:"type"`

	// ID is the option identifier (e.g., "mode", "model", "reasoning_effort").
	ID string `json:"id"`

	// Name is a human-readable name.
	Name string `json:"name"`

	// Description is optional provider-supplied guidance for the option.
	Description string `json:"description,omitempty"`

	// CurrentValue is the currently selected value.
	CurrentValue string `json:"current_value"`

	// Category groups related options.
	Category string `json:"category,omitempty"`

	// Options lists the selectable values.
	Options []ConfigOptionValue `json:"options,omitempty"`
}

// ConfigOptionValue represents a selectable value for a ConfigOption.
type ConfigOptionValue struct {
	// Value is the option value.
	Value string `json:"value"`

	// Name is a human-readable label.
	Name string `json:"name"`

	// Description is optional provider-supplied guidance for the value.
	Description string `json:"description,omitempty"`
}

// PromptUsage contains token usage info from an ACP prompt response.
//
// ThoughtTokens is emitted by opencode-acp (reasoning models). It is
// captured for forward use but does not feed the pricing math today.
//
// ProviderReportedCostSubcents is the CLI's own USD cost for the turn
// (claude-acp emits this on usage_update.cost.amount; opencode-acp
// sometimes emits 0 on BYOK). The amount is stored as int64 hundredths
// of a cent (amount_usd * 10000). When > 0 the office subscriber records
// it verbatim and skips the models.dev pricing lookup.
//
// Estimated is true when the token counts do not represent an
// authoritative per-turn measurement: either the adapter synthesised them
// (e.g. codex-acp's cumulative-delta inference when no typed usage frame
// is present at all — fallbackUsageForNilTypedUsage in
// server/adapter/transport/acp/adapter_prompt.go), or the typed frame
// itself is scoped to less than the whole turn (codex-acp's usage field is
// hardcoded to the LAST model request of a multi-request turn, not a
// per-turn total — see normalizeCodexPromptUsage in
// server/adapter/transport/acp/dialect_codex.go). Rows flagged estimated
// still count toward budget totals at face value per
// docs/specs/office/requirements/costs.md.
type PromptUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`

	// OutputTokensPresent distinguishes an observed zero from a missing
	// output-token sample. Adapters set it false when they cannot observe output
	// tokens, such as the context-occupancy fallback. It is always serialized so
	// downstream consumers can distinguish false from a legacy missing field.
	OutputTokensPresent bool `json:"output_tokens_present"`

	CachedReadTokens             int64 `json:"cached_read_tokens,omitempty"`
	CachedWriteTokens            int64 `json:"cached_write_tokens,omitempty"`
	ThoughtTokens                int64 `json:"thought_tokens,omitempty"`
	TotalTokens                  int64 `json:"total_tokens"`
	ProviderReportedCostSubcents int64 `json:"provider_reported_cost_subcents,omitempty"`
	// ProviderReportedCostPresent distinguishes an explicit provider-reported
	// zero from a missing cost sample. Providers can legitimately report zero
	// for BYOK/free turns, and that value must still suppress list-price
	// estimation downstream.
	ProviderReportedCostPresent bool `json:"provider_reported_cost_present,omitempty"`
	Estimated                   bool `json:"estimated,omitempty"`
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
