package mcpconfig

import "github.com/kandev/kandev/internal/agentctl/types"

// ToACPServers converts resolved MCP servers into ACP server list.
// Supports both stdio and SSE transports.
func ToACPServers(resolved []ResolvedServer) []types.McpServer {
	servers := make([]types.McpServer, 0, len(resolved))
	for _, server := range resolved {
		switch server.Type {
		case ServerTypeStdio:
			servers = append(servers, types.McpServer{
				Name:    server.Name,
				Type:    "stdio",
				Command: server.Command,
				Args:    append([]string{}, server.Args...),
			})
		case ServerTypeSSE:
			servers = append(servers, types.McpServer{
				Name: server.Name,
				Type: "sse",
				URL:  server.URL,
			})
		}
	}
	return servers
}
