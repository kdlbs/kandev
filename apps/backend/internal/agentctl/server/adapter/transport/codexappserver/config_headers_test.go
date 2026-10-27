package codexappserver

import (
	"testing"

	agenttypes "github.com/kandev/kandev/internal/agentctl/types"
)

func TestCodexConfigCarriesKandevAuthorizationHeader(t *testing.T) {
	cfg := codexConfig([]agenttypes.McpServer{{
		Name:    "kandev",
		Type:    "http",
		URL:     "http://127.0.0.1:1/mcp",
		Headers: map[string]string{"Authorization": "Bearer abc"},
	}})
	servers, _ := cfg["mcp_servers"].(map[string]any)
	entry, _ := servers["kandev"].(map[string]any)
	headers, _ := entry["http_headers"].(map[string]string)
	if headers["Authorization"] != "Bearer abc" {
		t.Fatalf("entry = %v", entry)
	}
}
