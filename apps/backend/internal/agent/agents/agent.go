// Package agents defines the Agent interface and supporting types.
// Each agent (Auggie, Claude Code, Codex, etc.) implements this interface
// in its own file, consolidating identity, discovery, models, protocol,
// execution, and runtime configuration in one place.
package agents

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/pkg/agent"
)

// ErrNotSupported is returned when an agent does not support an operation.
var ErrNotSupported = errors.New("not supported by this agent")

// Agent is the core interface for all coding agents.
type Agent interface {
	// --- Identity ---
	ID() string
	Name() string
	DisplayName() string
	Description() string
	Enabled() bool
	DisplayOrder() int // lower = higher priority in listings

	// --- Assets ---
	Logo(variant LogoVariant) []byte // nil if unavailable

	// --- Discovery ---
	IsInstalled(ctx context.Context) (*DiscoveryResult, error)

	// --- Models ---
	DefaultModel() string
	ListModels(ctx context.Context) (*ModelList, error)

	// --- Execution ---
	BuildCommand(opts CommandOptions) Command

	// --- Permissions ---
	PermissionSettings() map[string]PermissionSetting

	// --- Runtime ---
	Runtime() *RuntimeConfig

	// --- Remote Auth ---
	// RemoteAuth returns the auth methods this agent supports in remote environments.
	// Returns nil if the agent has no remote auth configuration.
	RemoteAuth() *RemoteAuth

	// --- Installation ---
	// InstallScript returns shell commands to pre-install the agent CLI in remote environments.
	// Returns empty string if no installation is needed.
	InstallScript() string
}

// InferenceAgent is an optional capability for agents that support one-shot LLM inference.
// Agents implementing this interface can execute single prompts without a persistent session.
type InferenceAgent interface {
	// InferenceConfig returns the configuration for one-shot inference.
	InferenceConfig() *InferenceConfig

	// InferenceModels returns models available for inference (may be a subset of full models).
	InferenceModels() []InferenceModel
}

// PassthroughAgent is an optional capability for agents that support CLI passthrough mode.
type PassthroughAgent interface {
	PassthroughConfig() PassthroughConfig
	BuildPassthroughCommand(opts PassthroughOptions) Command
}

// IsPassthroughOnly returns true if the agent only supports passthrough mode
// and should not have interactive MCP tools (e.g. ask_user_question) registered.
func IsPassthroughOnly(a Agent) bool {
	_, ok := a.(*TUIAgent)
	return ok
}

// LogoVariant selects light or dark logo.
type LogoVariant int

const (
	LogoLight LogoVariant = iota
	LogoDark
)

// Model describes a single model available for an agent.
type Model struct {
	ID            string `json:"id"`
	ACPID         string `json:"acp_id,omitempty"` // ACP protocol model ID (if different from ID)
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Provider      string `json:"provider"`
	ContextWindow int    `json:"context_window"` // 0 = unspecified
	IsDefault     bool   `json:"is_default"`
	Source        string `json:"source,omitempty"` // "static" or "dynamic"
}

// ACPModelID returns the model ID for ACP SetModel. Falls back to ID if ACPID is not set.
func (m Model) ACPModelID() string {
	if m.ACPID != "" {
		return m.ACPID
	}
	return m.ID
}

// ResolveACPModelID finds the ACP model ID for a given profile model ID.
// Searches the model list for a matching ID and returns its ACPID.
// Returns the original ID if no ACPID mapping exists.
func ResolveACPModelID(models []Model, profileModel string) string {
	for _, m := range models {
		if m.ID == profileModel && m.ACPID != "" {
			return m.ACPID
		}
	}
	return profileModel
}

// ToInferenceModel converts a Model to an InferenceModel.
func (m Model) ToInferenceModel() InferenceModel {
	return InferenceModel{
		ID:          m.ID,
		Name:        m.Name,
		Description: m.Description,
		IsDefault:   m.IsDefault,
	}
}

// ModelsToInferenceModels converts a slice of Models to InferenceModels.
func ModelsToInferenceModels(models []Model) []InferenceModel {
	result := make([]InferenceModel, 0, len(models))
	for _, m := range models {
		result = append(result, m.ToInferenceModel())
	}
	return result
}

// ModelList is the result of listing models for an agent.
type ModelList struct {
	Models          []Model
	SupportsDynamic bool // true = UI shows refresh button
}

// DiscoveryResult is the result of checking if an agent is installed.
type DiscoveryResult struct {
	Available         bool
	MatchedPath       string
	SupportsMCP       bool
	MCPConfigPaths    []string
	InstallationPaths []string
	Capabilities      DiscoveryCapabilities
}

// DiscoveryCapabilities describes what the agent supports.
type DiscoveryCapabilities struct {
	SupportsSessionResume bool
	SupportsShell         bool
	SupportsWorkspaceOnly bool
}

// CommandOptions are passed to BuildCommand.
type CommandOptions struct {
	Model               string
	SessionID           string // for --resume flag
	ResumeAtMessageUUID string // for --resume-session-at flag (truncate conversation)
	AutoApprove         bool
	PermissionPolicy    string          // "autonomous", "supervised", "plan"
	PermissionValues    map[string]bool // e.g. {"allow_indexing": true}
	AgentType           string          // for --agent flag (e.g. "task" for subagent)
}

// PassthroughOptions are passed to BuildPassthroughCommand.
type PassthroughOptions struct {
	Model            string
	SessionID        string          // ACP session ID; resumes a specific session via --resume <id>
	Prompt           string          // initial prompt for new sessions
	Resume           bool            // generic "continue last session" (e.g. -c, --resume latest)
	PermissionValues map[string]bool // e.g. {"auto_approve": true}
	WorkDir          string
}

// RuntimeConfig holds Docker / standalone runtime settings.
type RuntimeConfig struct {
	Image          string
	Tag            string
	Cmd            Command
	Entrypoint     Command
	WorkingDir     string
	Env            map[string]string
	RequiredEnv    []string
	Mounts         []MountTemplate
	ResourceLimits ResourceLimits
	SessionConfig  SessionConfig
	Protocol       agent.Protocol
	ModelFlag      Param  // e.g. NewParam("--model", "{model}")
	WorkspaceFlag  string // e.g. "--workspace-root"
	AssumeMcpSse   bool   // Override: assume agent supports SSE MCP servers even if not advertised
}

// MountTemplate defines a mount with template variables.
type MountTemplate struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only"`
}

// ResourceLimits defines resource constraints.
type ResourceLimits struct {
	MemoryMB int64         `json:"memory_mb"`
	CPUCores float64       `json:"cpu_cores"`
	Timeout  time.Duration `json:"timeout"`
}

// SessionConfig defines session resumption behaviour.
type SessionConfig struct {
	NativeSessionResume     bool
	HistoryContextInjection bool // Opt-in: inject conversation history on session resume for agents without native resume
	ResumeFlag              Param
	CanRecover              *bool
	SessionDirTemplate      string
	SessionDirTarget        string
	ForkSessionCmd          Command
	ContinueSessionCmd      Command
}

// SupportsRecovery returns whether the agent supports session recovery.
// Returns true by default if CanRecover is not explicitly set.
func (c SessionConfig) SupportsRecovery() bool {
	if c.CanRecover == nil {
		return true
	}
	return *c.CanRecover
}

// PermissionSetting defines metadata for a permission setting option.
type PermissionSetting struct {
	Supported    bool   `json:"supported"`
	Default      bool   `json:"default"`
	Label        string `json:"label"`
	Description  string `json:"description"`
	ApplyMethod  string `json:"apply_method,omitempty"`
	CLIFlag      string `json:"cli_flag,omitempty"`
	CLIFlagValue string `json:"cli_flag_value,omitempty"`
}

// PassthroughConfig defines configuration for CLI passthrough mode.
type PassthroughConfig struct {
	Supported         bool
	Label             string
	Description       string
	PassthroughCmd    Command
	ModelFlag         Param
	PromptFlag        Param
	PromptPattern     string
	IdleTimeout       time.Duration
	BufferMaxBytes    int64
	StatusDetector    string
	CheckInterval     time.Duration
	StabilityWindow   time.Duration
	ResumeFlag        Param // generic "continue last session" (e.g. NewParam("-c"), NewParam("--resume", "latest"))
	SessionResumeFlag Param // resume a specific session by ID (e.g. NewParam("--resume"))
	WaitForTerminal   bool
}

// DefaultBufferMaxBytes is the default maximum buffer size for passthrough mode (2 MB).
const DefaultBufferMaxBytes int64 = 2 * 1024 * 1024

// DefaultResourceLimits is the standard resource limit set shared by most agents.
var DefaultResourceLimits = ResourceLimits{
	MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour,
}

// InferenceConfig describes how an agent executes one-shot prompts.
type InferenceConfig struct {
	// Supported indicates the agent can do one-shot inference.
	Supported bool
	// Command is the CLI command for one-shot inference (e.g., ["claude", "--print"]).
	// Uses the local CLI directly since agents must be installed/authenticated to work.
	Command Command
	// ModelFlag is the flag template for specifying the model (e.g., ["--model", "{model}"]).
	ModelFlag Param
	// OutputFormat describes how to parse the output: "text", "stream-json".
	OutputFormat string
	// StdinInput if true, prompt is sent via stdin; otherwise as a positional argument.
	StdinInput bool
}

// InferenceModel describes a model available for inference.
type InferenceModel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
}

// RemoteAuth describes all auth methods an agent supports for remote environments.
type RemoteAuth struct {
	Methods []RemoteAuthMethod `json:"methods"`
}

// RemoteAuthMethod describes one way an agent can authenticate in a remote environment.
type RemoteAuthMethod struct {
	// Type is "env" (set env var via secret) or "files" (copy local files to remote).
	Type string `json:"type"`
	// EnvVar is the environment variable name (for type="env").
	EnvVar string `json:"env_var,omitempty"`
	// SetupHint is a UI hint for the user (for type="env").
	SetupHint string `json:"setup_hint,omitempty"`
	// SourceFiles maps OS name to relative paths from home dir (for type="files").
	// Keys: "darwin", "linux", "windows".
	SourceFiles map[string][]string `json:"source_files,omitempty"`
	// TargetRelDir is the target directory relative to the remote user home (for type="files").
	TargetRelDir string `json:"target_rel_dir,omitempty"`
	// Label is a UI label for the file copy option (for type="files").
	Label string `json:"label,omitempty"`
	// SetupScript is an optional shell script that runs on the remote after the
	// env var is resolved. Used to bootstrap credential files from env vars.
	// Only meaningful for type="env". Can reference the env var by name.
	SetupScript string `json:"setup_script,omitempty"`
}

// Command is a domain value type representing a CLI command with arguments.
// Serialize to []string only at system boundaries (process exec, Docker API, JSON DTOs).
type Command struct {
	args []string
}

// NewCommand creates a Command from the given arguments.
func NewCommand(args ...string) Command {
	return Command{args: append([]string{}, args...)}
}

// Args returns the raw string slice for serialization at system boundaries.
func (c Command) Args() []string {
	return c.args
}

// IsEmpty reports whether the command has no arguments.
func (c Command) IsEmpty() bool {
	return len(c.args) == 0
}

// With returns a CmdBuilder seeded with this command's arguments,
// allowing fluent extension without mutating the original.
func (c Command) With() *CmdBuilder {
	return &CmdBuilder{args: append([]string{}, c.args...)}
}

// Param is a command fragment — one or more pre-split CLI arguments
// (flags, flag+value pairs, templates with placeholders).
// Composed into a Command via CmdBuilder methods.
type Param struct {
	args []string
}

// NewParam creates a Param from the given arguments.
func NewParam(args ...string) Param {
	return Param{args: append([]string{}, args...)}
}

// Args returns the raw string slice.
func (p Param) Args() []string { return p.args }

// IsEmpty reports whether the param has no arguments.
func (p Param) IsEmpty() bool { return len(p.args) == 0 }
