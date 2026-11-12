package lifecycle

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// materializeRuntimeProjectMCP writes project-local MCP config for protocol-mode
// agents whose underlying CLI does not consume ACP session/new mcpServers.
func (m *Manager) materializeRuntimeProjectMCP(
	ctx context.Context,
	execution *AgentExecution,
	agentConfig agents.Agent,
	profileInfo *AgentProfileInfo,
	executorType string,
) error {
	return m.materializeRuntimeProjectMCPWithPreparation(ctx, execution, agentConfig, profileInfo, executorType, nil)
}

// materializeRuntimeProjectMCPWithPreparation also produces the execution-bound
// delivery evidence that session admission uses. Evidence is replaced only
// after a successful write and is discarded on every skipped or failed path.
func (m *Manager) materializeRuntimeProjectMCPWithPreparation(
	ctx context.Context,
	execution *AgentExecution,
	agentConfig agents.Agent,
	profileInfo *AgentProfileInfo,
	executorType string,
	progress *prepareProgressRecorder,
	operations ...*preparedMCPOperation,
) error {
	return m.materializeRuntimeProjectMCPWithClient(ctx, execution, agentConfig, profileInfo, executorType, nil, progress, operations...)
}

// materializeRuntimeProjectMCPWithClient prepares project delivery for an
// explicit helper client. Restart paths pass their staged client so evidence is
// bound to the replacement process rather than the one being retired.
func (m *Manager) materializeRuntimeProjectMCPWithClient(
	ctx context.Context,
	execution *AgentExecution,
	agentConfig agents.Agent,
	profileInfo *AgentProfileInfo,
	executorType string,
	client *agentctl.Client,
	progress *prepareProgressRecorder,
	operations ...*preparedMCPOperation,
) error {
	if execution == nil || agentConfig == nil {
		return nil
	}
	rt := agentConfig.Runtime()
	if rt == nil || rt.ProjectMCPStrategy == nil {
		execution.clearProjectMCPDelivery()
		return nil
	}
	if isCursorMCPAuthStrategy(rt.ProjectMCPStrategy) {
		execution.clearProjectMCPDelivery()
		if err := m.prepareCursorMCPAuthWithProgress(execution, profileInfo, executorType, rt.ProjectMCPStrategy, progress); err != nil {
			return err
		}
		return m.reconcileAndMaterializeCursorProjectMCPWithPreparation(ctx, execution, agentConfig, profileInfo, executorType, rt.ProjectMCPStrategy, progress, operations...)
	}
	if err := m.prepareCursorMCPAuth(execution, profileInfo, executorType, rt.ProjectMCPStrategy); err != nil {
		execution.clearProjectMCPDelivery()
		return err
	}
	release := func() {}
	if client == nil {
		client, release = execution.AcquireAgentCtlClient()
	}
	defer release()
	placement, ok, err := m.resolveProjectMCPPlacement(ctx, execution, client, executorType)
	if err != nil {
		execution.clearProjectMCPDelivery()
		return err
	}
	if !ok {
		execution.clearProjectMCPDelivery()
		return nil
	}
	return m.materializeProjectMCPPlacement(ctx, execution, agentConfig, rt.ProjectMCPStrategy, placement, operations...)
}

func (m *Manager) materializeProjectMCPPlacement(
	ctx context.Context,
	execution *AgentExecution,
	agentConfig agents.Agent,
	strategy mcpconfig.PassthroughMCPStrategy,
	placement projectMCPPlacement,
	operations ...*preparedMCPOperation,
) error {
	operation, err := m.resolveMCPOperation(ctx, execution, agentConfig, operations...)
	if err != nil {
		execution.clearProjectMCPDelivery()
		return err
	}
	credential, err := clientBuiltinMCPCredential(ctx, placement.client)
	if err != nil {
		execution.clearProjectMCPDelivery()
		return err
	}
	servers, err := builtinPassthroughMCPServers(execution, placement.coreURL, credential, operation.servers)
	if err != nil {
		execution.clearProjectMCPDelivery()
		return fmt.Errorf("resolve project MCP servers: %w", err)
	}
	if len(servers) == 0 {
		execution.clearProjectMCPDelivery()
		return nil
	}
	artifacts, err := strategy.BuildPassthroughMCP(servers, m.projectMCPPaths(execution, placement))
	if err != nil {
		execution.clearProjectMCPDelivery()
		return fmt.Errorf("build project MCP config: %w", err)
	}
	materialized, err := m.writeProjectMCPFiles(ctx, execution, placement, artifacts.Files)
	if err != nil {
		execution.clearProjectMCPDelivery()
		return err
	}
	if !materialized {
		execution.clearProjectMCPDelivery()
		return nil
	}
	execution.setProjectMCPDelivery(projectMCPDeliveryEvidence(execution, strategy, servers, placement.client, placement.workspace))
	return nil
}

func (m *Manager) resolveMCPOperation(
	ctx context.Context,
	execution *AgentExecution,
	agentConfig agents.Agent,
	operations ...*preparedMCPOperation,
) (*preparedMCPOperation, error) {
	if len(operations) > 0 && operations[0] != nil {
		return operations[0], nil
	}
	return m.prepareExecutionMCP(ctx, execution, agentConfig)
}
