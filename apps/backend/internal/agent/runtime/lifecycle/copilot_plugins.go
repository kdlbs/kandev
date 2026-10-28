package lifecycle

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/copilotplugins"
	agentruntime "github.com/kandev/kandev/internal/agentruntime"
)

// prepareCopilotPluginArgs resolves the profile's stable plugin identities
// against the final child environment and returns the base argv with native
// `--plugin-dir` additions. An empty selection never runs the native CLI.
func (m *Manager) prepareCopilotPluginArgs(
	ctx context.Context,
	execution *AgentExecution,
	env map[string]string,
	baseArgs []string,
) ([]string, error) {
	if execution == nil || execution.AgentID != agents.CopilotACPAgentID {
		return baseArgs, nil
	}
	selection, err := m.copilotPluginSelection(ctx, execution)
	if err != nil {
		return nil, err
	}
	if len(selection) == 0 {
		return baseArgs, nil
	}
	if !copilotPluginHostExecutor(execution) {
		return nil, &copilotplugins.Error{Reason: copilotplugins.ReasonUnsupportedLaunch}
	}
	version, ok := copilotplugins.RuntimeVersionFromCommand(baseArgs)
	if !ok {
		return nil, &copilotplugins.Error{Reason: copilotplugins.ReasonUnsupportedLaunch}
	}
	spec := agents.NewCopilotACP().ManagedNPMRuntime()
	launch := copilotplugins.LaunchContext{
		AgentType:      agents.CopilotACPAgentID,
		ExecutorType:   execution.ExecutorType,
		Env:            env,
		WorkDir:        execution.WorkspacePath,
		CLIFlags:       baseArgs,
		CommandPrefix:  execution.LaunchWrapper.Argv,
		RuntimeVersion: version,
		RuntimeCommand: spec.InteractiveCommand(version).Args(),
	}
	prepared, err := copilotplugins.ResolveLaunchSelection(ctx, m.copilotPluginRunner(), launch, selection)
	if err != nil {
		return nil, err
	}
	args, err := copilotplugins.ApplyToArgsInDir(baseArgs, prepared.Plugins, execution.WorkspacePath)
	if err != nil {
		return nil, err
	}
	m.logger.Info("prepared copilot profile plugins",
		zap.String("execution_id", execution.ID),
		zap.Int("selected_plugins", len(selection)),
		zap.Int("mounted_plugins", len(prepared.Plugins)))
	return args, nil
}

func (m *Manager) copilotPluginSelection(ctx context.Context, execution *AgentExecution) ([]string, error) {
	profileID := execution.ExecutionProfileID
	if profileID == "" {
		// Legacy sessions may not carry the concrete profile identity. Agent
		// profile IDs are the only authorized snapshot available in that case.
		profileID = execution.AgentProfileID
	}
	if profileID == "" {
		return append([]string{}, execution.CopilotSelectedPlugins...), nil
	}
	if m.profileResolver == nil {
		return nil, &copilotplugins.Error{Reason: copilotplugins.ReasonMetadataUnavailable}
	}
	// Resolve the profile on every new process. A resume or relaunch after a
	// profile edit must use the saved selection rather than an execution
	// snapshot from an earlier launch.
	profile, err := m.profileResolver.ResolveProfile(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("resolve copilot plugin profile: %w", err)
	}
	if profile == nil {
		return nil, &copilotplugins.Error{Reason: copilotplugins.ReasonMetadataUnavailable}
	}
	execution.CopilotSelectedPlugins = append([]string{}, profile.CopilotSelectedPlugins...)
	return execution.CopilotSelectedPlugins, nil
}

func (m *Manager) copilotPluginRuntimeVersion(ctx context.Context, spec agents.ManagedNPMRuntimeSpec) (string, error) {
	defaultVersion := spec.DefaultVersionOrPinned()
	if m.managedRuntimeSelections == nil {
		return defaultVersion, nil
	}
	selection, found, err := m.managedRuntimeSelections.Get(ctx, agents.CopilotACPAgentID, spec.Package)
	if err != nil {
		return "", fmt.Errorf("resolve selected copilot runtime: %w", err)
	}
	if !found || selection.Package != spec.Package || selection.Version == "" {
		return defaultVersion, nil
	}
	return selection.Version, nil
}

func copilotPluginHostExecutor(execution *AgentExecution) bool {
	if execution == nil || !isHostLocalExecutor(execution.ExecutorType) {
		return false
	}
	// Empty executor metadata is only safe when the runtime itself is the
	// backend-owned standalone process. A container/remote runtime with an
	// unknown executor label must not receive host plugin paths.
	return execution.ExecutorType != "" || execution.RuntimeName == agentruntime.RuntimeStandalone
}

func (m *Manager) copilotPluginRunner() copilotplugins.Runner {
	if m.copilotPlugins != nil {
		return m.copilotPlugins
	}
	return copilotplugins.ExecRunner{}
}

// copilotPassthroughBaseCommand selects the managed interactive CLI for a
// profile with typed plugin selections. Empty selections keep the existing
// unversioned passthrough base for compatibility.
func (m *Manager) copilotPassthroughBaseCommand(
	ctx context.Context,
	agentConfig agents.Agent,
	profile *AgentProfileInfo,
) (agents.Command, error) {
	if agentConfig == nil || agentConfig.ID() != agents.CopilotACPAgentID {
		return agents.Command{}, nil
	}
	if profile == nil || len(profile.CopilotSelectedPlugins) == 0 {
		return agents.Command{}, nil
	}
	spec := agents.NewCopilotACP().ManagedNPMRuntime()
	version, err := m.copilotPluginRuntimeVersion(ctx, spec)
	if err != nil {
		return agents.Command{}, err
	}
	return spec.InteractiveCommand(version), nil
}

// applyPassthroughCopilotPlugins appends native plugin mounts after the
// terminal environment is composed and before the interactive process starts.
func (m *Manager) applyPassthroughCopilotPlugins(
	ctx context.Context,
	execution *AgentExecution,
	profile *AgentProfileInfo,
	cmd *passthroughCommand,
	env map[string]string,
) error {
	if execution == nil || cmd == nil || execution.AgentID != agents.CopilotACPAgentID {
		return nil
	}
	selection := execution.CopilotSelectedPlugins
	if profile != nil {
		selection = profile.CopilotSelectedPlugins
	}
	if len(selection) == 0 {
		return nil
	}
	if !copilotPluginHostExecutor(execution) {
		return &copilotplugins.Error{Reason: copilotplugins.ReasonUnsupportedLaunch}
	}
	version, ok := copilotplugins.RuntimeVersionFromCommand(cmd.Args())
	if !ok {
		return &copilotplugins.Error{Reason: copilotplugins.ReasonUnsupportedLaunch}
	}
	spec := agents.NewCopilotACP().ManagedNPMRuntime()
	launch := copilotplugins.LaunchContext{
		AgentType:      agents.CopilotACPAgentID,
		ExecutorType:   execution.ExecutorType,
		Env:            env,
		WorkDir:        execution.WorkspacePath,
		CLIFlags:       cmd.Args(),
		CommandPrefix:  execution.LaunchWrapper.Argv,
		RuntimeVersion: version,
		RuntimeCommand: spec.InteractiveCommand(version).Args(),
	}
	prepared, err := copilotplugins.ResolveLaunchSelection(ctx, m.copilotPluginRunner(), launch, selection)
	if err != nil {
		return err
	}
	args, err := copilotplugins.ApplyToArgsInDir(cmd.Args(), prepared.Plugins, execution.WorkspacePath)
	if err != nil {
		return err
	}
	cmd.Command = agents.NewCommand(args...)
	return nil
}
