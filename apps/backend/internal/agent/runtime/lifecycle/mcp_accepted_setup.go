package lifecycle

import (
	"context"
	"slices"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

// preparedMCPOperation pairs the exact ephemeral delivery configuration with
// its safe resolution projection and accepted-setup revision.
type preparedMCPOperation struct {
	servers  []types.McpServer
	metadata []streams.MCPResolutionEntry
	setup    *mcpconfig.MCPSetupSnapshot
	revision int64
	captured bool
}

type mcpSetupContextKey struct{}
type mcpOperationSetup struct {
	setup    *mcpconfig.MCPSetupSnapshot
	revision int64
	captured bool
}

func prepareMCPResolution(result *mcpconfig.EffectiveMCPResolution) *preparedMCPOperation {
	op := &preparedMCPOperation{servers: mcpconfig.ToACPServers(result.Servers), revision: result.DesiredRevision, captured: result.Setup != nil || result.DesiredRevision != 0}
	if result.Setup != nil {
		op.setup = &mcpconfig.MCPSetupSnapshot{Version: result.Setup.Version, Servers: slices.Clone(result.Setup.Servers)}
	}
	for _, server := range op.servers {
		entry := streams.MCPResolutionEntry{RuntimeName: server.Name, DefinitionID: server.DefinitionID, DefinitionRevision: server.DefinitionRevision}
		for _, origin := range server.Origins {
			entry.Origins = append(entry.Origins, streams.MCPSelectionOrigin{Scope: origin.Scope, WorkspaceID: origin.WorkspaceID, OwnerID: origin.OwnerID})
		}
		op.metadata = append(op.metadata, entry)
	}
	for _, decision := range result.Decisions {
		entry := streams.MCPResolutionEntry{RuntimeName: decision.RuntimeName, DefinitionID: decision.DefinitionID, DefinitionRevision: decision.DefinitionRevision, ReasonCode: decision.ReasonCode}
		for _, origin := range decision.Origins {
			entry.Origins = append(entry.Origins, streams.MCPSelectionOrigin{Scope: string(origin.Scope), WorkspaceID: origin.WorkspaceID, OwnerID: origin.OwnerID})
		}
		op.metadata = append(op.metadata, entry)
	}
	op.metadata = streams.NormalizeMCPResolution(op.metadata)
	return op
}

func (m *Manager) prepareExecutionMCP(ctx context.Context, execution *AgentExecution, agentConfig agents.Agent) (*preparedMCPOperation, error) {
	if execution == nil {
		return &preparedMCPOperation{}, nil
	}
	if m.mcpResolver != nil && execution.WorkspaceID != "" {
		result, err := m.resolveEffectiveMCPResolution(ctx, mcpconfig.ResolutionContext{WorkspaceID: execution.WorkspaceID, RepositoryIDs: execution.RepositoryIDs, ProfileID: execution.AgentProfileID, TaskID: execution.TaskID, SessionID: execution.SessionID}, execution.AgentProfileID, execution.MetadataSnapshot(), agentConfig, execution.RuntimeName)
		if err != nil {
			return nil, err
		}
		return prepareMCPResolution(result), nil
	}
	servers, err := m.resolveMcpServersWithParams(ctx, execution.AgentProfileID, execution.MetadataSnapshot(), agentConfig)
	return &preparedMCPOperation{servers: servers}, err
}

// context carries only safe metadata, never the servers containing secrets.
func (op *preparedMCPOperation) context(ctx context.Context) context.Context {
	ctx = streams.WithMCPResolution(ctx, op.metadata)
	return context.WithValue(ctx, mcpSetupContextKey{}, mcpOperationSetup{setup: op.setup, revision: op.revision, captured: op.captured})
}

func capturedMCPSetup(ctx context.Context) (*mcpconfig.MCPSetupSnapshot, int64, bool) {
	value, _ := ctx.Value(mcpSetupContextKey{}).(mcpOperationSetup)
	return value.setup, value.revision, value.captured
}

func (m *Manager) recordAcceptedMCPSetupOperation(ctx context.Context, execution *AgentExecution, setup *mcpconfig.MCPSetupSnapshot, revision int64, attempt string) {
	if err := m.saveAppliedSessionMCP(ctx, execution.SessionID, revision, attempt, setup); err != nil {
		m.logger.Warn("failed to record accepted MCP setup", zap.String("session_id", execution.SessionID), zap.Error(err))
	}
}

func (m *Manager) configureAndStartAgentWithMCP(ctx context.Context, execution *AgentExecution, agentConfig agents.Agent, operation *preparedMCPOperation) (string, error) {
	if rt := agentConfig.Runtime(); rt == nil || rt.ProjectMCPStrategy == nil {
		return m.configureAndStartAgent(ctx, execution)
	}
	var profile *AgentProfileInfo
	if execution.AgentProfileID != "" && m.profileResolver != nil {
		var err error
		profile, err = m.profileResolver.ResolveProfile(ctx, execution.AgentProfileID)
		if err != nil {
			return "", err
		}
	}
	if err := m.materializeRuntimeProjectMCPWithPreparation(ctx, execution, agentConfig, profile, execution.ExecutorType, nil, operation); err != nil {
		return "", err
	}
	return m.configureAndStartAgent(ctx, execution)
}
