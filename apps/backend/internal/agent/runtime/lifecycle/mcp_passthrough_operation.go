package lifecycle

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// startPassthroughWithMCP is the single delivery-evidence owner for local and
// remote agent CLI starts. Adopting an existing process does not call it.
func (m *Manager) startPassthroughWithMCP(ctx context.Context, execution *AgentExecution, runner executionPassthroughRunner, request process.InteractiveStartRequest, operation *preparedMCPOperation) (*process.InteractiveProcessInfo, error) {
	generation := execution.startupAttemptSnapshot()
	info, err := runner.Start(ctx, request)
	if err != nil || info == nil || operation == nil {
		return info, err
	}
	execution.withStartupAttempt(generation, func(startupID string) {
		if current, ok := m.executionStore.GetBySessionID(execution.SessionID); !ok || current != execution {
			return
		}
		attempt := buildPassthroughMCPAttempt(execution, info.ID, operation)
		if m.eventPublisher != nil {
			m.eventPublisher.publishAgentStreamEventWithAttempt(execution, agentctl.AgentEvent{Type: streams.EventTypeMCPAttachment, MCPAttachmentAttempt: &attempt, SessionSettingsSourceGeneration: generation}, startupID)
		}
		if operation.captured {
			m.recordAcceptedMCPSetupOperation(ctx, execution, operation.setup, operation.revision, attempt.AttemptID)
		}
	})
	return info, nil
}

func buildPassthroughMCPAttempt(execution *AgentExecution, processID string, operation *preparedMCPOperation) streams.MCPAttachmentAttempt {
	attemptID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(execution.ID+":"+processID)).String()
	attempt := streams.MCPAttachmentAttempt{AttemptID: attemptID, TaskID: execution.TaskID, SessionID: execution.SessionID, ExecutionID: execution.ID, AgentID: execution.AgentID, AgentProfileID: execution.AgentProfileID, StartedAt: time.Now().UTC()}
	attempt.BuiltinPackages = execution.builtinMCPManifestSnapshot()
	delivered := make(map[string]bool, len(operation.servers))
	history := streams.MCPAttachmentHistory{}
	history.StartAttempt(attempt)
	for _, server := range operation.servers {
		delivered[server.Name] = true
		target := streams.SanitizeMCPStdioTarget(server.Command)
		if server.Type != string(mcpconfig.ServerTypeStdio) {
			target = streams.SanitizeMCPNetworkTarget(server.URL)
		}
		source := streams.MCPServerSourceProfile
		if server.Name == kandevMCPServerName || server.IsInjectedBuiltin() {
			source = streams.MCPServerSourceKandev
		}
		evidence := streams.MCPAttachmentEvidence{AttemptID: attemptID, ServerName: server.Name, Source: source, Transport: server.Type, Target: target, OccurredAt: attempt.StartedAt, DefinitionID: server.DefinitionID, DefinitionRevision: server.DefinitionRevision}
		if server.IsInjectedBuiltin() {
			evidence.BuiltinPackage = server.BuiltinPackage
		}
		for _, origin := range server.Origins {
			evidence.Origins = append(evidence.Origins, streams.MCPSelectionOrigin{Scope: origin.Scope, WorkspaceID: origin.WorkspaceID, OwnerID: origin.OwnerID})
		}
		evidence.Kind = streams.MCPAttachmentEvidenceConfigured
		history.Apply(evidence)
		evidence.Kind = streams.MCPAttachmentEvidenceSessionAccepted
		history.Apply(evidence)
	}
	for _, evidence := range streams.MCPResolutionEvidence(attemptID, operation.metadata, attempt.StartedAt) {
		if !delivered[evidence.ServerName] {
			history.Apply(evidence)
		}
	}
	return history.Current
}

func filterPassthroughMCPServers(operation *preparedMCPOperation, strategy mcpconfig.PassthroughMCPStrategy) {
	switch strategy.(type) {
	case mcpconfig.ClaudeStrategy, *mcpconfig.ClaudeStrategy:
	default:
		return
	}
	filtered := make([]types.McpServer, 0, len(operation.servers))
	for _, server := range operation.servers {
		if server.Name != mcpconfig.ClaudeReservedServerName {
			filtered = append(filtered, server)
			continue
		}
		entry := streams.MCPResolutionEntry{RuntimeName: server.Name, DefinitionID: server.DefinitionID, DefinitionRevision: server.DefinitionRevision, ReasonCode: "server_policy"}
		for _, origin := range server.Origins {
			entry.Origins = append(entry.Origins, streams.MCPSelectionOrigin{Scope: origin.Scope, WorkspaceID: origin.WorkspaceID, OwnerID: origin.OwnerID})
		}
		operation.metadata = append(operation.metadata, entry)
	}
	operation.servers = filtered
}
