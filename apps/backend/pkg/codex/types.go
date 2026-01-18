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

// SandboxPolicy configures sandbox behavior
type SandboxPolicy struct {
	Type          string   `json:"type"` // "workspaceWrite", "readOnly", "dangerFullAccess", "externalSandbox"
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
	Type   string `json:"type"`   // "userMessage", "agentMessage", "commandExecution", "fileChange", "reasoning"
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
