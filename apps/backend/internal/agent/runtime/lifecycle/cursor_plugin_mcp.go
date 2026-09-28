package lifecycle

import (
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

func (m *Manager) discoverCursorPluginMCPServers(
	execution *AgentExecution,
	profileInfo *AgentProfileInfo,
	executorType string,
	strategy mcpconfig.PassthroughMCPStrategy,
) []agentctltypes.McpServer {
	if execution == nil || execution.WorkspacePath == "" || !isCursorMCPAuthStrategy(strategy) {
		return nil
	}
	if !isCursorMCPAuthLocalExecutor(executorType) {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	if runtimeHome, exists := execution.RuntimeEnvironment()["HOME"]; exists && !sameCursorMCPAuthHome(home, runtimeHome) {
		return nil
	}
	if profileInfo == nil || !profileInfo.CursorPluginsMCPEnabled {
		return nil
	}

	discovered, err := mcpconfig.DiscoverCursorPluginMCPServers(filepath.Join(home, ".cursor"))
	if err != nil {
		if m.logger != nil {
			m.logger.Warn("could not discover local Cursor plugin MCP servers",
				zap.Error(err),
				zap.String("agent_id", execution.AgentID))
		}
		return nil
	}
	return discovered
}

func mergeDiscoveredPluginServers(
	base []agentctltypes.McpServer,
	discovered []agentctltypes.McpServer,
) []agentctltypes.McpServer {
	if len(discovered) == 0 {
		return base
	}
	existing := make(map[string]struct{}, len(base))
	for _, srv := range base {
		existing[strings.ToLower(srv.Name)] = struct{}{}
	}

	merged := make([]agentctltypes.McpServer, len(base), len(base)+len(discovered))
	copy(merged, base)

	for _, srv := range discovered {
		name := strings.ToLower(srv.Name)
		if name == "" || name == kandevMCPServerName {
			continue
		}
		if _, exists := existing[name]; !exists {
			existing[name] = struct{}{}
			merged = append(merged, srv)
		}
	}
	return merged
}
