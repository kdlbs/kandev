// Package lifecycle provides agent runtime abstractions.
package lifecycle

import (
	"context"
	"strings"
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

	// RequiresCloneURL reports whether this executor needs a git clone URL
	// instead of a local filesystem path for repository access.
	RequiresCloneURL() bool

	// ShouldApplyPreferredShell reports whether the user's preferred shell
	// should be injected into the agent environment.
	ShouldApplyPreferredShell() bool

	// IsAlwaysResumable reports whether sessions on this executor can be
	// resumed even without an explicit resume token.
	IsAlwaysResumable() bool
}

// McpServerConfig holds configuration for an MCP server.
// Type alias for agentctl.McpServerConfig to avoid conversion boilerplate.
type McpServerConfig = agentctl.McpServerConfig

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
)

// persistentMetadataKeys lists metadata keys carried forward from a previous
// ExecutorRunning record when a session is resumed. Keys not listed here
// (e.g., task_description, session_id) are treated as launch-time-only and
// are NOT copied on resume.
var persistentMetadataKeys = map[string]bool{
	// Sprites runtime
	"sprite_name":       true,
	"sprite_state":      true,
	"sprite_created_at": true,
	"local_port":        true,
	"instance_port":     true,

	// Executor type marker
	MetadataKeyIsRemote: true,

	// Executor profile / auth config
	MetadataKeyCleanupScript:       true,
	MetadataKeyRepoSetupScript:     true,
	MetadataKeyRemoteAuthHome:      true,
	MetadataKeyGitUserName:         true,
	MetadataKeyGitUserEmail:        true,
	"remote_credentials":           true,
	"remote_auth_secrets":          true,
	"executor_mcp_policy":          true,
	"sprites_network_policy_rules": true,
	"executor_profile_id":          true,
}

// persistentMetadataPrefixes lists key prefixes that should persist.
// Any key starting with one of these prefixes is carried forward.
var persistentMetadataPrefixes = []string{
	"env_secret_id_", // Secret store UUIDs for profile env vars
}

// ShouldPersistMetadataKey returns true if the given metadata key should
// be carried forward when resuming a session from an ExecutorRunning record.
func ShouldPersistMetadataKey(key string) bool {
	if persistentMetadataKeys[key] {
		return true
	}
	for _, prefix := range persistentMetadataPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// FilterPersistentMetadata returns a copy of src containing only keys that
// should be carried forward across session resumes. Returns nil if no keys match.
func FilterPersistentMetadata(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	filtered := make(map[string]interface{})
	for k, v := range src {
		if ShouldPersistMetadataKey(k) {
			filtered[k] = v
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

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
	InstanceID          string
	TaskID              string
	SessionID           string
	AgentProfileID      string
	WorkspacePath       string
	Protocol            string
	Env                 map[string]string
	Metadata            map[string]interface{}
	McpServers          []McpServerConfig
	AgentConfig         agents.Agent // Agent type info needed by runtimes
	PreviousExecutionID string       // Non-empty when reconnecting to a previous execution

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

	var historyEnabled bool
	var agentID string
	if req.AgentConfig != nil {
		agentID = req.AgentConfig.ID()
		if rt := req.AgentConfig.Runtime(); rt != nil {
			historyEnabled = rt.SessionConfig.HistoryContextInjection
		}
	}

	return &AgentExecution{
		ID:                   ri.InstanceID,
		TaskID:               req.TaskID,
		SessionID:            req.SessionID,
		AgentProfileID:       req.AgentProfileID,
		AgentID:              agentID,
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
		historyEnabled:       historyEnabled,
		promptDoneCh:         make(chan PromptCompletionSignal, 1),
	}
}
