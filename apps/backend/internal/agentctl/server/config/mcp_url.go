package config

import (
	"github.com/kandev/kandev/internal/common/mcpurl"
)

// MCPReachableHost returns a host that a process in agentctl's execution
// environment can use to reach an instance listener bound to listenHost.
// An empty or unspecified host means agentctl listens on all interfaces, so
// loopback is the narrowest reachable address to advertise.
func MCPReachableHost(listenHost string) string {
	return mcpurl.ReachableHost(listenHost)
}

// MCPServerURL builds an HTTP MCP endpoint for an instance server.
func MCPServerURL(host string, port int, path string) string {
	value, _ := mcpurl.BuiltinURL(host, port, "", false, path)
	return value
}

// MCPServerURLForInstance builds a native endpoint using the effective mode
// accepted by the helper instance.
func MCPServerURLForInstance(host string, port int, instanceID string, scoped bool, path string) (string, error) {
	return mcpurl.BuiltinURL(host, port, instanceID, scoped, path)
}

// MCPServerBaseURL returns the host origin advertised by the SSE transport.
func MCPServerBaseURL(host string, port int) string {
	value, _ := mcpurl.OriginURL(host, port)
	return value
}
