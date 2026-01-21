// Package jsonrpc implements JSON-RPC 2.0 protocol for ACP (Agent Client Protocol)
package jsonrpc

import "encoding/json"

// Request represents a JSON-RPC 2.0 request
type Request struct {
	JSONRPC string          `json:"jsonrpc"` // Always "2.0"
	ID      interface{}     `json:"id,omitempty"` // Request ID (int or string), omit for notifications
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response represents a JSON-RPC 2.0 response
type Response struct {
	JSONRPC string          `json:"jsonrpc"` // Always "2.0"
	ID      interface{}     `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Error represents a JSON-RPC 2.0 error
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Notification represents a JSON-RPC 2.0 notification (no ID, no response expected)
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Standard JSON-RPC error codes
const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

// ACP Methods
const (
	// Client -> Agent methods
	MethodInitialize     = "initialize"
	MethodSessionNew     = "session/new"
	MethodSessionPrompt  = "session/prompt"
	MethodSessionLoad    = "session/load"
	MethodSessionCancel  = "session/cancel"
	MethodAuthenticate   = "authenticate"

	// Agent -> Client notifications
	NotificationSessionUpdate = "session/update"

	// Agent -> Client requests (require response)
	MethodRequestPermission = "session/request_permission"
)

// InitializeParams for initialize method
type InitializeParams struct {
	ProtocolVersion int                 `json:"protocolVersion"`
	ClientInfo      ClientInfo          `json:"clientInfo"`
	Capabilities    ClientCapabilities  `json:"capabilities,omitempty"`
}

// ClientInfo identifies the client
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ClientCapabilities describes what the client supports
type ClientCapabilities struct {
	Streaming bool `json:"streaming,omitempty"`
}

// InitializeResult from initialize method
type InitializeResult struct {
	ProtocolVersion int                `json:"protocolVersion"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
	Capabilities    ServerCapabilities `json:"capabilities,omitempty"`
}

// ServerInfo identifies the server (agent)
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerCapabilities describes what the server supports
type ServerCapabilities struct {
	ToolsProvider bool `json:"toolsProvider,omitempty"`
}

// SessionNewParams for session/new method
type SessionNewParams struct {
	Cwd        string      `json:"cwd"`        // Working directory for the session
	McpServers []McpServer `json:"mcpServers"` // MCP servers (required, can be empty array)
}

// McpServer configuration for MCP servers
// Supports both stdio (command+args) and remote (url+type) transports
type McpServer struct {
	Name    string   `json:"name"`
	Command string   `json:"command,omitempty"` // For stdio transport
	Args    []string `json:"args,omitempty"`    // For stdio transport
	URL     string   `json:"url,omitempty"`     // For HTTP/SSE transport
	Type    string   `json:"type,omitempty"`    // "sse" or "http" for remote transport
}

// SessionNewResult from session/new method
type SessionNewResult struct {
	SessionID string `json:"sessionId"`
}

// ContentBlock represents a content block in ACP protocol
// The prompt field in session/prompt is an array of ContentBlock
type ContentBlock struct {
	Type string `json:"type"` // "text", "resource", "image", etc.
	Text string `json:"text,omitempty"` // For type="text"
	// Resource *ResourceContent `json:"resource,omitempty"` // For type="resource" (not implemented yet)
}

// SessionPromptParams for session/prompt method
// According to ACP protocol, prompt is an array of ContentBlock, not a string
type SessionPromptParams struct {
	SessionID string         `json:"sessionId"` // Session ID from session/new
	Prompt    []ContentBlock `json:"prompt"`    // Array of content blocks
}

// SessionPromptResult from session/prompt method
type SessionPromptResult struct {
	// Result is empty, updates come via notifications
}

// SessionLoadParams for session/load method (resume session)
type SessionLoadParams struct {
	SessionID string `json:"sessionId"`
}

// SessionLoadResult from session/load method
type SessionLoadResult struct {
	SessionID string `json:"sessionId"`
	Restored  bool   `json:"restored"`
}

// SessionCancelParams for session/cancel notification
type SessionCancelParams struct {
	Reason string `json:"reason,omitempty"`
}

// SessionUpdate notification from agent
type SessionUpdate struct {
	Type string          `json:"type"` // content, toolCall, thinking, error, complete
	Data json.RawMessage `json:"data,omitempty"`
}

// SessionUpdateContent for type="content"
type SessionUpdateContent struct {
	Text string `json:"text"`
}

// SessionUpdateToolCall for type="toolCall"
type SessionUpdateToolCall struct {
	ToolName string          `json:"toolName"`
	Args     json.RawMessage `json:"args,omitempty"`
	Status   string          `json:"status"` // pending, running, complete, error
	Result   string          `json:"result,omitempty"`
}

// SessionUpdateComplete for type="complete"
type SessionUpdateComplete struct {
	SessionID string `json:"sessionId"`
	Success   bool   `json:"success"`
}

// SessionUpdateInputRequested for type="input_requested"
// Sent by agent when it needs user input to continue
type SessionUpdateInputRequested struct {
	SessionID string `json:"sessionId"`
	Message   string `json:"message"` // The question or prompt for the user
}

// RequestPermissionParams for session/request_permission request from agent
type RequestPermissionParams struct {
	SessionID string                  `json:"sessionId"`
	ToolCall  ToolCallUpdate          `json:"toolCall"`
	Options   []PermissionOption      `json:"options"`
}

// ToolCallUpdate contains tool call information in permission requests
type ToolCallUpdate struct {
	ToolCallID string `json:"toolCallId"`
	Title      string `json:"title,omitempty"`
}

// PermissionOption represents a permission choice
type PermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"` // allow_once, allow_always, reject_once, reject_always
}

// RequestPermissionResult is the response to session/request_permission
type RequestPermissionResult struct {
	Outcome PermissionOutcome `json:"outcome"`
}

// PermissionOutcome represents the user's decision
type PermissionOutcome struct {
	Outcome  string `json:"outcome"`  // "selected" or "cancelled"
	OptionID string `json:"optionId,omitempty"` // Only present when outcome="selected"
}
