package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

// ResolveAgentProfile resolves an agent profile ID to profile information.
// This is exposed for external callers (like the orchestrator executor) to get profile info.
// The profile's model is guaranteed to be non-empty as it's validated at creation time.
func (m *Manager) ResolveAgentProfile(ctx context.Context, profileID string) (*AgentProfileInfo, error) {
	if m.profileResolver == nil {
		return nil, fmt.Errorf("profile resolver not configured")
	}
	return m.profileResolver.ResolveProfile(ctx, profileID)
}

// getAgentConfigForExecution retrieves the agent configuration for an execution.
// The execution must have AgentCommand set (which includes the agent type).
func (m *Manager) getAgentConfigForExecution(execution *AgentExecution) (agents.Agent, error) {
	if execution.AgentProfileID == "" {
		return nil, fmt.Errorf("execution %s has no agent profile ID", execution.ID)
	}

	if m.profileResolver == nil {
		return nil, fmt.Errorf("profile resolver not configured")
	}

	profileInfo, err := m.profileResolver.ResolveProfile(context.Background(), execution.AgentProfileID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve profile: %w", err)
	}

	agentTypeName := profileInfo.AgentName
	agentConfig, ok := m.registry.Get(agentTypeName)
	if !ok {
		return nil, fmt.Errorf("agent type not found: %s", agentTypeName)
	}

	return agentConfig, nil
}

// resolveMcpServers centralizes MCP resolution for a session:
// - loads per-agent MCP config,
// - applies executor-scoped transport rules, allow/deny lists, URL rewrites, and env injection,
// - converts to ACP stdio server definitions used during session initialization.
func (m *Manager) resolveMcpServers(ctx context.Context, execution *AgentExecution, agentConfig agents.Agent) ([]agentctltypes.McpServer, error) {
	if execution == nil {
		return nil, nil
	}
	return m.resolveMcpServersWithParams(ctx, execution.AgentProfileID, execution.Metadata, agentConfig)
}

// applyExecutorMcpPolicy reads executor metadata and applies any executor-scoped MCP policy
// override on top of the given default policy. It returns the updated policy and executor ID.
func (m *Manager) applyExecutorMcpPolicy(profileID string, executorID string, metadata map[string]interface{}, policy mcpconfig.Policy) (mcpconfig.Policy, string, error) {
	if metadata == nil {
		return policy, executorID, nil
	}
	if value, ok := metadata["executor_id"].(string); ok {
		executorID = value
	}
	value, ok := metadata["executor_mcp_policy"]
	if !ok {
		return policy, executorID, nil
	}
	updated, policyWarnings, err := mcpconfig.ApplyExecutorPolicy(policy, value)
	if err != nil {
		return policy, executorID, fmt.Errorf("invalid executor MCP policy: %w", err)
	}
	policy = updated
	for _, warning := range policyWarnings {
		m.logger.Warn("mcp policy warning",
			zap.String("profile_id", profileID),
			zap.String("executor_id", executorID),
			zap.String("warning", warning))
	}
	return policy, executorID, nil
}

// resolveMcpServersWithParams resolves MCP servers with explicit parameters.
// This is used by Launch() before the execution object exists.
func (m *Manager) resolveMcpServersWithParams(ctx context.Context, profileID string, metadata map[string]interface{}, agentConfig agents.Agent) ([]agentctltypes.McpServer, error) {
	if m.mcpProvider == nil || agentConfig == nil {
		return nil, nil
	}

	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return nil, nil
	}

	config, err := m.mcpProvider.GetConfigByProfileID(ctx, profileID)
	if err != nil {
		if errors.Is(err, mcpconfig.ErrAgentMcpUnsupported) || errors.Is(err, mcpconfig.ErrAgentProfileNotFound) {
			return nil, nil
		}
		m.logger.Warn("failed to load MCP config",
			zap.String("profile_id", profileID),
			zap.Error(err))
		return nil, err
	}
	if config == nil || !config.Enabled {
		return nil, nil
	}

	// Get default runtime for MCP policy (used before execution exists)
	defaultRT, _ := m.getDefaultExecutorBackend()
	policy := mcpconfig.DefaultPolicyForRuntime(runtimeName(defaultRT))
	executorID := ""

	policy, executorID, err = m.applyExecutorMcpPolicy(profileID, executorID, metadata, policy)
	if err != nil {
		return nil, err
	}

	resolved, warnings, err := mcpconfig.Resolve(config, policy)
	if err != nil {
		return nil, err
	}
	for _, warning := range warnings {
		m.logger.Warn("mcp config warning",
			zap.String("profile_id", profileID),
			zap.String("executor_id", executorID),
			zap.String("warning", warning))
	}

	return mcpconfig.ToACPServers(resolved), nil
}

func runtimeName(rt ExecutorBackend) executor.Name {
	if rt == nil {
		return executor.NameUnknown
	}
	return rt.Name()
}

// resolveProfileSessionConfig resolves the ACP session config on an agent profile.
// Returns empty values if the profile cannot be resolved.
func (m *Manager) resolveProfileSessionConfig(ctx context.Context, profileID string) (string, string, map[string]string) {
	if profileID == "" || m.profileResolver == nil {
		return "", "", nil
	}
	info, err := m.profileResolver.ResolveProfile(ctx, profileID)
	if err != nil || info == nil {
		return "", "", nil
	}
	return info.Model, info.Mode, info.ConfigOptions
}

// initializeACPSession delegates to SessionManager for full ACP session initialization and prompting.
// We pass MarkBootReady (not MarkReady) for the no-prompt branches: dispatchInitialPrompt only
// invokes the callback when there's no taskDescription/attachments to send, which is a *boot*
// signal (the agent has never run a turn). When there IS a prompt, the callback is unused and
// MarkReady fires later from handleCompleteEvent — that path is the true turn-end.
func (m *Manager) initializeACPSession(ctx context.Context, execution *AgentExecution, agentConfig agents.Agent, taskDescription string, attachments []MessageAttachment, mcpServers []agentctltypes.McpServer) error {
	profileModel, profileMode, profileConfigOptions := m.resolveProfileSessionConfig(ctx, execution.AgentProfileID)
	info := m.lookupSessionInfo(ctx, execution)
	model := effectiveSessionValue(info, func(i *WorkspaceInfo) string { return i.SessionModel }, profileModel)
	mode := effectiveSessionValue(info, func(i *WorkspaceInfo) string { return i.SessionMode }, profileMode)
	configOptions := effectiveSessionConfigOptions(info, profileConfigOptions)
	return m.sessionManager.InitializeAndPrompt(ctx, execution, agentConfig, taskDescription, attachments, mcpServers, m.MarkBootReady, model, mode, configOptions)
}

// lookupSessionInfo fetches the per-session overrides (mode, model, config
// options) from the workspace info provider. Returns nil when no provider is
// wired or the lookup fails — callers must fall back to the profile defaults.
func (m *Manager) lookupSessionInfo(ctx context.Context, execution *AgentExecution) *WorkspaceInfo {
	if m.workspaceInfoProvider == nil {
		return nil
	}
	info, err := m.workspaceInfoProvider.GetWorkspaceInfoForSession(ctx, execution.TaskID, execution.SessionID)
	if err != nil {
		return nil
	}
	return info
}

// effectiveSessionValue returns the session-level override for a scalar field
// (model, mode, …) when present, otherwise the profile fallback.
func effectiveSessionValue(info *WorkspaceInfo, pick func(*WorkspaceInfo) string, fallback string) string {
	if info == nil {
		return fallback
	}
	if v := pick(info); v != "" {
		return v
	}
	return fallback
}

// effectiveSessionConfigOptions returns the persisted session config options
// when present, otherwise the profile defaults. The snapshot stores the full
// set of options the agent advertised after the user's last change, so a
// non-empty session map fully replaces the profile defaults (no merge) —
// mirroring the model / mode override semantics. See issue #1183 / #1310.
func effectiveSessionConfigOptions(info *WorkspaceInfo, profileOptions map[string]string) map[string]string {
	if info == nil || len(info.SessionConfigOptions) == 0 {
		return profileOptions
	}
	return info.SessionConfigOptions
}

// effectiveSessionMode prefers a session-level permission mode persisted in the
// session metadata (session_mode — declared via the set_session_mode workflow
// action or a user toggle) over the agent profile's default mode. This makes a
// fresh launch start in the declared mode before the first prompt, rather than
// reverting to the profile default. Falls back to profileMode when no provider
// is wired, the lookup fails, or no session mode is set. See issue #1183.
func (m *Manager) effectiveSessionMode(ctx context.Context, execution *AgentExecution, profileMode string) string {
	return effectiveSessionValue(m.lookupSessionInfo(ctx, execution), func(i *WorkspaceInfo) string { return i.SessionMode }, profileMode)
}
