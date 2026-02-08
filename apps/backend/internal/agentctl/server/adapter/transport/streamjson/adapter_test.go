package streamjson

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/common/logger"
)

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	return log
}

func TestPrepareCommandArgs_NoServers(t *testing.T) {
	a := NewAdapter(&shared.Config{AgentID: "claude-code"}, newTestLogger(t))
	args := a.PrepareCommandArgs()
	if args != nil {
		t.Fatalf("expected nil args for no MCP servers, got %v", args)
	}
}

func TestPrepareCommandArgs_SSEServer(t *testing.T) {
	a := NewAdapter(&shared.Config{
		AgentID: "claude-code",
		McpServers: []shared.McpServerConfig{
			{Name: "kandev", URL: "http://localhost:10001/sse", Type: "sse"},
		},
	}, newTestLogger(t))

	args := a.PrepareCommandArgs()
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}
	if args[0] != "--mcp-config" {
		t.Fatalf("expected first arg to be --mcp-config, got %s", args[0])
	}

	// Parse the JSON to verify structure
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(args[1]), &config); err != nil {
		t.Fatalf("failed to parse MCP config JSON: %v", err)
	}

	// Must have mcpServers wrapper
	servers, ok := config["mcpServers"]
	if !ok {
		t.Fatalf("MCP config missing 'mcpServers' wrapper key, got: %s", args[1])
	}

	serversMap, ok := servers.(map[string]interface{})
	if !ok {
		t.Fatalf("mcpServers is not an object")
	}

	kandev, ok := serversMap["kandev"]
	if !ok {
		t.Fatalf("mcpServers missing 'kandev' key")
	}

	kandevMap, ok := kandev.(map[string]interface{})
	if !ok {
		t.Fatalf("kandev server config is not an object")
	}

	if kandevMap["url"] != "http://localhost:10001/sse" {
		t.Errorf("expected url 'http://localhost:10001/sse', got %v", kandevMap["url"])
	}
	if kandevMap["type"] != "sse" {
		t.Errorf("expected type 'sse', got %v", kandevMap["type"])
	}
}

func TestPrepareCommandArgs_StdioServer(t *testing.T) {
	a := NewAdapter(&shared.Config{
		AgentID: "claude-code",
		McpServers: []shared.McpServerConfig{
			{Name: "my-tool", Command: "node", Args: []string{"server.js", "--port", "3000"}},
		},
	}, newTestLogger(t))

	args := a.PrepareCommandArgs()
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(args[1]), &config); err != nil {
		t.Fatalf("failed to parse MCP config JSON: %v", err)
	}

	servers, ok := config["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatalf("MCP config missing 'mcpServers' wrapper")
	}

	tool, ok := servers["my-tool"].(map[string]interface{})
	if !ok {
		t.Fatalf("mcpServers missing 'my-tool' key")
	}

	if tool["command"] != "node" {
		t.Errorf("expected command 'node', got %v", tool["command"])
	}

	toolArgs, ok := tool["args"].([]interface{})
	if !ok {
		t.Fatalf("expected args array, got %T", tool["args"])
	}
	if len(toolArgs) != 3 {
		t.Errorf("expected 3 args, got %d", len(toolArgs))
	}
}

func TestPrepareCommandArgs_MultipleServers(t *testing.T) {
	a := NewAdapter(&shared.Config{
		AgentID: "claude-code",
		McpServers: []shared.McpServerConfig{
			{Name: "kandev", URL: "http://localhost:10001/sse", Type: "sse"},
			{Name: "other", Command: "python", Args: []string{"-m", "mcp_server"}},
		},
	}, newTestLogger(t))

	args := a.PrepareCommandArgs()

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(args[1]), &config); err != nil {
		t.Fatalf("failed to parse MCP config JSON: %v", err)
	}

	servers, ok := config["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatalf("MCP config missing 'mcpServers' wrapper")
	}

	if len(servers) != 2 {
		t.Errorf("expected 2 servers, got %d", len(servers))
	}
	if _, ok := servers["kandev"]; !ok {
		t.Error("missing 'kandev' server")
	}
	if _, ok := servers["other"]; !ok {
		t.Error("missing 'other' server")
	}
}
