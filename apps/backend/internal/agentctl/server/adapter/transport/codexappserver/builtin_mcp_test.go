package codexappserver

import (
	agenttypes "github.com/kandev/kandev/internal/agentctl/types"
	"strings"
	"testing"
)

func TestBuiltinPackagesCodexConfigurationPreservesLogicalServers(t *testing.T) {
	var input []agenttypes.McpServer
	expected := map[string]string{
		"kandev":               "http://localhost:4231/mcp",
		"kandev-configuration": "http://localhost:4231/mcp/packages/configuration",
		"kandev-walkthroughs":  "http://localhost:4231/mcp/packages/walkthroughs",
	}
	for name, url := range expected {
		input = append(input, agenttypes.McpServer{Name: name, Type: "http", URL: url}, agenttypes.McpServer{Name: name, Type: "sse", URL: strings.Replace(url, "/mcp", "/sse", 1)})
	}
	servers, ok := codexConfig(input)["mcp_servers"].(map[string]any)
	if !ok || len(servers) != len(expected) {
		t.Fatalf("logical package servers lost: %#v", servers)
	}
	for name, url := range expected {
		config, ok := servers[name].(map[string]any)
		if !ok || config["url"] != url {
			t.Fatalf("package %s delivered to %#v", name, config)
		}
	}
}
