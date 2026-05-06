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

// BuildCommand builds a Command from agent config and options. Delegates to
// the Agent.BuildCommand method and then appends the user-configured CLI
// flag tokens so every agent participates in the cli_flags feature without
// each BuildCommand needing to remember to opt in.
func (cb *CommandBuilder) BuildCommand(ag agents.Agent, opts agents.CommandOptions) agents.Command {
	cmd := ag.BuildCommand(opts)
	if len(opts.CLIFlagTokens) == 0 {
		return cmd
	}
	return cmd.With().Flag(opts.CLIFlagTokens...).Build()
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
		Flag(opts.CLIFlagTokens...).
		Build()

	return strings.Join(cmd.Args(), " ")
}

// ExpandSessionDir resolves the host-side directory that should be bind-
// mounted into the container at SessionDirTarget. The path is the kandev-
// managed per-container session dir (~/.kandev/agent-sessions/<instance_id>/
// <dotdir>) — isolated from the user's actual ~/<dotdir> so the host's stale
// state DBs and session caches stay out of the container.
//
// Returns empty string if no session directory is configured or if kandev
// home / instance ID are unavailable. Production callers always supply a
// resolved kandev home, so the empty-string path only fires in tests.
func (cb *CommandBuilder) ExpandSessionDir(ag agents.Agent, kandevHomeDir, instanceID string) string {
	template := ag.Runtime().SessionConfig.SessionDirTemplate
	if template == "" {
		return ""
	}
	path := SessionDirHostPath(kandevHomeDir, instanceID, template)
	if path == "" {
		return ""
	}
	_ = os.MkdirAll(path, 0o755)
	return path
}

// GetSessionDirTarget returns the container path for session directory mount.
// Returns empty string if no session directory is configured.
func (cb *CommandBuilder) GetSessionDirTarget(ag agents.Agent) string {
	return ag.Runtime().SessionConfig.SessionDirTarget
}
