// Package codex provides types and client for the OpenAI Codex app-server protocol.
// Codex uses a JSON-RPC 2.0 variant over stdio, but omits the "jsonrpc":"2.0" header.
package codex

import "encoding/json"

// Request represents a Codex JSON-RPC request (without jsonrpc field)
type Request struct {
	ID     interface{}     `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response represents a Codex JSON-RPC response
type Response struct {
	ID     interface{}     `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// Notification represents a Codex notification (no id field)
type Notification struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Error represents a JSON-RPC error
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Standard error codes
const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

// Codex method names
const (
	MethodInitialize    = "initialize"
	MethodInitialized   = "initialized" // Notification
	MethodThreadStart   = "thread/start"
	MethodThreadResume  = "thread/resume"
	MethodThreadFork    = "thread/fork"
	MethodThreadList    = "thread/list"
	MethodThreadArchive = "thread/archive"
	MethodTurnStart     = "turn/start"
	MethodTurnInterrupt = "turn/interrupt"
	MethodCommandExec   = "command/exec"
	MethodModelList     = "model/list"
	MethodSkillsList    = "skills/list"
	MethodAccountRead   = "account/read"
	MethodAccountLogout = "account/logout"
	MethodReviewStart   = "review/start"
	MethodConfigRead    = "config/read"
)

// Codex notification methods (server → client)
const (
	NotifyThreadStarted                 = "thread/started"
	NotifyTurnStarted                   = "turn/started"
	NotifyTurnCompleted                 = "turn/completed"
	NotifyTurnDiffUpdated               = "turn/diff/updated"
	NotifyTurnPlanUpdated               = "turn/plan/updated"
	NotifyItemStarted                   = "item/started"
	NotifyItemCompleted                 = "item/completed"
	NotifyItemAgentMessageDelta         = "item/agentMessage/delta"
	NotifyItemReasoningSummaryDelta     = "item/reasoning/summaryTextDelta"
	NotifyItemReasoningTextDelta        = "item/reasoning/textDelta"
	NotifyItemCmdExecOutputDelta        = "item/commandExecution/outputDelta"
	NotifyItemCmdExecRequestApproval    = "item/commandExecution/requestApproval"
	NotifyItemFileChangeRequestApproval = "item/fileChange/requestApproval"
	NotifyAccountUpdated                = "account/updated"
	NotifyAccountLoginCompleted         = "account/login/completed"
	NotifyError                         = "error"
	NotifyTokenCount                    = "token_count"
	NotifyThreadTokenUsageUpdated       = "thread/tokenUsage/updated"
	NotifyContextCompacted              = "context_compacted"
)

// InitializeParams for initialize request
type InitializeParams struct {
	ClientInfo *ClientInfo `json:"clientInfo"`
}

// ClientInfo identifies the client
type ClientInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version"`
}

// InitializeResult from initialize
type InitializeResult struct {
	UserAgent string `json:"userAgent,omitempty"`
}

// ThreadStartParams for thread/start
type ThreadStartParams struct {
	Model          string         `json:"model,omitempty"`
	Cwd            string         `json:"cwd,omitempty"`
	ApprovalPolicy string         `json:"approvalPolicy,omitempty"` // "untrusted", "on-failure", "on-request", "never"
	Sandbox        string         `json:"sandbox,omitempty"`        // "workspaceWrite", "readOnly", etc.
	SandboxPolicy  *SandboxPolicy `json:"sandboxPolicy,omitempty"`
}

// SandboxPolicy configures sandbox behavior.
// Type must use kebab-case values per Codex documentation:
// - "read-only": prevents edits, command execution, and network access
// - "workspace-write": allows reads, edits, and commands within the active workspace
// - "danger-full-access": removes all sandbox constraints (not recommended)
type SandboxPolicy struct {
	Type          string   `json:"type"` // "workspace-write", "read-only", "danger-full-access"
	WritableRoots []string `json:"writableRoots,omitempty"`
	NetworkAccess bool     `json:"networkAccess,omitempty"`
}

// Thread represents a Codex thread (conversation)
type Thread struct {
	ID            string `json:"id"`
	Preview       string `json:"preview,omitempty"`
	ModelProvider string `json:"modelProvider,omitempty"`
	CreatedAt     int64  `json:"createdAt,omitempty"`
}

// ThreadStartResult from thread/start
type ThreadStartResult struct {
	Thread *Thread `json:"thread"`
}

// ThreadResumeParams for thread/resume
type ThreadResumeParams struct {
	ThreadID       string         `json:"threadId"`
	Cwd            string         `json:"cwd,omitempty"`
	ApprovalPolicy string         `json:"approvalPolicy,omitempty"` // "untrusted", "on-failure", "on-request", "never"
	SandboxPolicy  *SandboxPolicy `json:"sandboxPolicy,omitempty"`
}

// ThreadResumeResult from thread/resume
type ThreadResumeResult struct {
	Thread *Thread `json:"thread"`
}

// UserInput represents input to a turn
type UserInput struct {
	Type string `json:"type"` // "text", "image", "localImage", "skill"
	Text string `json:"text,omitempty"`
	URL  string `json:"url,omitempty"`
	Path string `json:"path,omitempty"`
	Name string `json:"name,omitempty"`
}

// TurnStartParams for turn/start
type TurnStartParams struct {
	ThreadID string      `json:"threadId"`
	Input    []UserInput `json:"input"`
}

// Turn represents a Codex turn within a thread
type Turn struct {
	ID     string `json:"id"`
	Status string `json:"status"` // "inProgress", "completed", "failed"
	Items  []Item `json:"items"`
	Error  *Error `json:"error,omitempty"`
}

// TurnStartResult from turn/start
type TurnStartResult struct {
	Turn *Turn `json:"turn"`
}

// Item represents a Codex item (message, command, file change, etc.)
type Item struct {
	ID     string `json:"id"`
	Type   string `json:"type"`   // "userMessage", "agentMessage", "commandExecution", "fileChange", "reasoning", "mcpToolCall"
	Status string `json:"status"` // "inProgress", "completed", "failed"

	// For commandExecution type
	Command          string `json:"command,omitempty"`
	Cwd              string `json:"cwd,omitempty"`
	AggregatedOutput string `json:"aggregatedOutput,omitempty"`
	ExitCode         *int   `json:"exitCode,omitempty"`
	DurationMs       *int   `json:"durationMs,omitempty"`

	// For fileChange type
	Changes []FileChange `json:"changes,omitempty"`

	// For reasoning type - content can be objects like [{type: "text", text: "..."}]
	// or plain strings. FlexibleContent handles both formats.
	Summary FlexibleContent `json:"summary,omitempty"`
	Content FlexibleContent `json:"content,omitempty"`

	// For mcpToolCall type
	Server    string          `json:"server,omitempty"`
	Tool      string          `json:"tool,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	ToolError string          `json:"error,omitempty"` // Named ToolError to avoid conflict with Error type
}

// ContentPart represents a content part in a Codex item.
// This handles the OpenAI responses format where content is an array of typed objects.
type ContentPart struct {
	Type string `json:"type,omitempty"` // "text", "output_text", "refusal", "input_text", etc.
	Text string `json:"text,omitempty"`
}

// FlexibleContent is a type that can unmarshal from either a string or []ContentPart.
// Codex sometimes sends summary/content as a plain string, other times as an array.
type FlexibleContent []ContentPart

// UnmarshalJSON implements custom unmarshaling for FlexibleContent.
// It handles both string and array formats from Codex.
func (fc *FlexibleContent) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as array first (most common case)
	var parts []ContentPart
	if err := json.Unmarshal(data, &parts); err == nil {
		*fc = parts
		return nil
	}

	// Try to unmarshal as string
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*fc = []ContentPart{{Type: "text", Text: str}}
		return nil
	}

	// If both fail, return empty (don't fail parsing)
	*fc = nil
	return nil
}

// FileChange represents a file change in a fileChange item
type FileChange struct {
	Path string         `json:"path"`
	Kind FileChangeKind `json:"kind"`
	Diff string         `json:"diff,omitempty"`
}

// FileChangeKind represents the type of file change
type FileChangeKind struct {
	Type string `json:"type"` // "add", "modify", "delete"
}

// ItemStartedParams for item/started notification
type ItemStartedParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	Item     *Item  `json:"item"`
}

// ItemCompletedParams for item/completed notification
type ItemCompletedParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	Item     *Item  `json:"item"`
}

// AgentMessageDeltaParams for item/agentMessage/delta
type AgentMessageDeltaParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	ItemID   string `json:"itemId"`
	Delta    string `json:"delta"`
}

// ReasoningDeltaParams for item/reasoning/textDelta and summaryTextDelta
type ReasoningDeltaParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	ItemID   string `json:"itemId"`
	Delta    string `json:"delta"`
}

// CommandOutputDeltaParams for item/commandExecution/outputDelta
type CommandOutputDeltaParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	ItemID   string `json:"itemId"`
	Delta    string `json:"delta"`
}

// CommandApprovalParams for item/commandExecution/requestApproval
type CommandApprovalParams struct {
	ThreadID  string   `json:"threadId"`
	TurnID    string   `json:"turnId"`
	ItemID    string   `json:"itemId"`
	Command   string   `json:"command"`
	Cwd       string   `json:"cwd,omitempty"`
	Reasoning string   `json:"reasoning,omitempty"`
	Options   []string `json:"options,omitempty"` // e.g., ["approve", "reject", "approveAlways"]
}

// FileChangeApprovalParams for item/fileChange/requestApproval
type FileChangeApprovalParams struct {
	ThreadID  string   `json:"threadId"`
	TurnID    string   `json:"turnId"`
	ItemID    string   `json:"itemId"`
	Path      string   `json:"path"`
	Diff      string   `json:"diff,omitempty"`
	Reasoning string   `json:"reasoning,omitempty"`
	Options   []string `json:"options,omitempty"`
}

// TurnCompletedParams for turn/completed notification
type TurnCompletedParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

// TurnDiffUpdatedParams for turn/diff/updated notification
type TurnDiffUpdatedParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	Diff     string `json:"diff"`
}

// TurnPlanUpdatedParams for turn/plan/updated notification
type TurnPlanUpdatedParams struct {
	ThreadID string      `json:"threadId"`
	TurnID   string      `json:"turnId"`
	Plan     []PlanEntry `json:"plan"`
}

// PlanEntry represents a single plan item
type PlanEntry struct {
	Description string `json:"description"`
	Status      string `json:"status"` // "pending", "in_progress", "completed", "failed"
}

// CommandApprovalResponse for responding to command execution approval requests
// Decision values: "accept", "acceptForSession", "decline", "cancel"
type CommandApprovalResponse struct {
	Decision string `json:"decision"`
}

// FileChangeApprovalResponse for responding to file change approval requests
// Decision values: "accept", "acceptForSession", "decline", "cancel"
type FileChangeApprovalResponse struct {
	Decision string `json:"decision"`
}

// ErrorParams for error notification
type ErrorParams struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// TokenCountParams for token_count notification
// This is emitted by Codex after each turn to report token usage.
type TokenCountParams struct {
	Info       *TokenUsageInfo    `json:"info,omitempty"`
	RateLimits *RateLimitSnapshot `json:"rateLimits,omitempty"`
}

// TokenUsageInfo contains detailed token usage information.
type TokenUsageInfo struct {
	TotalTokenUsage    *TokenUsage `json:"totalTokenUsage,omitempty"`
	LastTokenUsage     *TokenUsage `json:"lastTokenUsage,omitempty"`
	ModelContextWindow *int64      `json:"modelContextWindow,omitempty"`
}

// TokenUsage contains token counts for a request/response cycle.
type TokenUsage struct {
	InputTokens           int32 `json:"inputTokens"`
	CachedInputTokens     int32 `json:"cachedInputTokens"`
	OutputTokens          int32 `json:"outputTokens"`
	ReasoningOutputTokens int32 `json:"reasoningOutputTokens"`
	TotalTokens           int32 `json:"totalTokens"`
}

// ThreadTokenUsageUpdatedParams for thread/tokenUsage/updated notification.
// This is emitted by Codex after each turn completes with token usage info.
type ThreadTokenUsageUpdatedParams struct {
	ThreadID   string            `json:"threadId"`
	TurnID     string            `json:"turnId"`
	TokenUsage *ThreadTokenUsage `json:"tokenUsage"`
}

// ThreadTokenUsage contains the token usage summary for a thread.
type ThreadTokenUsage struct {
	Total              *TokenUsage `json:"total,omitempty"`
	Last               *TokenUsage `json:"last,omitempty"`
	ModelContextWindow int64       `json:"modelContextWindow"`
}

// RateLimitSnapshot contains rate limit information.
type RateLimitSnapshot struct {
	Primary   *RateLimitWindow `json:"primary,omitempty"`
	Secondary *RateLimitWindow `json:"secondary,omitempty"`
	Credits   *CreditsSnapshot `json:"credits,omitempty"`
	PlanType  *string          `json:"planType,omitempty"`
}

// RateLimitWindow contains rate limit window information.
type RateLimitWindow struct {
	UsedPercent        int32  `json:"usedPercent"`
	WindowDurationMins *int64 `json:"windowDurationMins,omitempty"`
	ResetsAt           *int64 `json:"resetsAt,omitempty"`
}

// CreditsSnapshot contains credits information (placeholder for future use).
type CreditsSnapshot struct {
	Remaining *int64 `json:"remaining,omitempty"`
	Used      *int64 `json:"used,omitempty"`
}

// ContextCompactedParams for context_compacted notification.
// Emitted when the context has been compacted due to reaching limits.
type ContextCompactedParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
}
