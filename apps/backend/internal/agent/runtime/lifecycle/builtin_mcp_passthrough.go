package lifecycle

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"

	agentctlconfig "github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

const builtinCoreMCPPath = "/mcp"

func builtinPassthroughMCPServers(execution *AgentExecution, coreURL string, requested []types.McpServer) ([]types.McpServer, error) {
	var manifest *p.BuiltinManifest
	if value, bound := execution.metadataValue(p.BuiltinBindingMetadataKey); bound {
		profile, err := p.ParseBuiltinBindingMetadata(value)
		if err != nil {
			return nil, err
		}
		manifest = profile.BuiltinPackages
		if manifest.TaskID != execution.TaskID || manifest.WorkspaceID != execution.WorkspaceID {
			return nil, errors.New("built-in MCP binding belongs to another owner")
		}
	}
	host, port, err := builtinPassthroughEndpoint(coreURL)
	if err != nil {
		return nil, err
	}
	native, err := agentctlconfig.BuiltinMCPServers(manifest, host, port)
	if err != nil {
		return nil, err
	}
	servers := make([]types.McpServer, 0, len(native)/2+len(requested))
	for _, server := range native {
		if server.Type == urlSchemeHTTP {
			servers = append(servers, types.McpServer{Name: server.Name, Type: server.Type, URL: server.URL, BuiltinPackage: p.BuiltinPackageForRuntimeName(server.Name)})
		}
	}
	for _, server := range requested {
		if manifest != nil && p.ReservedBuiltinRuntimeName(server.Name) {
			return nil, fmt.Errorf("reserved built-in MCP name %q must be renamed", server.Name)
		}
		if server.Name == kandevMCPServerName {
			continue
		}
		servers = append(servers, server)
	}
	return servers, nil
}

func builtinPassthroughEndpoint(coreURL string) (string, int, error) {
	endpoint, err := url.Parse(coreURL)
	if err != nil || endpoint.Scheme != urlSchemeHTTP || endpoint.Path != builtinCoreMCPPath || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return "", 0, errors.New("invalid built-in MCP endpoint")
	}
	port, err := strconv.Atoi(endpoint.Port())
	if err != nil || port <= 0 {
		return "", 0, errors.New("invalid built-in MCP port")
	}
	return endpoint.Hostname(), port, nil
}

func (m *Manager) connectPassthroughMCPStream(execution *AgentExecution) {
	client, releaseClient := execution.AcquireAgentCtlClient()
	hasAgentStream := client != nil && client.HasAgentStream()
	releaseClient()
	if m.streamManager != nil && client != nil {
		m.streamManager.ConnectWorkspaceStream(execution, nil)
		// Also open the agent updates stream so the agentctl instance can proxy
		// kandev MCP tool calls to the backend (passthrough has no ACP stream
		// otherwise, so MCP tool calls would hang).
		if !hasAgentStream {
			m.streamManager.ConnectMCPStream(execution)
		}
	}
}
