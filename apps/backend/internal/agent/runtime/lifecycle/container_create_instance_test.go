package lifecycle

import "testing"

// TestBuildContainerCreateInstanceRequestForwardsTaskAndSession verifies that a
// fresh Docker request preserves the identity required by task-bound MCP tools.
func TestBuildContainerCreateInstanceRequestForwardsTaskAndSession(t *testing.T) {
	config := ContainerConfig{TaskID: "task-123", SessionID: "session-456"}

	req := buildContainerCreateInstanceRequest(config, "claude-acp", false, false, false, false, nil)

	if req.TaskID != "task-123" {
		t.Fatalf("TaskID = %q, want %q", req.TaskID, "task-123")
	}
	if req.SessionID != "session-456" {
		t.Fatalf("SessionID = %q, want %q", req.SessionID, "session-456")
	}
}
