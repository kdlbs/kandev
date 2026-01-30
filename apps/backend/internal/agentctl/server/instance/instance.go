// Package instance provides data structures for multi-agent instance management.
// It defines the core types used to represent, create, and serialize agent instances
// that run as separate processes with their own HTTP servers.
package instance

import (
	"net/http"
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
}

// McpServerConfig holds configuration for an MCP server.
type McpServerConfig struct {
	Name    string   `json:"name"`
	URL     string   `json:"url,omitempty"`
	Type    string   `json:"type,omitempty"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
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

	// WorkspaceFlag is the CLI flag for workspace path (e.g., "--workspace-root").
	// If empty, only cwd is used for workspace path.
	WorkspaceFlag string `json:"workspace_flag,omitempty"`

	// Env contains optional environment variables to pass to the agent process.
	Env map[string]string `json:"env,omitempty"`

	// AutoStart indicates whether to start the agent automatically after creation.
	AutoStart bool `json:"auto_start,omitempty"`

	// McpServers is a list of MCP servers to configure for the agent.
	McpServers []McpServerConfig `json:"mcp_servers,omitempty"`

	// BackendWsURL is the WebSocket URL to the Kandev backend for MCP tunneling.
	// If set, the MCP server will be enabled and forward tool calls to the backend.
	BackendWsURL string `json:"backend_ws_url,omitempty"`

	// SessionID is the task session ID for MCP tool calls (used by ask_user_question).
	SessionID string `json:"session_id,omitempty"`
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

