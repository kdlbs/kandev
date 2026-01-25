// Package constants provides application-wide constants and timeouts.
package constants

import "time"

// Timeouts for various operations.
const (
	// AgentLaunchTimeout is the maximum time to wait for agent launch,
	// including worktree creation and setup script execution.
	AgentLaunchTimeout = 6 * time.Minute

	// SetupScriptTimeout is the maximum time to wait for a setup script to complete.
	SetupScriptTimeout = 5 * time.Minute

	// CleanupScriptTimeout is the maximum time to wait for a cleanup script to complete.
	CleanupScriptTimeout = 5 * time.Minute

	// TaskDeleteTimeout is the maximum time to wait for task deletion,
	// including cleanup scripts and worktree removal.
	TaskDeleteTimeout = 2 * time.Minute

	// PromptTimeout is the maximum time to wait for an agent to complete a prompt.
	// Agent tasks can take a long time (complex code generation, large refactors),
	// so this is set to a generous value.
	PromptTimeout = 60 * time.Minute
)
