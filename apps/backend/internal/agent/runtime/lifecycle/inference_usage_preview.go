package lifecycle

import (
	"context"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/usage"
	util "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/launchwrapper"
)

// PreviewInferenceProfileUsage prepares the same literal profile overrides as
// ExecuteInferenceProfilePrompt, in an existing original session's namespace.
// Unlike actual dispatch it never calls GetOrEnsureExecution or launches a CLI.
func (m *Manager) PreviewInferenceProfileUsage(ctx context.Context, sessionID, profileID string) *usage.ExecutionUsageReport {
	if !m.providerUsageLimits || m.profileResolver == nil || sessionID == "" {
		return nil
	}
	if m.sessionAccessCheck != nil && m.sessionAccessCheck(ctx, sessionID) != nil {
		return nil
	}
	execution, ok := m.GetExecutionBySessionID(sessionID)
	if !ok {
		return nil
	}
	input := m.preparedInferenceUsagePreview(ctx, execution, profileID)
	if input == nil {
		return nil
	}
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if client == nil {
		return nil
	}
	report, err := client.PreviewInferenceUsage(ctx, input)
	if err != nil {
		return nil
	}
	return report
}

func (m *Manager) preparedInferenceUsagePreview(ctx context.Context, execution *AgentExecution, profileID string) *util.UsagePreviewRequest {
	profile, err := m.profileResolver.ResolveProfile(ctx, profileID)
	if err != nil || profile == nil || profile.ProviderKind != "" {
		return nil
	}
	name := profile.AgentName
	if name == "" {
		name = profile.AgentID
	}
	if name != codexACPAgentID && name != claudeACPAgentID {
		return nil
	}
	ia, ok := m.registry.GetInferenceAgent(name)
	if !ok {
		return nil
	}
	cfg := ia.InferenceConfig()
	if cfg == nil || !cfg.Supported {
		return nil
	}
	flags, prefix, _, err := util.ProfileInferenceEnvironment(profile.CLIFlags, profile.CommandPrefix, profile.EnvVars)
	if err != nil {
		return nil
	}
	env, err := m.ResolveUtilityProfileEnvironment(ctx, name, profile.EnvVars)
	if err != nil {
		return nil
	}
	input := &util.UsagePreviewRequest{ExecutionID: execution.ID, AgentID: name, Model: profile.Model, Policy: usage.ExecutionUsagePolicy{ProfileID: profileID, AgentID: name, ExecutorType: execution.ExecutorType}, InferenceConfig: &util.InferenceConfigDTO{Protocol: cfg.Protocol, Command: cfg.Command.Args(), ModelFlag: cfg.ModelFlag.Args(), WorkDir: execution.WorkspacePath, Env: env, StripEnv: agents.StripEnvFor(ia), CLIFlags: flags, LaunchWrapper: launchwrapper.Profile(prefix), OperatorDefined: cfg.OperatorDefined}}
	return input
}
