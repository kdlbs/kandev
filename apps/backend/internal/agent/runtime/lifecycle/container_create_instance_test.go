package lifecycle

import "testing"

// Regression: the Docker executor's fresh-launch create-instance request forwarded
// SessionID but dropped TaskID, so the MCP server built inside every new container
// ran with an empty task binding and task-bound tools (step_complete_kandev,
// set_task_title_kandev, ...) failed with "requires a bound task".
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
