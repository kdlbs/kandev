package sandboxmcp

import (
	"context"
)

const MCPPathHeader = "X-Kandev-Sandbox-MCP-Path"

func Open(parent context.Context, forward Forward) (*Relay, error) {
	return OpenWithPaths(parent, forward, nil)
}
