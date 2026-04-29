// Package ports defines the default service ports used by kandev.
// These values are the single source of truth in Go; the TypeScript CLI
// mirrors them in apps/cli/src/constants.ts.
//
// Ports cluster around 37429–39429 to avoid collisions with commonly used
// ports (8080, 9090, 9999, etc.) while keeping them memorable. The external
// MCP endpoint is served by the backend HTTP server at /mcp on the Backend port.
package ports

const (
	// Backend is the default HTTP port for the kandev Go backend.
	Backend = 38429

	// Web is the default HTTP port for the Next.js web server.
	Web = 37429

	// AgentCtl is the default control port for the agentctl sidecar.
	AgentCtl = 39429
)
