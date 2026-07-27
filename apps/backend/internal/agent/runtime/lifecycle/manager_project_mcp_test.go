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

func TestMaterializeRuntimeProjectMCPForCursorWritesProjectFile(t *testing.T) {
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
	agentConfig, ok := mgr.registry.Get("cursor-acp")
	if !ok {
		t.Fatal("cursor-acp agent missing from test registry")
	}

	if err := mgr.materializeRuntimeProjectMCP(context.Background(), execution, agentConfig); err != nil {
		t.Fatalf("materializeRuntimeProjectMCP: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(execution.WorkspacePath, ".cursor", "mcp.json"))
	if err != nil {
		t.Fatalf("cursor mcp.json not written: %v", err)
	}
	var payload struct {
		MCPServers map[string]struct {
			URL string `json:"url"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("cursor mcp.json not valid JSON: %v\n%s", err, data)
	}
	kandev := payload.MCPServers[kandevMCPServerName]
	if kandev.URL != "http://localhost:45678/mcp" {
		t.Fatalf("kandev URL = %q", kandev.URL)
	}
}

func TestMaterializeRuntimeProjectMCPSkipsWhenPortUnavailable(t *testing.T) {
	mgr := newTestManager(t)
	execution := &AgentExecution{
		ID:             "exec-1",
		TaskID:         "task-1",
		SessionID:      "session-1",
		AgentProfileID: "profile-1",
		WorkspacePath:  t.TempDir(),
		Metadata:       map[string]interface{}{},
	}
	agentConfig, ok := mgr.registry.Get("pi-acp")
	if !ok {
		t.Fatal("pi-acp agent missing from test registry")
	}

	if err := mgr.materializeRuntimeProjectMCP(context.Background(), execution, agentConfig); err != nil {
		t.Fatalf("materializeRuntimeProjectMCP: %v", err)
	}

	if _, err := os.Stat(filepath.Join(execution.WorkspacePath, ".pi", "mcp.json")); !os.IsNotExist(err) {
		t.Fatalf("expected no pi mcp.json when port unavailable, got err=%v", err)
	}
}

func TestPromoteWorkspaceExecutionResetsCommandWhenProjectMCPFails(t *testing.T) {
	mgr := newTestManager(t)
	mgr.profileResolver = &mockPassthroughProfileResolver{agentName: "pi-acp"}
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, ".pi"), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	execution := &AgentExecution{
		ID:             "exec-1",
		TaskID:         "task-1",
		SessionID:      "session-1",
		AgentProfileID: "profile-1",
		WorkspacePath:  workspace,
		Metadata:       map[string]interface{}{},
		standalonePort: 45678,
	}
	req := &LaunchRequest{
		TaskID:         "task-1",
		SessionID:      "session-1",
		AgentProfileID: "profile-1",
	}

	err := mgr.promoteWorkspaceExecution(context.Background(), execution, req)
	if err == nil {
		t.Fatal("expected promotion to fail when .pi/mcp.json cannot be written")
	}
	if execution.AgentCommand != "" {
		t.Fatalf("AgentCommand = %q, want reset to empty", execution.AgentCommand)
	}
	if execution.ContinueCommand != "" {
		t.Fatalf("ContinueCommand = %q, want reset to empty", execution.ContinueCommand)
	}
	if execution.AgentArgs != nil {
		t.Fatalf("AgentArgs = %#v, want nil after failed promotion", execution.AgentArgs)
	}
	if execution.ContinueArgs != nil {
		t.Fatalf("ContinueArgs = %#v, want nil after failed promotion", execution.ContinueArgs)
	}
	if execution.IsPassthrough {
		t.Fatal("IsPassthrough should be reset after failed promotion")
	}
}
