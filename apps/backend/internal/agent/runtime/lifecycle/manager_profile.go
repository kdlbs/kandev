package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
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
	if m.mcpResolver != nil && execution.WorkspaceID != "" {
		return m.resolveEffectiveMcpServers(ctx, mcpconfig.ResolutionContext{
			WorkspaceID: execution.WorkspaceID, RepositoryIDs: execution.RepositoryIDs,
			ProfileID: execution.AgentProfileID, TaskID: execution.TaskID,
			SessionID: execution.SessionID,
		}, execution.AgentProfileID, execution.MetadataSnapshot(), agentConfig, execution.RuntimeName)
	}
	return m.resolveMcpServersWithParams(ctx, execution.AgentProfileID, execution.MetadataSnapshot(), agentConfig)
}

func (m *Manager) resolveMcpServersForLaunch(ctx context.Context, req *LaunchRequest, agentConfig agents.Agent) ([]agentctltypes.McpServer, error) {
	if req == nil {
		return nil, nil
	}
	if m.mcpResolver != nil && req.WorkspaceID != "" {
		profileID := executionProfileID(req)
		return m.resolveEffectiveMcpServers(ctx, mcpconfig.ResolutionContext{
			WorkspaceID: req.WorkspaceID, RepositoryIDs: repositoryIDsFromLaunch(req),
			ProfileID: profileID, TaskID: req.TaskID, SessionID: req.SessionID,
		}, profileID, req.Metadata, agentConfig, models.ExecutorType(req.ExecutorType).Runtime())
	}
	return m.resolveMcpServersWithParams(ctx, executionProfileID(req), req.Metadata, agentConfig)
}

func (m *Manager) resolveEffectiveMcpServers(
	ctx context.Context,
	resolutionContext mcpconfig.ResolutionContext,
	profileID string,
	metadata map[string]interface{},
	agentConfig agents.Agent,
	runtime agentruntime.Runtime,
) ([]agentctltypes.McpServer, error) {
	if agentConfig == nil {
		return nil, nil
	}
	if runtime == "" {
		runtime = agentruntime.RuntimeStandalone
	}
	policy := mcpconfig.DefaultPolicyForRuntime(runtime)
	policy, executorID, err := m.applyExecutorMcpPolicy(profileID, "", metadata, policy)
	if err != nil {
		return nil, err
	}
	result, err := m.mcpResolver.Resolve(ctx, resolutionContext, policy)
	if err != nil {
		return nil, err
	}
	for _, decision := range result.Decisions {
		m.logger.Warn("MCP server was not attached",
			zap.String("profile_id", profileID), zap.String("executor_id", executorID),
			zap.String("definition_id", decision.DefinitionID), zap.String("reason_code", decision.ReasonCode))
	}
	resolved := make([]mcpconfig.ResolvedServer, 0, len(result.Servers))
	resolved = append(resolved, result.Servers...)
	return mcpconfig.ToACPServers(resolved), nil
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

// resolveProfileSessionConfigAndPolicy resolves the ACP session config and
// the no-silent-model-fallback policy from a single ResolveProfile call.
// Session start needs both, so a shared lookup avoids two DB reads of the
// same profile per session.
func (m *Manager) resolveProfileSessionConfigAndPolicy(ctx context.Context, profileID string) (string, string, map[string]string, StartModelPolicy) {
	if profileID == "" || m.profileResolver == nil {
		return "", "", nil, StartModelPolicy{}
	}
	info, err := m.profileResolver.ResolveProfile(ctx, profileID)
	if err != nil || info == nil {
		return "", "", nil, StartModelPolicy{}
	}
	return info.Model, info.Mode, info.ConfigOptions, StartModelPolicy{
		Model:         info.Model,
		FallbackModel: info.FallbackModel,
		AutoFallback:  info.AutoFallback,
	}
}

// resolveStartModelPolicy resolves the profile's no-silent-model-fallback
// policy (start model + optional fallback + legacy toggle). Returns a zero
// policy when the profile cannot be resolved, which the policy helper treats
// as "no start model configured".
func (m *Manager) resolveStartModelPolicy(ctx context.Context, profileID string) StartModelPolicy {
	if profileID == "" || m.profileResolver == nil {
		return StartModelPolicy{}
	}
	info, err := m.profileResolver.ResolveProfile(ctx, profileID)
	if err != nil || info == nil {
		return StartModelPolicy{}
	}
	return StartModelPolicy{
		Model:         info.Model,
		FallbackModel: info.FallbackModel,
		AutoFallback:  info.AutoFallback,
	}
}

// initializeACPSession delegates to SessionManager for full ACP session initialization and prompting.
// We pass MarkBootReady (not MarkReady) for the no-prompt branches: dispatchInitialPrompt only
// invokes the callback when there's no taskDescription/attachments to send, which is a *boot*
// signal (the agent has never run a turn). When there IS a prompt, the callback is unused and
// MarkReady fires later from handleCompleteEvent — that path is the true turn-end.
//
// Persisted session runtime state and explicit overrides take precedence over
// profile defaults. This preserves user-selected model and reasoning-effort
// values across process recovery.
func (m *Manager) initializeACPSession(ctx context.Context, execution *AgentExecution, agentConfig agents.Agent, taskDescription string, attachments []MessageAttachment, mcpServers []agentctltypes.McpServer) error {
	profileModel, profileMode, profileConfigOptions, policy := m.resolveProfileSessionConfigAndPolicy(ctx, execution.AgentProfileID)
	runtimeModel, runtimeMode, runtimeConfigOptions := m.sessionRuntimeOverrides(ctx, execution)
	// The effective runtime model (user-selected, persisted session state)
	// takes precedence over the profile's start model for the session; the
	// policy still carries the profile's fallback settings so a gone
	// effective model is handled the same way (InitializeAndPromptWithLayers
	// resolves the effective model and applies the policy).
	return m.sessionManager.InitializeAndPromptWithLayers(
		ctx, execution, agentConfig, taskDescription, attachments, mcpServers, m.MarkBootReady,
		profileModel, profileMode, profileConfigOptions,
		runtimeModel, runtimeMode, runtimeConfigOptions,
		policy,
	)
}

func (m *Manager) sessionRuntimeOverrides(ctx context.Context, execution *AgentExecution) (string, string, map[string]string) {
	info := m.sessionWorkspaceInfo(ctx, execution)
	if info == nil {
		return "", "", nil
	}
	var options map[string]string
	if info.RuntimeConfigOptionsSet {
		options = maps.Clone(info.RuntimeConfigOptions)
	}
	return info.RuntimeModel, info.SessionMode, options
}

func (m *Manager) effectiveSessionRuntimeConfig(ctx context.Context, execution *AgentExecution, profileModel, profileMode string, profileConfigOptions map[string]string) (string, string, map[string]string) {
	model, mode, configOptions, _ := m.effectiveSessionRuntimeConfigWithPresence(
		ctx, execution, profileModel, profileMode, profileConfigOptions,
	)
	return model, mode, configOptions
}

func (m *Manager) effectiveSessionRuntimeConfigWithPresence(
	ctx context.Context,
	execution *AgentExecution,
	profileModel, profileMode string,
	profileConfigOptions map[string]string,
) (string, string, map[string]string, bool) {
	model := profileModel
	mode := profileMode
	configOptions := maps.Clone(profileConfigOptions)
	configOptionsSet := profileConfigOptions != nil
	info := m.sessionWorkspaceInfo(ctx, execution)
	if info == nil {
		return model, mode, configOptions, configOptionsSet
	}
	if info.RuntimeModel != "" {
		model = info.RuntimeModel
	}
	if info.SessionMode != "" {
		mode = info.SessionMode
	}
	if info.RuntimeConfigOptionsSet {
		configOptions = maps.Clone(info.RuntimeConfigOptions)
		if configOptions == nil {
			configOptions = make(map[string]string)
		}
		configOptionsSet = true
	}
	return model, mode, configOptions, configOptionsSet
}

// effectiveSessionMode prefers a session-level permission mode persisted in the
// session metadata (session_mode — declared via the set_session_mode workflow
// action or a user toggle) over the agent profile's default mode. This makes a
// fresh launch start in the declared mode before the first prompt, rather than
// reverting to the profile default. Falls back to profileMode when no provider
// is wired, the lookup fails, or no session mode is set. See issue #1183.
func (m *Manager) effectiveSessionMode(ctx context.Context, execution *AgentExecution, profileMode string) string {
	info := m.sessionWorkspaceInfo(ctx, execution)
	if info == nil || info.SessionMode == "" {
		return profileMode
	}
	return info.SessionMode
}

func (m *Manager) sessionWorkspaceInfo(ctx context.Context, execution *AgentExecution) *WorkspaceInfo {
	if m.workspaceInfoProvider == nil || execution == nil {
		return nil
	}
	info, err := m.workspaceInfoProvider.GetWorkspaceInfoForSession(ctx, execution.TaskID, execution.SessionID)
	if err != nil {
		return nil
	}
	return info
}
