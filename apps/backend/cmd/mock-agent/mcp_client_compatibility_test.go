package main

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestMCPClientUsesLegacyProtocolForSSE(t *testing.T) {
	if mcp.IsModernProtocol(mcpSSEProtocolVersion) {
		t.Fatalf("SSE MCP protocol version = %q, want a legacy version", mcpSSEProtocolVersion)
	}
	if mcpSSEProtocolVersion != mcp.ProtocolVersion20241105 {
		t.Fatalf("SSE MCP protocol version = %q, want %q", mcpSSEProtocolVersion, mcp.ProtocolVersion20241105)
	}
}
