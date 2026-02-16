// Package agents defines the Agent interface and supporting types.
// Each agent (Auggie, Claude Code, Codex, etc.) implements this interface
// in its own file, consolidating identity, discovery, models, protocol,
// execution, and runtime configuration in one place.
package agents

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/common/logger"
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

	// --- Protocol ---
	CreateAdapter(cfg *adapter.Config, log *logger.Logger) (adapter.AgentAdapter, error)

	// --- Execution ---
	BuildCommand(opts CommandOptions) Command

	// --- Permissions ---
	PermissionSettings() map[string]PermissionSetting

	// --- Runtime ---
	Runtime() *RuntimeConfig
}

// InferenceAgent is an optional capability for agents that support direct LLM inference.
type InferenceAgent interface {
	GenerateText(ctx context.Context, req InferenceRequest) (*InferenceResponse, error)
}

// PassthroughAgent is an optional capability for agents that support CLI passthrough mode.
type PassthroughAgent interface {
	PassthroughConfig() PassthroughConfig
	BuildPassthroughCommand(opts PassthroughOptions) Command
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
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Provider      string `json:"provider"`
	ContextWindow int    `json:"context_window"` // 0 = unspecified
	IsDefault     bool   `json:"is_default"`
	Source        string `json:"source,omitempty"` // "static" or "dynamic"
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
	Model            string
	SessionID        string // for --resume flag
	AutoApprove      bool
	PermissionValues map[string]bool // e.g. {"allow_indexing": true}
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
	Capabilities   []string
	SessionConfig  SessionConfig
	Protocol       agent.Protocol
	ModelFlag      Param  // e.g. NewParam("--model", "{model}")
	WorkspaceFlag  string // e.g. "--workspace-root"
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
	NativeSessionResume bool
	ResumeFlag          Param
	CanRecover          *bool
	SessionDirTemplate  string
	SessionDirTarget    string
	ForkSessionCmd      Command
	ContinueSessionCmd  Command
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

// DefaultCapabilities is the standard set of capabilities shared by most agents.
var DefaultCapabilities = []string{
	"code_generation", "code_review", "refactoring", "testing", "shell_execution",
}

// DefaultResourceLimits is the standard resource limit set shared by most agents.
var DefaultResourceLimits = ResourceLimits{
	MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour,
}

// InferenceRequest for direct model inference.
type InferenceRequest struct {
	Prompt      string
	Model       string
	MaxTokens   int
	Credentials map[string]string
}

// InferenceResponse from direct model inference.
type InferenceResponse struct {
	Text         string
	Model        string
	TokensUsed   int
	FinishReason string
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
