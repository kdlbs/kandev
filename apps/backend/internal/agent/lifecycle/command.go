// Package lifecycle manages agent instance lifecycles including tracking,
// state transitions, and cleanup.
package lifecycle

import (
	"os"
	"strings"

	"github.com/kandev/kandev/internal/agent/agents"
)

// CommandBuilder builds agent commands from agent configuration.
// Delegates to the Agent interface's BuildCommand method.
type CommandBuilder struct{}

// NewCommandBuilder creates a new CommandBuilder
func NewCommandBuilder() *CommandBuilder {
	return &CommandBuilder{}
}

// BuildCommand builds a Command from agent config and options.
// Delegates to the Agent.BuildCommand method.
func (cb *CommandBuilder) BuildCommand(ag agents.Agent, opts agents.CommandOptions) agents.Command {
	return ag.BuildCommand(opts)
}

// BuildCommandString builds a command as a single string (for standalone mode)
func (cb *CommandBuilder) BuildCommandString(ag agents.Agent, opts agents.CommandOptions) string {
	cmd := cb.BuildCommand(ag, opts)
	return strings.Join(cmd.Args(), " ")
}

// BuildContinueCommandString builds the continue command with the same model/permission
// flags as the initial command. Used by one-shot agents (Amp) where each follow-up prompt
// needs a separate "threads continue" command. Returns empty string if the agent has no
// ContinueSessionCmd configured.
func (cb *CommandBuilder) BuildContinueCommandString(ag agents.Agent, opts agents.CommandOptions) string {
	sessionCfg := ag.Runtime().SessionConfig
	if sessionCfg.ContinueSessionCmd.IsEmpty() {
		return ""
	}

	// Start from the continue command base and apply the same flags as BuildCommand
	cmd := sessionCfg.ContinueSessionCmd.With().
		Model(ag.Runtime().ModelFlag, opts.Model).
		Settings(ag.PermissionSettings(), opts.PermissionValues).
		Build()

	return strings.Join(cmd.Args(), " ")
}

// ExpandSessionDir expands the session directory template from SessionConfig.
// Replaces {home} with the user's home directory.
// Returns empty string if no session directory is configured.
func (cb *CommandBuilder) ExpandSessionDir(ag agents.Agent) string {
	template := ag.Runtime().SessionConfig.SessionDirTemplate
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

// GetSessionDirTarget returns the container path for session directory mount.
// Returns empty string if no session directory is configured.
func (cb *CommandBuilder) GetSessionDirTarget(ag agents.Agent) string {
	return ag.Runtime().SessionConfig.SessionDirTarget
}
