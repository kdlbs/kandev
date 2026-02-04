// Package registry manages available agent types and their Docker image configurations.
package registry

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

//go:embed agents.json
var agentsFS embed.FS

// agentsConfig is the structure of the agents.json file
type agentsConfig struct {
	Version string             `json:"version"`
	Agents  []*AgentTypeConfig `json:"agents"`
}

// AgentTypeConfig holds configuration for an agent type
type AgentTypeConfig struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	Image          string            `json:"image"`                  // Docker image
	Tag            string            `json:"tag"`                    // Default tag
	Cmd            []string          `json:"cmd,omitempty"`          // Override container command
	Entrypoint     []string          `json:"entrypoint,omitempty"`
	WorkingDir     string            `json:"working_dir"`
	Env            map[string]string `json:"env,omitempty"`          // Default env vars
	RequiredEnv    []string          `json:"required_env"`           // Required env vars (credentials)
	Mounts         []MountTemplate   `json:"mounts,omitempty"`
	ResourceLimits ResourceLimits    `json:"resource_limits"`
	Capabilities   []string          `json:"capabilities"`           // What the agent can do
	Enabled        bool              `json:"enabled"`
	ModelFlag      string            `json:"model_flag,omitempty"`   // CLI flag for model selection (e.g., "--model")
	WorkspaceFlag  string            `json:"workspace_flag,omitempty"` // CLI flag for workspace path (e.g., "--workspace-root"), empty means use cwd only

	// Protocol configuration
	Protocol       agent.Protocol    `json:"protocol,omitempty"`        // Communication protocol: "acp", "rest", "mcp"
	ProtocolConfig map[string]string `json:"protocol_config,omitempty"` // Protocol-specific settings (e.g., base_url for REST)

	// Session resumption configuration
	SessionConfig SessionConfig `json:"session_config,omitempty"` // How to handle session resumption

	// Permission configuration
	PermissionConfig PermissionConfig `json:"permission_config,omitempty"` // How to handle tool permissions

	// Permission settings metadata (what settings are supported for this agent)
	PermissionSettings map[string]PermissionSetting `json:"permission_settings,omitempty"` // Supported permission settings

	// Discovery configuration (for detecting agent installation)
	Discovery DiscoveryConfig `json:"discovery,omitempty"` // Discovery metadata for agent detection

	// Model configuration
	ModelConfig ModelConfig `json:"model_config,omitempty"` // Available models and default model

	// Passthrough configuration (for CLI passthrough mode)
	PassthroughConfig PassthroughConfig `json:"passthrough_config,omitempty"` // CLI passthrough mode settings

	// Display name for UI (shorter than Name)
	DisplayName string `json:"display_name,omitempty"`
}

// SessionConfig defines how session resumption is handled for an agent type
type SessionConfig struct {
	// NativeSessionResume indicates whether session resumption should use the adapter's
	// native session loading mechanism (e.g., LoadSession method). This works across
	// different protocols (ACP, Copilot SDK, Codex, OpenCode, etc.).
	// If false, session resumption is handled via CLI flags (ResumeFlag) or context injection.
	NativeSessionResume bool `json:"native_session_resume"`

	// ResumeFlag is the CLI flag used for session resumption when NativeSessionResume is false.
	// Example: "--resume" for auggie, empty for agents that use native session loading.
	ResumeFlag string `json:"resume_flag,omitempty"`

	// CanRecover indicates whether this agent supports session recovery after a backend restart.
	// When false, sessions for this agent cannot be resumed after the backend restarts.
	// On recovery, a new session will be started instead of trying to resume.
	// Default is true (agent supports recovery).
	CanRecover *bool `json:"can_recover,omitempty"`

	// SessionDirTemplate is a template for the host directory where session data is stored.
	// Supports variables: {home} (user home directory).
	// Example: "{home}/.augment/sessions" for auggie.
	SessionDirTemplate string `json:"session_dir_template,omitempty"`

	// SessionDirTarget is the container path where the session directory is mounted.
	// Example: "/root/.augment/sessions" for auggie.
	SessionDirTarget string `json:"session_dir_target,omitempty"`
}

// SupportsRecovery returns whether the agent supports session recovery after backend restart.
// Returns true by default if CanRecover is not explicitly set.
func (c SessionConfig) SupportsRecovery() bool {
	if c.CanRecover == nil {
		return true
	}
	return *c.CanRecover
}

// PermissionConfig defines how tool permissions are requested for an agent type
type PermissionConfig struct {
	// PermissionFlag is the CLI flag format for requesting tool permissions.
	// Example: "--permission" for auggie (used as "--permission tool:ask-user").
	// If empty, the agent doesn't support CLI-based permission configuration.
	PermissionFlag string `json:"permission_flag,omitempty"`

	// ToolsRequiringPermission lists tools that should require user permission.
	// When AutoApprove is false, these tools will be configured to ask for permission.
	// Example: ["launch-process", "save-file", "str-replace-editor", "remove-files"]
	ToolsRequiringPermission []string `json:"tools_requiring_permission,omitempty"`
}

// PermissionSetting defines metadata for a permission setting option
type PermissionSetting struct {
	// Supported indicates whether this setting is available for the agent
	Supported bool `json:"supported"`

	// Default is the default value for this setting when creating new profiles
	Default bool `json:"default"`

	// Label is the display label for the UI
	Label string `json:"label"`

	// Description explains what the setting does
	Description string `json:"description"`

	// ApplyMethod defines how this setting is applied: "cli_flag", "acp", "config", or empty (none)
	ApplyMethod string `json:"apply_method,omitempty"`

	// CLIFlag is the CLI flag to use when ApplyMethod is "cli_flag"
	// Example: "--allow-indexing" for auggie
	CLIFlag string `json:"cli_flag,omitempty"`

	// CLIFlagValue is an optional value template for the CLI flag
	// If empty and the setting is true, just appends the flag
	// If set, appends "flag value" (e.g., "--approval-policy never")
	CLIFlagValue string `json:"cli_flag_value,omitempty"`
}

// MountTemplate defines a mount with template support
type MountTemplate struct {
	Source   string `json:"source"`   // Can use {workspace}, {task_id}
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only"`
}

// ResourceLimits defines resource constraints
type ResourceLimits struct {
	MemoryMB       int64   `json:"memory_mb"`
	CPUCores       float64 `json:"cpu_cores"`
	TimeoutSeconds int     `json:"timeout_seconds"`
}

// ModelConfig defines the model configuration for an agent
type ModelConfig struct {
	DefaultModel           string       `json:"default_model"`
	AvailableModels        []ModelEntry `json:"available_models"`
	DynamicModelsCmd       []string     `json:"dynamic_models_cmd,omitempty"`
	DynamicModelsTimeoutMs int          `json:"dynamic_models_timeout_ms,omitempty"`
}

// ModelEntry defines a single model available for an agent
type ModelEntry struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Provider      string `json:"provider"`
	ContextWindow int    `json:"context_window"`
	IsDefault     bool   `json:"is_default"`
	Source        string `json:"source,omitempty"` // "static" or "dynamic"
}

// OSPaths defines OS-specific paths for discovery
type OSPaths struct {
	Linux   []string `json:"linux"`
	Windows []string `json:"windows"`
	MacOS   []string `json:"macos"`
}

// DiscoveryCapabilities defines discovery-specific capabilities
type DiscoveryCapabilities struct {
	SupportsSessionResume bool `json:"supports_session_resume"`
	SupportsShell         bool `json:"supports_shell"`
	SupportsWorkspaceOnly bool `json:"supports_workspace_only"`
}

// DiscoveryConfig holds discovery-related configuration for an agent
type DiscoveryConfig struct {
	SupportsMCP            bool                  `json:"supports_mcp"`
	MCPConfigPath          OSPaths               `json:"mcp_config_path"`
	InstallationPath       OSPaths               `json:"installation_path"`
	DiscoveryCapabilities  DiscoveryCapabilities `json:"discovery_capabilities"`
}

// PassthroughConfig defines configuration for CLI passthrough mode
type PassthroughConfig struct {
	// Supported indicates whether passthrough mode is available for this agent
	Supported bool `json:"supported"`

	// Label is the display label for the UI toggle
	Label string `json:"label"`

	// Description explains what passthrough mode does
	Description string `json:"description"`

	// PassthroughCmd is the command to run in passthrough mode (without ACP/protocol flags)
	PassthroughCmd []string `json:"passthrough_cmd,omitempty"`

	// ModelFlag is the CLI flag template for specifying the model, e.g. "--model {model}"
	// The {model} placeholder will be replaced with the model ID from the profile
	ModelFlag string `json:"model_flag,omitempty"`

	// PromptFlag is the CLI flag template for specifying the prompt, e.g. "--prompt {prompt}"
	// The {prompt} placeholder will be replaced with the actual prompt.
	// If not set, the prompt is appended directly at the end of the command.
	PromptFlag string `json:"prompt_flag,omitempty"`

	// PromptPattern is the regex pattern to detect agent prompts (for turn detection)
	PromptPattern string `json:"prompt_pattern,omitempty"`

	// IdleTimeoutMs is the idle timeout in milliseconds for turn detection
	IdleTimeoutMs int `json:"idle_timeout_ms,omitempty"`

	// BufferMaxBytes is the max size of the output ring buffer (default 2MB)
	BufferMaxBytes int64 `json:"buffer_max_bytes,omitempty"`

	// StatusDetector specifies which detector to use for TUI state detection
	// Valid values: "claude_code", "codex", "" (no detection)
	StatusDetector string `json:"status_detector,omitempty"`

	// CheckIntervalMs is how often to check agent state (default 100ms)
	CheckIntervalMs int `json:"check_interval_ms,omitempty"`

	// StabilityWindowMs is how long state must be stable before reporting (default 0)
	// Codex uses 1000ms to prevent false exits from working state
	StabilityWindowMs int `json:"stability_window_ms,omitempty"`

	// ResumeFlag is the CLI flag to continue the last conversation in the workspace.
	// This is used when the backend restarts and user reconnects to an existing session.
	// Examples: "-c" for auggie/claude-code/opencode, "--resume latest" for gemini
	// If empty, the agent doesn't support resume and starts a fresh session.
	ResumeFlag string `json:"resume_flag,omitempty"`

	// WaitForTerminal indicates that the agent requires a terminal to be connected
	// before starting. Some agents (like Codex) query the terminal for cursor position
	// on startup and will fail if no terminal is connected to respond.
	// When true, the process waits for the terminal WebSocket to connect and send
	// dimensions before starting. When false (default), starts immediately.
	WaitForTerminal bool `json:"wait_for_terminal,omitempty"`
}

// Registry manages agent type configurations
type Registry struct {
	agents map[string]*AgentTypeConfig
	mu     sync.RWMutex
	logger *logger.Logger
}

// NewRegistry creates a new agent registry
func NewRegistry(log *logger.Logger) *Registry {
	return &Registry{
		agents: make(map[string]*AgentTypeConfig),
		logger: log,
	}
}

// LoadFromFile loads agent configurations from a JSON file
func (r *Registry) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var configs []*AgentTypeConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, config := range configs {
		if err := ValidateConfig(config); err != nil {
			r.logger.Warn("skipping invalid agent config",
				zap.String("id", config.ID),
				zap.Error(err))
			continue
		}
		r.agents[config.ID] = config
		r.logger.Debug("loaded agent type", zap.String("id", config.ID))
	}

	return nil
}

// LoadDefaults loads default agent configurations
func (r *Registry) LoadDefaults() {
	defaults := DefaultAgents()

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, config := range defaults {
		r.agents[config.ID] = config
		r.logger.Debug("loaded default agent type", zap.String("id", config.ID))
	}
}

// Register adds a new agent type
func (r *Registry) Register(config *AgentTypeConfig) error {
	if err := ValidateConfig(config); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[config.ID]; exists {
		return fmt.Errorf("agent type %q already registered", config.ID)
	}

	r.agents[config.ID] = config
	r.logger.Info("registered agent type", zap.String("id", config.ID))
	return nil
}

// Unregister removes an agent type
func (r *Registry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[id]; !exists {
		return fmt.Errorf("agent type %q not found", id)
	}

	delete(r.agents, id)
	r.logger.Info("unregistered agent type", zap.String("id", id))
	return nil
}

// Get returns an agent type configuration
func (r *Registry) Get(id string) (*AgentTypeConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	config, exists := r.agents[id]
	return config, exists
}

// GetDefault returns the default agent type configuration.
// It tries "auggie" first, then falls back to the first enabled agent.
func (r *Registry) GetDefault() (*AgentTypeConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try auggie first
	if config, exists := r.agents["auggie"]; exists && config.Enabled {
		return config, nil
	}

	// Fall back to first enabled agent
	for _, config := range r.agents {
		if config.Enabled {
			return config, nil
		}
	}

	return nil, fmt.Errorf("no default agent type available")
}

// List returns all registered agent types
func (r *Registry) List() []*AgentTypeConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*AgentTypeConfig, 0, len(r.agents))
	for _, config := range r.agents {
		result = append(result, config)
	}
	return result
}

// ListEnabled returns only enabled agent types
func (r *Registry) ListEnabled() []*AgentTypeConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*AgentTypeConfig, 0, len(r.agents))
	for _, config := range r.agents {
		if config.Enabled {
			result = append(result, config)
		}
	}
	return result
}

// Exists checks if an agent type exists
func (r *Registry) Exists(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.agents[id]
	return exists
}

// ToAPIType converts to API response type
func (c *AgentTypeConfig) ToAPIType() *v1.AgentType {
	return &v1.AgentType{
		ID:          c.ID,
		Name:        c.Name,
		Description: c.Description,
		DockerImage: c.Image,
		DockerTag:   c.Tag,
		DefaultResources: v1.ResourceLimits{
			CPULimit:    fmt.Sprintf("%.1f", c.ResourceLimits.CPUCores),
			MemoryLimit: fmt.Sprintf("%dM", c.ResourceLimits.MemoryMB),
		},
		EnvironmentVars: c.Env,
		Capabilities:    c.Capabilities,
		Enabled:         c.Enabled,
		CreatedAt:       time.Now(), // These would be set properly when persisted
		UpdatedAt:       time.Now(),
	}
}

// ValidateConfig validates an agent type configuration
func ValidateConfig(config *AgentTypeConfig) error {
	if config.ID == "" {
		return fmt.Errorf("agent type ID is required")
	}
	if config.Name == "" {
		return fmt.Errorf("agent type name is required")
	}
	// Standalone agents (like Claude Code) don't need a Docker image - they use Cmd directly
	// Docker-based agents need an image
	if config.Image == "" && len(config.Cmd) == 0 {
		return fmt.Errorf("agent type requires either image (Docker) or cmd (standalone)")
	}
	if config.Image != "" && config.Tag == "" {
		config.Tag = "latest" // Default to latest if not specified
	}
	if config.ResourceLimits.MemoryMB <= 0 {
		return fmt.Errorf("memory limit must be positive")
	}
	if config.ResourceLimits.CPUCores <= 0 {
		return fmt.Errorf("CPU cores must be positive")
	}
	if config.ResourceLimits.TimeoutSeconds <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	return nil
}

// DefaultAgents returns the default agent configurations loaded from agents.json
func DefaultAgents() []*AgentTypeConfig {
	agents, err := loadAgentsFromJSON()
	if err != nil {
		// Fall back to empty list if loading fails (should not happen with embedded file)
		return []*AgentTypeConfig{}
	}
	return agents
}

// loadAgentsFromJSON loads agent configurations from the embedded agents.json file
func loadAgentsFromJSON() ([]*AgentTypeConfig, error) {
	data, err := agentsFS.ReadFile("agents.json")
	if err != nil {
		return nil, fmt.Errorf("read agents config: %w", err)
	}

	var cfg agentsConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse agents config: %w", err)
	}

	// Post-process to convert protocol strings to agent.Protocol type
	for _, agentCfg := range cfg.Agents {
		agentCfg.Protocol = parseProtocol(string(agentCfg.Protocol))
	}

	return cfg.Agents, nil
}

// parseProtocol converts a protocol string to agent.Protocol
func parseProtocol(p string) agent.Protocol {
	switch p {
	case "acp":
		return agent.ProtocolACP
	case "rest":
		return agent.ProtocolREST
	case "mcp":
		return agent.ProtocolMCP
	case "codex":
		return agent.ProtocolCodex
	case "claude-code":
		return agent.ProtocolClaudeCode
	case "opencode":
		return agent.ProtocolOpenCode
	case "copilot":
		return agent.ProtocolCopilot
	case "amp":
		return agent.ProtocolAmp
	default:
		return agent.ProtocolACP // Default to ACP
	}
}

// GetAgentDefinitions returns the raw agent definitions from agents.json for discovery.
// This provides access to discovery metadata that may be needed by the discovery package.
func GetAgentDefinitions() ([]*AgentTypeConfig, error) {
	return loadAgentsFromJSON()
}
