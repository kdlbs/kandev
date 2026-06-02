package instance

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/pkg/agent"
	"github.com/stretchr/testify/assert"
)

// TestBuildMcpServerConfigs_DropsMissingStdioCommand is the regression
// test for GH issue #1247's "stale `/snap/bin/brave` MCP" repro: a stdio
// MCP entry whose Command binary no longer exists on disk should not be
// passed through to the agent — it would just spawn a permanently-broken
// child process. URL-transport MCPs should always pass through.
func TestBuildMcpServerConfigs_DropsMissingStdioCommand(t *testing.T) {
	mgr := NewManager(&config.Config{
		Ports:    config.PortConfig{Base: 0, Max: 0},
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
		// IdleTimeout zero => reaper goroutine never starts; no shutdown needed.
	}, newTestLogger(t))

	// Create a valid stdio command so we have at least one survivor.
	validCmd := filepath.Join(t.TempDir(), "valid-mcp")
	require := func(err error) {
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	require(os.WriteFile(validCmd, []byte("#!/bin/sh\nsleep 1\n"), 0o755))

	input := []McpServerConfig{
		{Name: "missing-stdio", Command: "/nonexistent/abs/path/does-not-exist"},
		{Name: "missing-on-path", Command: "this-binary-is-definitely-not-on-path-xyzzy-1247"},
		{Name: "ok-stdio", Command: validCmd},
		{Name: "ok-http", URL: "http://localhost:9999/mcp", Type: "http"},
		{Name: "ok-sse", URL: "http://localhost:9999/sse", Type: "sse"},
	}

	got := mgr.buildMcpServerConfigs(input)

	names := make([]string, 0, len(got))
	for _, srv := range got {
		names = append(names, srv.Name)
	}

	assert.NotContains(t, names, "missing-stdio",
		"missing absolute-path stdio MCP must be dropped")
	assert.NotContains(t, names, "missing-on-path",
		"missing on-PATH stdio MCP must be dropped")
	assert.Contains(t, names, "ok-stdio",
		"resolvable stdio MCP must survive")
	assert.Contains(t, names, "ok-http",
		"URL-transport MCP must always survive (no command to validate)")
	assert.Contains(t, names, "ok-sse",
		"URL-transport MCP must always survive (no command to validate)")
}

// TestMcpStdioValidationError covers the helper directly.
func TestMcpStdioValidationError(t *testing.T) {
	tests := []struct {
		name   string
		mcp    McpServerConfig
		expect string // "ok" or "error"
	}{
		{
			name:   "url-only entry skips validation",
			mcp:    McpServerConfig{Name: "x", URL: "http://example.test"},
			expect: "ok",
		},
		{
			name:   "empty command and empty URL fails",
			mcp:    McpServerConfig{Name: "x"},
			expect: "error",
		},
		{
			name:   "missing absolute path fails",
			mcp:    McpServerConfig{Name: "x", Command: "/this/path/does/not/exist"},
			expect: "error",
		},
		{
			name:   "missing PATH binary fails",
			mcp:    McpServerConfig{Name: "x", Command: "definitely-not-a-real-binary-1247"},
			expect: "error",
		},
		{
			name:   "compound command first token resolves",
			mcp:    McpServerConfig{Name: "x", Command: "sh -c 'echo hi'"},
			expect: "ok",
		},
		{
			name:   "compound command with missing first token fails",
			mcp:    McpServerConfig{Name: "x", Command: "definitely-not-a-real-binary-1247 -m mcp_server"},
			expect: "error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mcpStdioValidationError(tc.mcp)
			if tc.expect == "ok" {
				assert.Empty(t, got, "expected validation pass")
			} else {
				assert.NotEmpty(t, got, "expected validation failure")
			}
		})
	}
}
