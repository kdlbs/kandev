package adapter

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/pkg/copilot"
)

func TestMcpServersToCopilotConfig_Nil(t *testing.T) {
	result := mcpServersToCopilotConfig(nil)
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}
}

func TestMcpServersToCopilotConfig_Empty(t *testing.T) {
	result := mcpServersToCopilotConfig([]types.McpServer{})
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

func TestMcpServersToCopilotConfig_StdioServer(t *testing.T) {
	servers := []types.McpServer{
		{
			Name:    "my-stdio-server",
			Type:    "stdio",
			Command: "node",
			Args:    []string{"server.js", "--port", "3000"},
		},
	}

	result := mcpServersToCopilotConfig(servers)

	if len(result) != 1 {
		t.Fatalf("expected 1 server, got %d", len(result))
	}

	cfg, ok := result["my-stdio-server"]
	if !ok {
		t.Fatal("expected server 'my-stdio-server' in result")
	}

	assertConfigValue(t, cfg, "type", "local")
	assertConfigValue(t, cfg, "command", "node")

	args, ok := cfg["args"].([]string)
	if !ok {
		t.Fatalf("expected args to be []string, got %T", cfg["args"])
	}
	if len(args) != 3 || args[0] != "server.js" || args[1] != "--port" || args[2] != "3000" {
		t.Errorf("unexpected args: %v", args)
	}

	tools, ok := cfg["tools"].([]string)
	if !ok {
		t.Fatalf("expected tools to be []string, got %T", cfg["tools"])
	}
	if len(tools) != 1 || tools[0] != "*" {
		t.Errorf("expected tools [\"*\"], got %v", tools)
	}
}

func TestMcpServersToCopilotConfig_DefaultType(t *testing.T) {
	// When type is empty, should default to "local" (stdio)
	servers := []types.McpServer{
		{
			Name:    "default-server",
			Command: "my-tool",
			Args:    []string{"--verbose"},
		},
	}

	result := mcpServersToCopilotConfig(servers)
	cfg := result["default-server"]
	assertConfigValue(t, cfg, "type", "local")
	assertConfigValue(t, cfg, "command", "my-tool")
}

func TestMcpServersToCopilotConfig_SSEServer(t *testing.T) {
	servers := []types.McpServer{
		{
			Name: "my-sse-server",
			Type: "sse",
			URL:  "http://localhost:8080/sse",
		},
	}

	result := mcpServersToCopilotConfig(servers)

	if len(result) != 1 {
		t.Fatalf("expected 1 server, got %d", len(result))
	}

	cfg := result["my-sse-server"]
	assertConfigValue(t, cfg, "type", "sse")
	assertConfigValue(t, cfg, "url", "http://localhost:8080/sse")

	// Should not have command/args
	if _, ok := cfg["command"]; ok {
		t.Error("SSE server should not have 'command' field")
	}
	if _, ok := cfg["args"]; ok {
		t.Error("SSE server should not have 'args' field")
	}
}

func TestMcpServersToCopilotConfig_HTTPServer(t *testing.T) {
	servers := []types.McpServer{
		{
			Name: "my-http-server",
			Type: mcpServerTypeHTTP,
			URL:  "http://localhost:9090/mcp",
		},
	}

	result := mcpServersToCopilotConfig(servers)
	cfg := result["my-http-server"]
	assertConfigValue(t, cfg, "type", mcpServerTypeHTTP)
	assertConfigValue(t, cfg, "url", "http://localhost:9090/mcp")
}

func TestMcpServersToCopilotConfig_MultipleServers(t *testing.T) {
	servers := []types.McpServer{
		{
			Name:    "stdio-server",
			Type:    "stdio",
			Command: "my-tool",
			Args:    []string{"--mode", "dev"},
		},
		{
			Name: "sse-server",
			Type: "sse",
			URL:  "http://localhost:8080/sse",
		},
		{
			Name: "http-server",
			Type: mcpServerTypeHTTP,
			URL:  "http://localhost:9090/mcp",
		},
	}

	result := mcpServersToCopilotConfig(servers)

	if len(result) != 3 {
		t.Fatalf("expected 3 servers, got %d", len(result))
	}

	// Verify each server is present and correctly typed
	assertConfigValue(t, result["stdio-server"], "type", "local")
	assertConfigValue(t, result["stdio-server"], "command", "my-tool")

	assertConfigValue(t, result["sse-server"], "type", "sse")
	assertConfigValue(t, result["sse-server"], "url", "http://localhost:8080/sse")

	assertConfigValue(t, result["http-server"], "type", mcpServerTypeHTTP)
	assertConfigValue(t, result["http-server"], "url", "http://localhost:9090/mcp")
}

func TestMcpServersToCopilotConfig_NilArgs(t *testing.T) {
	// When Args is nil, the "args" key should not be set
	servers := []types.McpServer{
		{
			Name:    "no-args-server",
			Type:    "stdio",
			Command: "simple-tool",
		},
	}

	result := mcpServersToCopilotConfig(servers)
	cfg := result["no-args-server"]

	if _, ok := cfg["args"]; ok {
		t.Error("expected no 'args' key when Args is nil")
	}
}

func TestMcpServersToCopilotConfig_AllHaveWildcardTools(t *testing.T) {
	servers := []types.McpServer{
		{Name: "a", Type: "stdio", Command: "cmd-a"},
		{Name: "b", Type: "sse", URL: "http://b"},
		{Name: "c", Type: mcpServerTypeHTTP, URL: "http://c"},
	}

	result := mcpServersToCopilotConfig(servers)

	for name, cfg := range result {
		tools, ok := cfg["tools"].([]string)
		if !ok {
			t.Errorf("server %q: expected tools to be []string", name)
			continue
		}
		if len(tools) != 1 || tools[0] != "*" {
			t.Errorf("server %q: expected tools [\"*\"], got %v", name, tools)
		}
	}
}

func assertConfigValue(t *testing.T, cfg copilot.MCPServerConfig, key, expected string) {
	t.Helper()
	val, ok := cfg[key].(string)
	if !ok {
		t.Errorf("expected %q to be string, got %T (%v)", key, cfg[key], cfg[key])
		return
	}
	if val != expected {
		t.Errorf("expected %q = %q, got %q", key, expected, val)
	}
}
