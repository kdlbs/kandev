package mcpconfig

import "github.com/kandev/kandev/internal/agentctl/types"

// ToACPServers converts resolved MCP servers into ACP stdio server list.
func ToACPServers(resolved []ResolvedServer) []types.McpServer {
	servers := make([]types.McpServer, 0, len(resolved))
	for _, server := range resolved {
		if server.Type != ServerTypeStdio {
			continue
		}
		servers = append(servers, types.McpServer{
			Name:    server.Name,
			Command: server.Command,
			Args:    append([]string{}, server.Args...),
		})
	}
	return servers
}
