package mcpconfig

import "github.com/kandev/kandev/internal/agentctl/types"

// ToACPServers converts resolved MCP servers into ACP server list.
// Supports stdio, SSE, HTTP, and streamable HTTP transports.
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
				Env:     cloneStringMap(server.Env),
			})
		case ServerTypeSSE:
			servers = append(servers, types.McpServer{
				Name:    server.Name,
				Type:    "sse",
				URL:     server.URL,
				Headers: cloneStringMap(server.Headers),
			})
		case ServerTypeHTTP:
			servers = append(servers, types.McpServer{
				Name:    server.Name,
				Type:    "http",
				URL:     server.URL,
				Headers: cloneStringMap(server.Headers),
			})
		case ServerTypeStreamableHTTP:
			servers = append(servers, types.McpServer{
				Name:    server.Name,
				Type:    "streamable_http",
				URL:     server.URL,
				Headers: cloneStringMap(server.Headers),
			})
		}
	}
	return servers
}
