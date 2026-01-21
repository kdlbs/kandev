// Package lifecycle manages agent instance lifecycles including tracking,
// state transitions, and cleanup.
package lifecycle

import (
	"os"
	"strings"

	"github.com/kandev/kandev/internal/agent/registry"
)

// CommandBuilder builds agent commands from registry configuration
type CommandBuilder struct{}

// NewCommandBuilder creates a new CommandBuilder
func NewCommandBuilder() *CommandBuilder {
	return &CommandBuilder{}
}

// CommandOptions contains options for building a command
type CommandOptions struct {
	Model       string // Model to use (appended via ModelFlag if set)
	SessionID   string // Session ID to resume (appended via SessionConfig.ResumeFlag if not ResumeViaACP)
	AutoApprove bool   // If true, skip permission flags (auto-approve all tool calls)
}

// BuildCommand builds a command slice from agent config and options
// Returns the command as a string slice ready for execution
func (cb *CommandBuilder) BuildCommand(agentConfig *registry.AgentTypeConfig, opts CommandOptions) []string {
	// Start with base command from config
	cmd := make([]string, len(agentConfig.Cmd))
	copy(cmd, agentConfig.Cmd)

	// Append model flag if agent supports it and model is specified
	// ModelFlag supports {model} placeholder, e.g. "--model {model}" or "-c model=\"{model}\""
	if agentConfig.ModelFlag != "" && opts.Model != "" {
		expanded := strings.ReplaceAll(agentConfig.ModelFlag, "{model}", opts.Model)
		// Split on first space to separate flag from value (if combined)
		parts := strings.SplitN(expanded, " ", 2)
		cmd = append(cmd, parts...)
	}

	// Append session resume flag if:
	// 1. Session ID is provided
	// 2. Agent does NOT use ACP for session resumption (uses CLI flag instead)
	// 3. Agent has a ResumeFlag configured
	// 4. Agent supports session recovery (CanRecover is not false)
	if opts.SessionID != "" && !agentConfig.SessionConfig.ResumeViaACP && agentConfig.SessionConfig.ResumeFlag != "" && agentConfig.SessionConfig.SupportsRecovery() {
		cmd = append(cmd, agentConfig.SessionConfig.ResumeFlag, opts.SessionID)
	}

	// Add permission flags when AutoApprove is false
	// This makes the agent request permission via ACP for tool calls
	permConfig := agentConfig.PermissionConfig
	if !opts.AutoApprove && permConfig.PermissionFlag != "" && len(permConfig.ToolsRequiringPermission) > 0 {
		for _, tool := range permConfig.ToolsRequiringPermission {
			cmd = append(cmd, permConfig.PermissionFlag, tool+":ask-user")
		}
	}

	return cmd
}

// BuildCommandString builds a command as a single string (for standalone mode)
func (cb *CommandBuilder) BuildCommandString(agentConfig *registry.AgentTypeConfig, opts CommandOptions) string {
	cmd := cb.BuildCommand(agentConfig, opts)
	return strings.Join(cmd, " ")
}

// ExpandSessionDir expands the session directory template from SessionConfig
// Replaces {home} with the user's home directory
// Returns empty string if no session directory is configured
func (cb *CommandBuilder) ExpandSessionDir(agentConfig *registry.AgentTypeConfig) string {
	template := agentConfig.SessionConfig.SessionDirTemplate
	if template == "" {
		return ""
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "/tmp"
	}

	result := strings.ReplaceAll(template, "{home}", homeDir)

	// Ensure the directory exists
	_ = os.MkdirAll(result, 0755)

	return result
}

// GetSessionDirTarget returns the container path for session directory mount
// Returns empty string if no session directory is configured
func (cb *CommandBuilder) GetSessionDirTarget(agentConfig *registry.AgentTypeConfig) string {
	return agentConfig.SessionConfig.SessionDirTarget
}
