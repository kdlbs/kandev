package lifecycle

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMaterializeRuntimeProjectMCPForPiWritesProjectFile(t *testing.T) {
	mgr := newTestManager(t)
	execution := &AgentExecution{
		ID:             "exec-1",
		TaskID:         "task-1",
		SessionID:      "session-1",
		AgentProfileID: "profile-1",
		WorkspacePath:  t.TempDir(),
		Metadata:       map[string]interface{}{},
		standalonePort: 45678,
	}
	agentConfig, ok := mgr.registry.Get("pi-acp")
	if !ok {
		t.Fatal("pi-acp agent missing from test registry")
	}

	if err := mgr.materializeRuntimeProjectMCP(context.Background(), execution, agentConfig); err != nil {
		t.Fatalf("materializeRuntimeProjectMCP: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(execution.WorkspacePath, ".pi", "mcp.json"))
	if err != nil {
		t.Fatalf("pi mcp.json not written: %v", err)
	}
	var payload struct {
		MCPServers map[string]struct {
			Transport string `json:"transport"`
			URL       string `json:"url"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("pi mcp.json not valid JSON: %v\n%s", err, data)
	}
	kandev := payload.MCPServers[kandevMCPServerName]
	if kandev.Transport != "streamable-http" {
		t.Fatalf("kandev transport = %q, want streamable-http", kandev.Transport)
	}
	if kandev.URL != "http://localhost:45678/mcp" {
		t.Fatalf("kandev URL = %q", kandev.URL)
	}
}
