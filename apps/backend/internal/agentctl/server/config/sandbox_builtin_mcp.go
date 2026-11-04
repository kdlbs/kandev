package config

import (
	"net/url"

	"github.com/kandev/kandev/internal/common/sandbox"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

const builtinMCPHTTPTransport = "http"

// BuiltinMCPServersForLaunch preserves package selection in the captured transport.
func BuiltinMCPServersForLaunch(manifest *p.BuiltinManifest, host string, port int, launch *sandbox.Launch) ([]McpServerConfig, error) {
	servers, err := BuiltinMCPServers(manifest, host, port)
	if err != nil || launch == nil {
		return servers, err
	}
	if launch.MCPURL == "" {
		return nil, nil
	}
	private := make([]McpServerConfig, 0, len(servers)/2)
	for _, server := range servers {
		if server.Type != builtinMCPHTTPTransport {
			continue
		}
		endpoint, err := url.Parse(server.URL)
		if err != nil {
			return nil, sandbox.ErrInvalidPolicy
		}
		server.URL, err = launch.MCPURLForPath(endpoint.Path)
		if err != nil {
			return nil, err
		}
		private = append(private, server)
	}
	return private, nil
}
