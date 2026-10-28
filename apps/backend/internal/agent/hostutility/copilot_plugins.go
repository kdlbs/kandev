package hostutility

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/copilotplugins"
)

// profileCopilotPluginFlags resolves the profile's Copilot plugin selection
// into native `--plugin-dir` tokens for one profile-owned subprocess. Empty
// selections and other providers remain a no-op.
func (m *Manager) profileCopilotPluginFlags(
	ctx context.Context,
	agentType string,
	profileContext ProfileProbeContext,
	workDir string,
	runtimeCommand []string,
) ([]string, error) {
	if agentType != agents.CopilotACPAgentID || len(profileContext.CopilotSelectedPlugins) == 0 {
		return nil, nil
	}
	version, ok := copilotplugins.RuntimeVersionFromCommand(runtimeCommand)
	if !ok {
		return nil, &copilotplugins.Error{Reason: copilotplugins.ReasonUnsupportedLaunch}
	}
	launch := copilotplugins.LaunchContext{
		AgentType:      agentType,
		ExecutorType:   "",
		Env:            profileContext.Env,
		CLIFlags:       profileContext.CLIFlags,
		CommandPrefix:  profileContext.CommandPrefix,
		RuntimeVersion: version,
		RuntimeCommand: runtimeCommand,
	}
	prepared, err := copilotplugins.ResolveLaunchSelection(ctx, m.copilotPluginRunner(), launch, profileContext.CopilotSelectedPlugins)
	if err != nil {
		return nil, err
	}
	return copilotplugins.ApplyToArgsInDir(profileContext.CLIFlags, prepared.Plugins, workDir)
}

func (m *Manager) copilotPluginRunner() copilotplugins.Runner {
	if m.copilotPluginRunnerImpl != nil {
		return m.copilotPluginRunnerImpl
	}
	return copilotplugins.ExecRunner{}
}

func profileCopilotPluginStatus(err error) Status {
	var pluginErr *copilotplugins.Error
	if errors.As(err, &pluginErr) && pluginErr.IsUnsupported() {
		return StatusUnsupported
	}
	return StatusFailed
}

// SetCopilotPluginRunner installs the bounded native inventory runner used by
// profile probes and utility prompts. Tests inject a fake.
func (m *Manager) SetCopilotPluginRunner(runner copilotplugins.Runner) {
	m.copilotPluginRunnerImpl = runner
}
