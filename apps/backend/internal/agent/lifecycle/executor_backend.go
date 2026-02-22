// Package lifecycle provides agent runtime abstractions.
package lifecycle

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Runtime abstracts the agent execution environment (Docker, Standalone, K8s, SSH, etc.)
// Each runtime is responsible for creating and managing agentctl instances.
// Agent subprocess launching is handled separately via agentctl client methods.
type ExecutorBackend interface {
	// Name returns the runtime identifier (e.g., "docker", "standalone", "k8s")
	Name() executor.Name

	// HealthCheck verifies the runtime is available and operational
	HealthCheck(ctx context.Context) error

	// CreateInstance creates a new agentctl instance for a task.
	// This starts the agentctl process/container with workspace access (shell, git, files).
	// Agent subprocess is NOT started - use agentctl.Client.ConfigureAgent() + Start().
	CreateInstance(ctx context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error)

	// StopInstance stops an agentctl instance.
	StopInstance(ctx context.Context, instance *ExecutorInstance, force bool) error

	// RecoverInstances discovers and recovers instances that were running before a restart.
	// Returns recovered instances that can be re-tracked by the manager.
	RecoverInstances(ctx context.Context) ([]*ExecutorInstance, error)

	// GetInteractiveRunner returns the interactive runner for passthrough mode.
	// May return nil if the runtime doesn't support passthrough mode.
	GetInteractiveRunner() *process.InteractiveRunner
}

// McpServerConfig holds configuration for an MCP server.
type McpServerConfig struct {
	Name    string   `json:"name"`
	URL     string   `json:"url,omitempty"`
	Type    string   `json:"type,omitempty"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
}

// Metadata keys for runtime-specific configuration
const (
	MetadataKeyMainRepoGitDir = "main_repo_git_dir"
	MetadataKeyWorktreeID     = "worktree_id"
	MetadataKeyWorktreeBranch = "worktree_branch"

	// Remote executor metadata keys
	MetadataKeyRepositoryPath  = "repository_path"
	MetadataKeySetupScript     = "setup_script"
	MetadataKeyCleanupScript   = "cleanup_script"
	MetadataKeyRepoSetupScript = "repository_setup_script"
	MetadataKeyBaseBranch      = "base_branch"
	MetadataKeyIsRemote        = "is_remote"
	MetadataKeyRemoteAuthHome  = "remote_auth_target_home"
	MetadataKeyGitUserName     = "git_user_name"
	MetadataKeyGitUserEmail    = "git_user_email"
	MetadataKeyRemoteReconnect = "remote_reconnect_required"
	MetadataKeyRemoteName      = "remote_reconnect_name"
	MetadataKeyRemoteExecID    = "remote_previous_execution_id"
)

// RemoteStatus describes runtime health/details for remote executors.
// It is intentionally generic so each executor can include extra details in Details.
type RemoteStatus struct {
	RuntimeName   string                 `json:"runtime_name"`
	RemoteName    string                 `json:"remote_name,omitempty"`
	State         string                 `json:"state,omitempty"`
	CreatedAt     *time.Time             `json:"created_at,omitempty"`
	LastCheckedAt time.Time              `json:"last_checked_at"`
	ErrorMessage  string                 `json:"error_message,omitempty"`
	Details       map[string]interface{} `json:"details,omitempty"`
}

// RemoteSessionResumer is an optional capability for remote runtimes that need
// explicit reattachment logic on resume (e.g. reconnect to an existing sprite).
type RemoteSessionResumer interface {
	ResumeRemoteInstance(ctx context.Context, req *ExecutorCreateRequest) error
}

// RemoteStatusProvider is an optional capability for runtimes that can expose
// remote environment status for UX (cloud icon tooltip, degraded state, etc.).
type RemoteStatusProvider interface {
	GetRemoteStatus(ctx context.Context, instance *ExecutorInstance) (*RemoteStatus, error)
}

// ExecutorCreateRequest contains parameters for creating an agentctl instance.
type ExecutorCreateRequest struct {
	InstanceID     string
	TaskID         string
	SessionID      string
	AgentProfileID string
	WorkspacePath  string
	Protocol       string
	Env            map[string]string
	Metadata       map[string]interface{}
	McpServers     []McpServerConfig
	AgentConfig    agents.Agent // Agent type info needed by runtimes

	// OnProgress is an optional callback for streaming preparation progress.
	// Executors that perform multi-step setup (e.g. Sprites, remote Docker) can
	// call this to report real-time progress to the frontend.
	OnProgress PrepareProgressCallback
}

// ExecutorInstance represents an agentctl instance created by a runtime.
// This is returned by the runtime and contains enough info to build an AgentExecution.
type ExecutorInstance struct {
	// Core identifiers
	InstanceID string
	TaskID     string
	SessionID  string

	// Runtime name (e.g., "docker", "standalone") - set by the runtime that created this instance
	RuntimeName string

	// Agentctl client for communicating with this instance
	Client *agentctl.Client

	// Runtime-specific identifiers (only one set is populated)
	ContainerID          string // Docker
	ContainerIP          string // Docker
	StandaloneInstanceID string // Standalone
	StandalonePort       int    // Standalone

	// Common fields
	WorkspacePath string
	Metadata      map[string]interface{}
	StopReason    string
}

// ToAgentExecution converts a ExecutorInstance to an AgentExecution.
func (ri *ExecutorInstance) ToAgentExecution(req *ExecutorCreateRequest) *AgentExecution {
	metadata := req.Metadata
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	// Merge runtime metadata
	for k, v := range ri.Metadata {
		metadata[k] = v
	}

	workspacePath := ri.WorkspacePath
	if workspacePath == "" {
		workspacePath = req.WorkspacePath
	}

	return &AgentExecution{
		ID:                   ri.InstanceID,
		TaskID:               req.TaskID,
		SessionID:            req.SessionID,
		AgentProfileID:       req.AgentProfileID,
		ContainerID:          ri.ContainerID,
		ContainerIP:          ri.ContainerIP,
		WorkspacePath:        workspacePath,
		RuntimeName:          ri.RuntimeName,
		Status:               v1.AgentStatusRunning,
		StartedAt:            time.Now(),
		Metadata:             metadata,
		agentctl:             ri.Client,
		standaloneInstanceID: ri.StandaloneInstanceID,
		standalonePort:       ri.StandalonePort,
		promptDoneCh:         make(chan PromptCompletionSignal, 1),
	}
}
