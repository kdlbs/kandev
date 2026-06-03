// Package instance provides data structures for multi-agent instance management.
// It defines the core types used to represent, create, and serialize agent instances
// that run as separate processes with their own HTTP servers.
package instance

import (
	"net/http"
	"sync/atomic"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/process"
)

// Instance represents a single agent instance running as a subprocess.
// Each instance has its own process manager, HTTP server, and configuration.
type Instance struct {
	// ID is the unique identifier for this instance
	ID string

	// Port is the HTTP port this instance is listening on
	Port int

	// Status is the current status of the instance (e.g., "running", "stopped", "error")
	Status string

	// WorkspacePath is the absolute path to the workspace directory for this instance
	WorkspacePath string

	// AgentCommand is the command used to start the agent subprocess
	AgentCommand string

	// Env contains environment variables passed to the agent process
	Env map[string]string

	// CreatedAt is the timestamp when this instance was created
	CreatedAt time.Time

	// manager is the process manager handling the agent subprocess (unexported)
	manager *process.Manager

	// server is the HTTP server for this instance's API (unexported)
	server *http.Server

	// lastActivityNanos is the unix-nano timestamp of the most recently
	// observed HTTP request on this instance's port. Used by the idle reaper
	// to detect disconnected sessions. Bumped on request entry and exit by
	// the activity middleware installed in instance.Manager.CreateInstance.
	lastActivityNanos atomic.Int64

	// inflightRequests counts HTTP requests currently being served on this
	// instance's port. Long-lived requests (WebSocket upgrades for
	// /agent/stream, /workspace/stream, etc.) keep this above zero for the
	// full duration of the connection, so the idle reaper treats an
	// instance with an open WS as active even when no new HTTP request
	// arrives. Maintained by the activity middleware.
	inflightRequests atomic.Int32
}

// MarkActivity stamps the current time as the most recent activity on this
// instance. Called by the activity middleware on every request entry and exit.
func (i *Instance) MarkActivity() {
	i.lastActivityNanos.Store(time.Now().UnixNano())
}

// LastActivity returns the timestamp of the most recent observed activity on
// this instance. Zero if no activity has been recorded yet.
func (i *Instance) LastActivity() time.Time {
	v := i.lastActivityNanos.Load()
	if v == 0 {
		return time.Time{}
	}
	return time.Unix(0, v)
}

// IsIdle reports whether the instance has been idle for at least `timeout`.
// An instance with any in-flight HTTP request (notably an open WebSocket
// stream) is never considered idle.
func (i *Instance) IsIdle(now time.Time, timeout time.Duration) bool {
	if i.inflightRequests.Load() > 0 {
		return false
	}
	last := i.LastActivity()
	if last.IsZero() {
		// No activity observed since creation — fall back to CreatedAt so a
		// never-touched instance still gets reaped after timeout.
		last = i.CreatedAt
	}
	if last.IsZero() {
		return false
	}
	return now.Sub(last) >= timeout
}

// McpServerConfig holds configuration for an MCP server.
type McpServerConfig struct {
	Name    string            `json:"name"`
	URL     string            `json:"url,omitempty"`
	Type    string            `json:"type,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// CreateRequest contains the parameters for creating a new agent instance.
type CreateRequest struct {
	// ID is an optional identifier for the instance. If empty, one will be generated.
	ID string `json:"id,omitempty"`

	// WorkspacePath is the required absolute path to the workspace directory.
	WorkspacePath string `json:"workspace_path"`

	// AgentCommand is an optional command to start the agent. If empty, a default is used.
	AgentCommand string `json:"agent_command,omitempty"`

	// Protocol is the protocol adapter to use (acp, codex, auggie). If empty, default is used.
	Protocol string `json:"protocol,omitempty"`

	// AgentType identifies the agent (e.g., "auggie", "codex", "claude-code").
	// Required for debug file naming. Typically matches the agent ID from the registry.
	AgentType string `json:"agent_type,omitempty"`

	// WorkspaceFlag is the CLI flag for workspace path (e.g., "--workspace-root").
	// If empty, only cwd is used for workspace path.
	WorkspaceFlag string `json:"workspace_flag,omitempty"`

	// Env contains optional environment variables to pass to the agent process.
	Env map[string]string `json:"env,omitempty"`

	// AutoStart indicates whether to start the agent automatically after creation.
	AutoStart bool `json:"auto_start,omitempty"`

	// McpServers is a list of MCP servers to configure for the agent.
	McpServers []McpServerConfig `json:"mcp_servers,omitempty"`

	// SessionID is the task session ID for MCP tool calls (used by ask_user_question).
	SessionID string `json:"session_id,omitempty"`

	// TaskID is the task ID for MCP plan tool calls (server-side injection).
	TaskID string `json:"task_id,omitempty"`

	// DisableAskQuestion disables the ask_user_question MCP tool (for TUI agents).
	DisableAskQuestion bool `json:"disable_ask_question,omitempty"`

	// AssumeMcpSse overrides MCP capability filtering to assume SSE support.
	AssumeMcpSse bool `json:"assume_mcp_sse,omitempty"`

	// AssumeMcpHttp overrides MCP capability filtering to assume HTTP support.
	AssumeMcpHttp bool `json:"assume_mcp_http,omitempty"`

	// McpMode controls which MCP tools are registered: "task" (default) or "config".
	McpMode string `json:"mcp_mode,omitempty"`

	// RequiresProcessKill forces the agent's process group to be killed on
	// shutdown instead of relying on stdin close. Required for agents whose
	// runtime keeps child processes alive when stdin closes (notably
	// opencode acp, which spawns MCP server children that leak otherwise).
	RequiresProcessKill bool `json:"requires_process_kill,omitempty"`
}

// CreateResponse contains the result of creating a new agent instance.
type CreateResponse struct {
	// ID is the unique identifier assigned to the created instance
	ID string `json:"id"`

	// Port is the HTTP port the instance is listening on
	Port int `json:"port"`
}

// InstanceInfo contains serializable information about an instance for API responses.
// It mirrors the exported fields from Instance and is safe for JSON serialization.
type InstanceInfo struct {
	// ID is the unique identifier for this instance
	ID string `json:"id"`

	// Port is the HTTP port this instance is listening on
	Port int `json:"port"`

	// Status is the current status of the instance
	Status string `json:"status"`

	// WorkspacePath is the absolute path to the workspace directory
	WorkspacePath string `json:"workspace_path"`

	// AgentCommand is the command used to start the agent subprocess
	AgentCommand string `json:"agent_command"`

	// Env contains environment variables passed to the agent process
	Env map[string]string `json:"env,omitempty"`

	// CreatedAt is the timestamp when this instance was created
	CreatedAt time.Time `json:"created_at"`
}

// Info returns a safe copy of the instance data for API serialization.
// This method creates an InstanceInfo struct containing only the exported,
// serializable fields from the Instance.
func (i *Instance) Info() *InstanceInfo {
	// Create a copy of the environment map to prevent external modification
	var envCopy map[string]string
	if i.Env != nil {
		envCopy = make(map[string]string, len(i.Env))
		for k, v := range i.Env {
			envCopy[k] = v
		}
	}

	return &InstanceInfo{
		ID:            i.ID,
		Port:          i.Port,
		Status:        i.Status,
		WorkspacePath: i.WorkspacePath,
		AgentCommand:  i.AgentCommand,
		Env:           envCopy,
		CreatedAt:     i.CreatedAt,
	}
}
