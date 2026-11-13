package hostutility

import (
	"context"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/usage"
	util "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/launchwrapper"
)

func (m *Manager) CandidateExecutionUsageEnabled() bool {
	return m != nil && m.providerUsageLimits
}

// PreviewProfileUsage uses an existing warm native instance only. Neither a
// capability probe nor getInstance/repair/creation is allowed on this path.
func (m *Manager) PreviewProfileUsage(ctx context.Context, profileID string) *usage.ExecutionUsageReport {
	if !m.providerUsageLimits || m.profileResolver == nil {
		return nil
	}
	profile, err := m.profileResolver.Resolve(ctx, profileID)
	if err != nil || profile == nil || profile.ProviderKind != "" {
		return nil
	}
	inst := m.existingUsageInstance(profile.AgentID)
	if inst == nil {
		return nil
	}
	ia, ok := m.registry.GetInferenceAgent(profile.AgentID)
	if !ok {
		return nil
	}
	command, localEnv, release, err := m.acquireInferenceCommand(ctx, inst, ia, agents.Command{})
	if err != nil {
		return nil
	}
	defer release()
	flags, prefix, _, err := util.ProfileInferenceEnvironment(profile.CLIFlags, profile.CommandPrefix, profile.EnvVars)
	if err != nil {
		return nil
	}
	cfg := inferenceConfigForHostUtility(ia)
	if cfg == nil {
		return nil
	}
	env, err := m.resolveProfileEnvironment(ctx, profile.AgentID, profile.EnvVars)
	if err != nil {
		return nil
	}
	env = mergeInferenceEnv(env, localEnv)
	input := &util.UsagePreviewRequest{ExecutionID: inst.instanceID, AgentID: profile.AgentID, Model: m.resolveModel(profile.AgentID, profile.Model, ia), Policy: usage.ExecutionUsagePolicy{ProfileID: profileID, AgentID: profile.AgentID, ExecutorType: "standalone"}, InferenceConfig: &util.InferenceConfigDTO{Protocol: cfg.Protocol, Command: command.Args(), ModelFlag: cfg.ModelFlag.Args(), WorkDir: inst.workDir, Env: env, StripEnv: agents.StripEnvFor(ia), CLIFlags: flags, LaunchWrapper: launchwrapper.Profile(prefix), OperatorDefined: cfg.OperatorDefined}}
	report, err := inst.client.PreviewInferenceUsage(ctx, input)
	if err != nil {
		return nil
	}
	return report
}

func (m *Manager) existingUsageInstance(agentID string) *instance {
	if agentID != "codex-acp" && agentID != "claude-acp" {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	inst := m.instances[agentID]
	if m.stopped || inst == nil || inst.runtimeEpoch != m.runtimeEpoch || inst.client == nil {
		return nil
	}
	return inst
}
