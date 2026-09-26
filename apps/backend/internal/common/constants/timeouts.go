// Package constants provides application-wide constants and timeouts.
package constants

import (
	"math"
	"time"
)

// Timeouts for various operations.
const (
	defaultSetupScriptTimeout = 10 * time.Minute
	setupLaunchAllowance      = 5 * time.Minute
	maxSetupScriptTimeout     = time.Duration(math.MaxInt64) - setupLaunchAllowance

	// CleanupScriptTimeout is the maximum time to wait for a cleanup script to complete.
	CleanupScriptTimeout = 5 * time.Minute

	// TaskDeleteTimeout is the maximum time to wait for task deletion,
	// including cleanup scripts and worktree removal.
	TaskDeleteTimeout = 2 * time.Minute

	// SessionNewTimeout is the maximum time for ACP session/new. Session creation
	// can initialize providers, plugins, and MCP servers before the agent responds.
	SessionNewTimeout = 2 * time.Minute

	// SessionLoadTimeout is the maximum time for ACP session/load (resume).
	// Session loading may involve deserializing large conversation histories.
	SessionLoadTimeout = 2 * time.Minute

	// StepHistoryWriteTimeout bounds the ADR 0015 session_step_history audit
	// insert issued after a step transition has already been durably
	// persisted. It runs on context.WithoutCancel so a client disconnect (or
	// turn-end context cancellation) cannot drop the audit row for a
	// transition that already committed.
	StepHistoryWriteTimeout = 5 * time.Second
)

var (
	// SetupScriptTimeout is the maximum time to wait for a setup script to complete.
	SetupScriptTimeout = defaultSetupScriptTimeout

	// AgentLaunchTimeout is the maximum time to wait for agent launch,
	// including worktree creation and setup script execution.
	AgentLaunchTimeout = SetupScriptTimeout + setupLaunchAllowance
)

// ApplyPreparationTimeout applies the resolved typed startup setting to the
// process-wide launch budgets. It is called once after config.Load, before any
// task or agent services are constructed. Keeping the default here preserves
// callers that use the constants package without a backend startup sequence.
func ApplyPreparationTimeout(timeout time.Duration) {
	if timeout <= 0 || timeout > maxSetupScriptTimeout {
		timeout = defaultSetupScriptTimeout
	}
	SetupScriptTimeout = timeout
	AgentLaunchTimeout = timeout + setupLaunchAllowance
}

func resolveSetupScriptTimeout(value string) time.Duration {
	if value == "" {
		return defaultSetupScriptTimeout
	}

	timeout, err := time.ParseDuration(value)
	if err != nil || timeout <= 0 || timeout > maxSetupScriptTimeout {
		return defaultSetupScriptTimeout
	}
	return timeout
}
