// Package lifecycle provides agent runtime abstractions.
package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	agentkubernetes "github.com/kandev/kandev/internal/agent/kubernetes"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentruntime"
	commonconfig "github.com/kandev/kandev/internal/common/config"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func remoteContributionsFromMetadata(metadata map[string]interface{}) (map[string]models.RemoteContribution, error) {
	if metadata == nil {
		return nil, nil
	}
	raw, ok := metadata[MetadataKeyRemoteContributions]
	if !ok || raw == nil {
		return nil, nil
	}
	if typed, ok := raw.(map[string]models.RemoteContribution); ok {
		return validateRemoteContributions(typed)
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("encode remote contributions: %w", err)
	}
	var decoded map[string]models.RemoteContribution
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, fmt.Errorf("decode remote contributions: %w", err)
	}
	return validateRemoteContributions(decoded)
}

func validateRemoteContributions(values map[string]models.RemoteContribution) (map[string]models.RemoteContribution, error) {
	if len(values) == 0 {
		return nil, nil
	}
	validated := make(map[string]models.RemoteContribution, len(values))
	for key, binding := range values {
		if err := binding.Validate(); err != nil {
			return nil, fmt.Errorf("validate remote contribution %q: %w", key, err)
		}
		validated[key] = binding
	}
	return validated, nil
}

func contributionDestinationsFromMetadata(metadata map[string]interface{}) (map[string]models.ContributionDestination, error) {
	if metadata == nil {
		return nil, nil
	}
	raw, ok := metadata[MetadataKeyContributionDestinations]
	if !ok || raw == nil {
		return nil, nil
	}
	if typed, ok := raw.(map[string]models.ContributionDestination); ok {
		return validateContributionDestinations(typed)
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("encode contribution destinations: %w", err)
	}
	var decoded map[string]models.ContributionDestination
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, fmt.Errorf("decode contribution destinations: %w", err)
	}
	return validateContributionDestinations(decoded)
}

func validateContributionDestinations(values map[string]models.ContributionDestination) (map[string]models.ContributionDestination, error) {
	if len(values) == 0 {
		return nil, nil
	}
	validated := make(map[string]models.ContributionDestination, len(values))
	for key, destination := range values {
		if err := destination.Validate(); err != nil {
			return nil, fmt.Errorf("validate contribution destination %q: %w", key, err)
		}
		validated[key] = destination
	}
	return validated, nil
}

func comparisonTargetsFromMetadata(metadata map[string]interface{}) (map[string]models.ComparisonTarget, error) {
	if metadata == nil {
		return nil, nil
	}
	raw, ok := metadata[MetadataKeyComparisonTargets]
	if !ok || raw == nil {
		return nil, nil
	}
	if typed, ok := raw.(map[string]models.ComparisonTarget); ok {
		return validateComparisonTargets(typed)
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("encode comparison targets: %w", err)
	}
	var decoded map[string]models.ComparisonTarget
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, fmt.Errorf("decode comparison targets: %w", err)
	}
	return validateComparisonTargets(decoded)
}

func validateComparisonTargets(values map[string]models.ComparisonTarget) (map[string]models.ComparisonTarget, error) {
	if len(values) == 0 {
		return nil, nil
	}
	validated := make(map[string]models.ComparisonTarget, len(values))
	for key, target := range values {
		if err := target.Validate(); err != nil {
			return nil, fmt.Errorf("validate comparison target %q: %w", key, err)
		}
		validated[key] = target
	}
	return validated, nil
}

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
	// MetadataKeyBaseBranches stores a map[string]string (RepositoryName →
	// base branch ref) for per-repo diff-stat resolution inside agentctl.
	// The empty key "" applies to the root / single-repo tracker.
	MetadataKeyBaseBranches             = "base_branches"
	MetadataKeyRemoteContributions      = "remote_contributions"
	MetadataKeyContributionDestinations = "contribution_destinations"
	MetadataKeyComparisonTargets        = "comparison_targets"
	MetadataKeyIsRemote                 = "is_remote"
	MetadataKeyRemoteAuthHome           = "remote_auth_target_home"
	MetadataKeyAgentConfigBundles       = "agent_config_bundles"
	MetadataKeyExecutorProfileID        = "executor_profile_id"
	MetadataKeyGitUserName              = "git_user_name"
	MetadataKeyGitUserEmail             = "git_user_email"
	MetadataKeyImageTagOverride         = "image_tag_override"
	MetadataKeyContainerID              = "container_id"
	MetadataKeySpriteName               = "sprite_name"
	MetadataKeySpriteState              = "sprite_state"
	MetadataKeySpriteCreatedAt          = "sprite_created_at"
	MetadataKeyLocalPort                = "local_port"

	// Kubernetes executor connection metadata. These names mirror the persisted
	// executor config parsed by internal/agent/kubernetes.
	MetadataKeyKubernetesAuthMode              = "auth_mode"
	MetadataKeyKubernetesKubeconfigPath        = "kubeconfig_path"
	MetadataKeyKubernetesKubeContext           = "kube_context"
	MetadataKeyKubernetesConfigNamespace       = "namespace"
	MetadataKeyKubernetesRequestTimeoutSeconds = "request_timeout_seconds"
	MetadataKeyKubernetesProfilePlatform       = "platform"
	MetadataKeyKubernetesProfileMainContainer  = "main_container"
	MetadataKeyKubernetesPodTemplateYAML       = "pod_template_yaml"
	MetadataKeyKubernetesWorkspaceMode         = "workspace.mode"
	MetadataKeyKubernetesWorkspaceSize         = "workspace.size"
	MetadataKeyKubernetesWorkspaceStorageClass = "workspace.storage_class"
	MetadataKeyKubernetesWorkspaceAccessModes  = "workspace.access_modes"
	MetadataKeyKubernetesWorkspaceClaimName    = "workspace.claim_name"

	// Kubernetes runtime inventory. The local forward port is deliberately not
	// represented: every process reconnect must allocate a fresh loopback port.
	MetadataKeyKubernetesNamespace             = "kubernetes_namespace"
	MetadataKeyKubernetesPodName               = "kubernetes_pod_name"
	MetadataKeyKubernetesPodUID                = "kubernetes_pod_uid"
	MetadataKeyKubernetesMainContainer         = "kubernetes_main_container"
	MetadataKeyKubernetesPlatform              = "kubernetes_platform"
	MetadataKeyKubernetesRuntimeWorkspaceMode  = "kubernetes_workspace_mode"
	MetadataKeyKubernetesPVCName               = "kubernetes_pvc_name"
	MetadataKeyKubernetesPVCUID                = "kubernetes_pvc_uid"
	MetadataKeyKubernetesPVCCreated            = "kubernetes_pvc_created"
	MetadataKeyKubernetesAgentctlRemotePort    = "kubernetes_agentctl_remote_port"
	MetadataKeyKubernetesAgentctlInstanceID    = "kubernetes_agentctl_instance_id"
	MetadataKeyKubernetesContainerRestartCount = "kubernetes_main_container_restart_count"
	MetadataKeyKubernetesResourceInstanceID    = agentkubernetes.MetadataKeyResourceInstanceID
	MetadataKeyKubernetesResourceExecutorID    = agentkubernetes.MetadataKeyResourceExecutorID
	MetadataKeyKubernetesResourceProfileID     = agentkubernetes.MetadataKeyResourceProfileID
	MetadataKeyKubernetesResourceTaskID        = agentkubernetes.MetadataKeyResourceTaskID
	MetadataKeyKubernetesResourceSessionID     = agentkubernetes.MetadataKeyResourceSessionID
	MetadataKeyKubernetesResourceEnvironmentID = agentkubernetes.MetadataKeyResourceEnvironmentID
	MetadataKeyKubernetesExecutorConfigHash    = "kubernetes_executor_config_hash"
	MetadataKeyKubernetesProfileConfigHash     = "kubernetes_profile_config_hash"
	MetadataKeyKubernetesTemplateHash          = "kubernetes_template_hash"
	MetadataKeyKubernetesProfileSnapshot       = "kubernetes_profile_snapshot"
	// MetadataKeyKubernetesInventoryState records how far Kubernetes admission
	// progressed. Create-returned UIDs are checkpointed before validation so a
	// failed rollback remains exactly recoverable after process restart.
	MetadataKeyKubernetesInventoryState = "kubernetes_inventory_state"

	// MetadataKeyModelOverride holds a user-requested model that overrides the
	// agent profile's configured model on the next launch. Set by SetSessionModel
	// for passthrough sessions, which restart the PTY to apply the new --model.
	MetadataKeyModelOverride = "model_override"

	// Office metadata keys
	MetadataKeySkillManifestJSON = "skill_manifest_json"

	// SSH runtime metadata keys (per-session, except SSHWorkdirRoot which is per-profile).
	MetadataKeySSHHostAlias          = "ssh_host_alias"
	MetadataKeySSHHost               = "ssh_host"
	MetadataKeySSHPort               = "ssh_port"
	MetadataKeySSHUser               = "ssh_user"
	MetadataKeySSHHostFingerprint    = "ssh_host_fingerprint"
	MetadataKeySSHRemoteTaskDir      = "ssh_remote_task_dir"
	MetadataKeySSHRemoteSessionDir   = "ssh_remote_session_dir"
	MetadataKeySSHRemoteAgentctlPort = "ssh_remote_agentctl_port"
	MetadataKeySSHRemoteAgentctlPID  = "ssh_remote_agentctl_pid"
	MetadataKeySSHLocalForwardPort   = "ssh_local_forward_port"
	MetadataKeySSHRemoteAgentctlURL  = "ssh_remote_agentctl_url"
	MetadataKeySSHWorkdirRoot        = "ssh_workdir_root"
	MetadataKeySSHProxyJump          = "ssh_proxy_jump"
	MetadataKeySSHIdentitySource     = "ssh_identity_source"
	MetadataKeySSHIdentityFile       = "ssh_identity_file"
	// MetadataKeySSHShell names the login shell used when running commands
	// over SSH on the remote (probe, agentctl launch, install, setup
	// scripts). Empty / unset falls back to "bash" at runtime — see
	// WrapLoginShell. Stored per-profile so different profiles on the same
	// host can use different shells; flows into req.Metadata via the
	// standard executor-config merge in buildLaunchMetadata.
	MetadataKeySSHShell = "ssh_shell"
	// MetadataKeySSHReclaimTaskDir opts a profile in to removing the remote
	// task directory once its owning task reaches a terminal outcome. Only
	// the exact string "true" enables it; absent, empty, and every other
	// value leave the pre-existing behavior of keeping the directory
	// forever. Default-off is deliberate: existing hosts hold directories
	// created under a documented promise that Kandev would not delete them,
	// and an upgrade must not retroactively break that promise against data
	// on a machine Kandev does not own.
	//
	// This is an *authoritative* profile key (see
	// profileConfigAuthoritativeKeys in internal/orchestrator/executor) —
	// task-supplied metadata can never enable it, because that would let a
	// task arm a destructive remote operation its profile never approved.
	MetadataKeySSHReclaimTaskDir = "ssh_reclaim_task_dir"
)

// persistentMetadataKeys lists metadata keys carried forward from a previous
// ExecutorRunning record when a session is resumed. Keys not listed here
// (e.g., task_description, session_id) are treated as launch-time-only and
// are NOT copied on resume.
var persistentMetadataKeys = map[string]bool{
	// Sprites runtime
	MetadataKeySpriteName:      true,
	MetadataKeySpriteState:     true,
	MetadataKeySpriteCreatedAt: true,
	MetadataKeyLocalPort:       true,

	// SSH runtime
	MetadataKeySSHHost:               true,
	MetadataKeySSHPort:               true,
	MetadataKeySSHUser:               true,
	MetadataKeySSHHostFingerprint:    true,
	MetadataKeySSHRemoteTaskDir:      true,
	MetadataKeySSHRemoteSessionDir:   true,
	MetadataKeySSHRemoteAgentctlPort: true,
	MetadataKeySSHRemoteAgentctlPID:  true,
	MetadataKeySSHLocalForwardPort:   true,
	MetadataKeySSHRemoteAgentctlURL:  true,
	MetadataKeySSHWorkdirRoot:        true,
	MetadataKeySSHProxyJump:          true,
	MetadataKeySSHIdentitySource:     true,
	MetadataKeySSHIdentityFile:       true,
	MetadataKeySSHShell:              true,
	MetadataKeySSHReclaimTaskDir:     true,

	// Kubernetes connection and exact resource inventory.
	MetadataKeyKubernetesAuthMode:              true,
	MetadataKeyKubernetesKubeconfigPath:        true,
	MetadataKeyKubernetesKubeContext:           true,
	MetadataKeyKubernetesConfigNamespace:       true,
	MetadataKeyKubernetesRequestTimeoutSeconds: true,
	MetadataKeyKubernetesNamespace:             true,
	MetadataKeyKubernetesPodName:               true,
	MetadataKeyKubernetesPodUID:                true,
	MetadataKeyKubernetesMainContainer:         true,
	MetadataKeyKubernetesPlatform:              true,
	MetadataKeyKubernetesRuntimeWorkspaceMode:  true,
	MetadataKeyKubernetesPVCName:               true,
	MetadataKeyKubernetesPVCUID:                true,
	MetadataKeyKubernetesPVCCreated:            true,
	MetadataKeyKubernetesAgentctlRemotePort:    true,
	MetadataKeyKubernetesAgentctlInstanceID:    true,
	MetadataKeyKubernetesContainerRestartCount: true,
	MetadataKeyKubernetesResourceInstanceID:    true,
	MetadataKeyKubernetesResourceExecutorID:    true,
	MetadataKeyKubernetesResourceProfileID:     true,
	MetadataKeyKubernetesResourceTaskID:        true,
	MetadataKeyKubernetesResourceSessionID:     true,
	MetadataKeyKubernetesResourceEnvironmentID: true,
	MetadataKeyKubernetesExecutorConfigHash:    true,
	MetadataKeyKubernetesProfileConfigHash:     true,
	MetadataKeyKubernetesTemplateHash:          true,
	MetadataKeyKubernetesProfileSnapshot:       true,
	MetadataKeyKubernetesInventoryState:        true,

	// Executor type marker
	MetadataKeyIsRemote: true,

	// Executor profile / auth config
	MetadataKeyCleanupScript:            true,
	MetadataKeyRepoSetupScript:          true,
	MetadataKeyRemoteAuthHome:           true,
	MetadataKeyAgentConfigBundles:       true,
	MetadataKeyGitUserName:              true,
	MetadataKeyGitUserEmail:             true,
	"remote_credentials":                true,
	"remote_auth_secrets":               true,
	"executor_mcp_policy":               true,
	"sprites_network_policy_rules":      true,
	MetadataKeyExecutorProfileID:        true,
	MetadataKeyImageTagOverride:         true,
	MetadataKeyContainerID:              true,
	MetadataKeyWorktreeBranch:           true,
	MetadataKeyRemoteContributions:      true,
	MetadataKeyContributionDestinations: true,
}

// persistentMetadataPrefixes lists key prefixes that should persist.
// Any key starting with one of these prefixes is carried forward.
var persistentMetadataPrefixes = []string{
	"env_secret_id_", // Secret store UUIDs for profile env vars
}

// sessionScopedMetadataKeys lists metadata keys that point to per-session
// runtime resources (process PIDs, allocated ports, session directories on
// the remote). These keys ARE persisted across a SAME-session resume — that's
// how a backend restart reattaches to a still-running remote agent — but they
// MUST NOT be carried across SIBLING sessions on the same task. If they were,
// the second session would try to attach to the first session's agentctl
// process and end up sharing its ACP session and instance port.
var sessionScopedMetadataKeys = map[string]bool{
	MetadataKeySSHRemoteSessionDir:             true,
	MetadataKeySSHRemoteAgentctlPort:           true,
	MetadataKeySSHRemoteAgentctlPID:            true,
	MetadataKeySSHLocalForwardPort:             true,
	MetadataKeySSHRemoteAgentctlURL:            true,
	MetadataKeyKubernetesAuthMode:              true,
	MetadataKeyKubernetesKubeconfigPath:        true,
	MetadataKeyKubernetesKubeContext:           true,
	MetadataKeyKubernetesConfigNamespace:       true,
	MetadataKeyKubernetesRequestTimeoutSeconds: true,
	MetadataKeyKubernetesNamespace:             true,
	MetadataKeyKubernetesPodName:               true,
	MetadataKeyKubernetesPodUID:                true,
	MetadataKeyKubernetesMainContainer:         true,
	MetadataKeyKubernetesPlatform:              true,
	MetadataKeyKubernetesRuntimeWorkspaceMode:  true,
	MetadataKeyKubernetesPVCName:               true,
	MetadataKeyKubernetesPVCUID:                true,
	MetadataKeyKubernetesPVCCreated:            true,
	MetadataKeyKubernetesAgentctlRemotePort:    true,
	MetadataKeyKubernetesAgentctlInstanceID:    true,
	MetadataKeyKubernetesContainerRestartCount: true,
	MetadataKeyKubernetesResourceInstanceID:    true,
	MetadataKeyKubernetesResourceExecutorID:    true,
	MetadataKeyKubernetesResourceProfileID:     true,
	MetadataKeyKubernetesResourceTaskID:        true,
	MetadataKeyKubernetesResourceSessionID:     true,
	MetadataKeyKubernetesResourceEnvironmentID: true,
	MetadataKeyKubernetesExecutorConfigHash:    true,
	MetadataKeyKubernetesProfileConfigHash:     true,
	MetadataKeyKubernetesTemplateHash:          true,
	MetadataKeyKubernetesProfileSnapshot:       true,
	MetadataKeyKubernetesInventoryState:        true,
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

// IsSessionScopedMetadataKey reports whether key references per-session
// runtime resources that must not leak across sibling sessions on the same
// task environment.
func IsSessionScopedMetadataKey(key string) bool {
	return sessionScopedMetadataKeys[key]
}

// sshWorkspaceFallbackKeys are the STABLE SSH executor-config keys projected
// into workspace metadata as a fallback for terminal / workspace-restore when
// no live ExecutorRunning record exists. This is deliberately a connection +
// per-profile allowlist and MUST NOT include the session-scoped runtime keys
// (remote session dir, agentctl port/PID/URL, local forward port) — projecting
// a stale one would make the lifecycle manager try to reattach to a dead remote
// agentctl instance instead of creating a fresh one. It mirrors
// trustedExecutorConfigKeys (the connection-routing set targetFromMetadata
// reads) plus the two per-profile keys the terminal path needs (workdir root,
// login shell). Notably it includes ssh_host_alias so alias-only executors
// (host read from ~/.ssh/config) survive restore.
var sshWorkspaceFallbackKeys = map[string]bool{
	MetadataKeySSHHost:            true,
	MetadataKeySSHHostAlias:       true,
	MetadataKeySSHPort:            true,
	MetadataKeySSHUser:            true,
	MetadataKeySSHHostFingerprint: true,
	MetadataKeySSHIdentitySource:  true,
	MetadataKeySSHIdentityFile:    true,
	MetadataKeySSHProxyJump:       true,
	MetadataKeySSHWorkdirRoot:     true,
	MetadataKeySSHShell:           true,
}

// FilterSSHWorkspaceFallbackConfig returns the subset of a stored SSH executor
// config that is safe to project into workspace metadata as a fallback for the
// terminal / workspace-restore path. Only stable connection + per-profile keys
// are copied (see sshWorkspaceFallbackKeys); session-scoped runtime keys are
// intentionally dropped. Returns nil when nothing matches.
func FilterSSHWorkspaceFallbackConfig(config map[string]string) map[string]interface{} {
	if len(config) == 0 {
		return nil
	}
	filtered := make(map[string]interface{})
	for k, v := range config {
		if sshWorkspaceFallbackKeys[k] {
			filtered[k] = v
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
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
	RuntimeName   agentruntime.Runtime   `json:"runtime_name"`
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

// RemoteInstanceRefresh is a staged replacement for a live remote control
// connection. Commit installs it after lifecycle has durably persisted the
// replacement token/inventory; Abort closes only the staged resources.
type RemoteInstanceRefresh struct {
	Instance               *ExecutorInstance
	AgentConfig            agents.Agent
	McpServers             []McpServerConfig
	AutoApprovePermissions bool
	ProcessRestarted       bool
	// Commit installs the staged runtime connection and invokes publish before
	// closing the prior connection. Callers use publish to atomically expose the
	// matching lifecycle client under their client-replacement write lock.
	Commit func(publish func()) error
	Abort  func()
}

// RemoteInstanceRefresher is an optional capability for a tracked remote whose
// control process can restart independently of the backend process.
type RemoteInstanceRefresher interface {
	RefreshRemoteInstance(ctx context.Context, instance *ExecutorInstance) (*RemoteInstanceRefresh, error)
}

// ExecutorCreateRequest contains parameters for creating an agentctl instance.
type ExecutorCreateRequest struct {
	InstanceID        string
	TaskID            string
	TaskTitle         string
	SessionID         string
	TaskEnvironmentID string // Env this execution belongs to (shared across sessions in same task)
	// WorkspaceReuseRequired means this runtime must attach to the supplied
	// environment handle and must never fall back to provisioning a replacement.
	WorkspaceReuseRequired bool
	AgentProfileID         string
	OfficeAgentProfileID   string
	PromptTurnID           string
	WorkspacePath          string
	WorkspaceSourceRoots   []string
	Protocol               string
	Env                    map[string]string
	// ApprovedSecretEnvKeys contains repository binding keys explicitly
	// approved for SSH forwarding. Other request env keys remain filtered.
	ApprovedSecretEnvKeys  []string
	AutoApprovePermissions bool
	// AutoApprovePermissionsOverride is set when a resolved profile explicitly
	// selected the auto-approve value. Nil preserves agentctl defaults/env fallback.
	AutoApprovePermissionsOverride *bool
	Metadata                       map[string]interface{}
	// RemoteContributions carries validated, credential-free bindings to the
	// runtime/agentctl boundary. Keys use the same workspace subpath convention
	// as BaseBranches; the empty key is the workspace root.
	RemoteContributions      map[string]models.RemoteContribution
	ContributionDestinations map[string]models.ContributionDestination
	ComparisonTargets        map[string]models.ComparisonTarget
	McpServers               []McpServerConfig
	AgentConfig              agents.Agent // Agent type info needed by runtimes
	// ManagedRuntimeVersion is the effective exact version resolved for this
	// launch. Remote executors use it during preflight before agentctl receives
	// the final command.
	ManagedRuntimeVersion string
	PreviousExecutionID   string   // Non-empty when reconnecting to a previous execution
	McpMode               string   // MCP tool mode: "task" (default), "task-title-pending", "config", "office", or "automation"
	McpProviders          []string // Normalized provider capabilities attached to the task
	McpProfile            *mcpprofile.Context
	AuthToken             string // Previously handshaken agentctl token for reconnects
	BootstrapNonce        string // Stored nonce for re-handshake after container restart
	AgentctlStartupConfig commonconfig.AgentctlStartupConfig

	// OnProgress is an optional callback for streaming preparation progress.
	// Executors that perform multi-step setup (e.g. Sprites, remote Docker) can
	// call this to report real-time progress to the frontend.
	OnProgress PrepareProgressCallback

	// CheckpointRuntimeInventory persists exact, non-secret resource inventory
	// before a Kubernetes launch enters its next fallible phase. It is nil for
	// runtimes and embedded tests that do not support provisional persistence.
	CheckpointRuntimeInventory func(context.Context, map[string]interface{}) error
	// ReleaseRuntimeInventory removes this launch's provisional row after every
	// created resource was rolled back. Implementations must use execution CAS.
	ReleaseRuntimeInventory func(context.Context) error
}

// ExecutorInstance represents an agentctl instance created by a runtime.
// This is returned by the runtime and contains enough info to build an AgentExecution.
type ExecutorInstance struct {
	// Core identifiers
	InstanceID string
	TaskID     string
	SessionID  string

	// Runtime name (e.g., "docker", "standalone") - set by the runtime that created this instance
	RuntimeName agentruntime.Runtime

	// Agentctl client for communicating with this instance
	Client *agentctl.Client

	// Runtime-specific identifiers (only one set is populated)
	ContainerID          string // Docker
	ContainerIP          string // Docker
	StandaloneInstanceID string // Standalone
	StandalonePort       int    // Standalone

	// Common fields
	WorkspacePath   string
	Metadata        map[string]interface{}
	StopReason      string
	AgentStopFailed bool

	// AuthToken is the agentctl auth token retrieved via handshake.
	// Populated by authenticated container/remote executors for encrypted storage in SecretStore.
	// Empty for standalone (launcher-owned token wired via cfg.Agent.StandaloneAuthToken)
	// and Sprites (no agentctl auth).
	AuthToken string

	// BootstrapNonce is the one-time nonce injected into the remote agentctl environment.
	// It is persisted so a restarted container can complete a fresh handshake
	// against the newly started agentctl process.
	BootstrapNonce string

	// ReleaseRuntimeInventory is carried into manager-owned post-create rollback
	// paths. It must run only after exact runtime cleanup succeeds.
	ReleaseRuntimeInventory func(context.Context) error
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
			historyEnabled = rt.SessionConfig.HistoryContextInjection ||
				rt.SessionConfig.NewSessionOnWorkspaceRebind
		}
	}

	execution := &AgentExecution{
		ID:                   ri.InstanceID,
		RunID:                req.Env["KANDEV_RUN_ID"],
		TaskID:               req.TaskID,
		SessionID:            req.SessionID,
		TaskEnvironmentID:    req.TaskEnvironmentID,
		AgentProfileID:       req.AgentProfileID,
		OfficeAgentProfileID: req.OfficeAgentProfileID,
		promptTurnID:         req.PromptTurnID,
		AgentID:              agentID,
		ContainerID:          ri.ContainerID,
		ContainerIP:          ri.ContainerIP,
		WorkspacePath:        workspacePath,
		WorkspaceSourceRoots: append([]string(nil), req.WorkspaceSourceRoots...),
		RuntimeName:          ri.RuntimeName,
		Status:               v1.AgentStatusRunning,
		StartedAt:            time.Now(),
		metadata:             metadata,
		agentctl:             ri.Client,
		standaloneInstanceID: ri.StandaloneInstanceID,
		standalonePort:       ri.StandalonePort,
		historyEnabled:       historyEnabled,
		promptDoneCh:         make(chan PromptCompletionSignal, 1),
	}
	execution.setRuntimeEnvironment(req.Env)
	return execution
}
