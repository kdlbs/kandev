package lifecycle

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctlconfig "github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/mcpurl"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

const builtinCoreMCPPath = "/mcp"

func builtinPassthroughMCPServers(execution *AgentExecution, client *agentctl.Client, coreURL string, requested []types.McpServer) ([]types.McpServer, error) {
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
	host, port, err := builtinPassthroughEndpoint(coreURL, client)
	if err != nil {
		return nil, err
	}
	instanceID, scoped := builtinMCPRouteMode(client)
	native, err := agentctlconfig.BuiltinMCPServersForInstance(manifest, host, port, instanceID, scoped)
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

func builtinPassthroughEndpoint(coreURL string, client *agentctl.Client) (string, int, error) {
	if client == nil {
		return "", 0, errors.New("agent helper is unavailable for built-in MCP delivery")
	}
	endpoint, err := url.Parse(coreURL)
	if err != nil {
		return "", 0, errors.New("invalid built-in MCP endpoint")
	}
	instanceID, scoped := builtinMCPRouteMode(client)
	wantPath, pathErr := mcpurl.BuiltinPath(instanceID, scoped, builtinCoreMCPPath)
	if pathErr != nil || !matchesBuiltinMCPURLShape(endpoint, wantPath) {
		return "", 0, errors.New("invalid built-in MCP endpoint")
	}
	port, err := strconv.Atoi(endpoint.Port())
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, errors.New("invalid built-in MCP port")
	}
	return endpoint.Hostname(), port, nil
}

func matchesBuiltinMCPURLShape(endpoint *url.URL, wantPath string) bool {
	return endpoint.Scheme == urlSchemeHTTP && endpoint.Path == wantPath && endpoint.RawPath == "" &&
		endpoint.Opaque == "" && endpoint.Hostname() != "" && endpoint.User == nil &&
		endpoint.RawQuery == "" && !endpoint.ForceQuery && endpoint.Fragment == ""
}

func canonicalBuiltinMCPURL(host string, port int, client *agentctl.Client) (string, error) {
	instanceID, scoped := builtinMCPRouteMode(client)
	return agentctlconfig.MCPServerURLForInstance(host, port, instanceID, scoped, builtinCoreMCPPath)
}

func validateBuiltinMCPURL(coreURL string, client *agentctl.Client) (string, int, error) {
	host, port, err := builtinPassthroughEndpoint(coreURL, client)
	if err != nil {
		return "", 0, err
	}
	canonical, err := canonicalBuiltinMCPURL(host, port, client)
	if err != nil || canonical != coreURL {
		return "", 0, errors.New("invalid built-in MCP endpoint")
	}
	return host, port, nil
}

func builtinMCPRouteMode(client *agentctl.Client) (string, bool) {
	if client == nil {
		return "", false
	}
	return client.InstanceID(), client.BuiltinMCPScopedRoutes()
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
