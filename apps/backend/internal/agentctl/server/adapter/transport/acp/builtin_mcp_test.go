package acp

import (
	"context"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-BUILTIN-MCP-003.3
func TestBuiltinSessionRequestsRejectUnsupportedInjection(t *testing.T) {
	for _, action := range []string{"new", "load", "resume", "reset"} {
		for _, supported := range []bool{false, true} {
			t.Run(action+map[bool]string{false: "/unsupported", true: "/sse fallback"}[supported], func(t *testing.T) {
				a, peer := newSessionResumeAdapter(t, true, true)
				a.capabilities.McpCapabilities = acpsdk.McpCapabilities{Sse: supported}
				servers := []types.McpServer{
					{Name: "kandev", Type: "http", URL: "http://localhost:1234/mcp", BuiltinPackage: p.PackageCore},
					{Name: "kandev", Type: "sse", URL: "http://localhost:1234/sse", BuiltinPackage: p.PackageCore},
					{Name: "kandev-configuration", Type: "http", URL: "http://localhost:1234/mcp/packages/configuration", BuiltinPackage: p.PackageConfiguration},
					{Name: "kandev-configuration", Type: "sse", URL: "http://localhost:1234/sse/packages/configuration", BuiltinPackage: p.PackageConfiguration},
				}
				var err error
				switch action {
				case "new":
					_, err = a.NewSession(context.Background(), servers)
				case "load":
					err = a.LoadSession(context.Background(), "saved", servers)
				case "resume":
					err = a.ResumeSession(context.Background(), "saved", servers)
				case "reset":
					_, err = a.ResetSession(context.Background(), servers)
				}
				if !supported {
					require.ErrorContains(t, err, "built-in MCP")
					require.Zero(t, peer.sessionCounter)
					require.Zero(t, peer.resumeCalls)
					require.Empty(t, peer.loadRequest.SessionId)
					return
				}
				require.NoError(t, err)
				selected := peer.newRequest.McpServers
				if action == "load" || action == "resume" {
					selected = peer.resumeRequest.McpServers
				}
				require.Len(t, selected, 2)
				for _, server := range selected {
					require.NotNil(t, server.Sse)
				}
			})
		}
	}
}
