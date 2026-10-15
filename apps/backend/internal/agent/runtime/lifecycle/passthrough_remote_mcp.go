package lifecycle

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/passthroughconfig"
	"go.uber.org/zap"
)

func (m *Manager) applySSHPassthroughMCP(ctx context.Context, execution *AgentExecution, pt agents.PassthroughConfig, agent agents.Agent, profile *AgentProfileInfo) ([]string, error) {
	runner := &remotePassthroughRunner{execution: execution}
	snapshot, err := runner.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	servers, err := m.resolveMcpServersWithParams(ctx, execution.AgentProfileID, execution.MetadataSnapshot(), agent)
	if err != nil {
		return nil, err
	}
	resolved := []types.McpServer{{Name: kandevMCPServerName, Type: string(mcpconfig.ServerTypeHTTP), URL: snapshot.MCPURL}}
	for _, server := range servers {
		if server.Name != kandevMCPServerName {
			resolved = append(resolved, server)
		}
	}
	artifacts, err := pt.MCPStrategy.BuildPassthroughMCP(resolved, mcpconfig.PassthroughPaths{
		TempConfigPath: snapshot.ConfigPath, WorkspaceDir: snapshot.WorkspacePath, Unix: true,
	})
	if err != nil {
		return nil, fmt.Errorf("build remote passthrough MCP config: %w", err)
	}
	files := make([]passthroughconfig.File, 0, len(artifacts.Files))
	for _, file := range artifacts.Files {
		files = append(files, passthroughconfig.File{Path: file.Path, Content: file.Content, MergeKey: file.MergeKey})
	}
	client, release, err := runner.client()
	if err != nil {
		return nil, err
	}
	defer release()
	generation := snapshot.Generation + 1
	if err := client.PreparePassthrough(ctx, types.PassthroughPrepareRequest{Generation: generation, Files: files}); err != nil {
		return nil, err
	}
	execution.remotePassthroughMu.Lock()
	execution.remotePassthrough.preparedGeneration = generation
	execution.remotePassthroughMu.Unlock()
	if err := m.updateRemotePassthroughRecord(ctx, execution, func(record *remotePassthroughRecord) { record.PreparedGeneration = generation }); err != nil {
		return nil, err
	}
	setPassthroughMCPEnv(execution, artifacts.Env)
	return artifacts.Args, nil
}

func (m *Manager) cleanupSSHPassthroughConfig(ctx context.Context, execution *AgentExecution) {
	execution.remotePassthroughMu.Lock()
	if cancel := execution.remotePassthrough.observerCancel; cancel != nil {
		cancel()
	}
	execution.remotePassthrough.observerTarget = types.PassthroughTarget{}
	execution.remotePassthrough.observerCancel = nil
	generation := execution.remotePassthrough.preparedGeneration
	execution.remotePassthroughMu.Unlock()
	if generation == 0 {
		return
	}
	client, release, err := (&remotePassthroughRunner{execution: execution}).client()
	if err != nil {
		return
	}
	defer release()
	if err := client.CleanupPassthrough(ctx, generation); err != nil {
		m.logger.Warn("remote MCP configuration cleanup failed", zap.String("execution_id", execution.ID), zap.Error(err))
	}
}
