package main

import (
	"encoding/json"
	"net/http"
	"testing"
)

func taskCreateBody(t *testing.T, captured *capturedRequest) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal([]byte(captured.Body), &body); err != nil {
		t.Fatalf("unmarshal body %q: %v", captured.Body, err)
	}
	return body
}

// @covers AC-OFFICE-AGENT-TASK-RELATIONS-004.1
func TestTaskCreate_SendsBlockedByAndWorkspaceMode(t *testing.T) {
	captured := setupMockTransport(t, http.StatusCreated, `{"task_id":"join-1"}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-run-token")

	code := runKandevCLI([]string{
		"task", "create", "--title", "Join", "--parent", "parent-1",
		"--blocked-by", " task-a , task-b ", "--workspace-mode", "new_workspace",
	})
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if captured.Path != "/api/v1/office/runtime/tasks" {
		t.Fatalf("path = %q, want runtime task endpoint", captured.Path)
	}
	body := taskCreateBody(t, captured)
	blockedBy, ok := body["blocked_by"].([]interface{})
	if !ok || len(blockedBy) != 2 || blockedBy[0] != "task-a" || blockedBy[1] != "task-b" {
		t.Fatalf("blocked_by = %#v, want [task-a task-b]", body["blocked_by"])
	}
	if body["workspace_mode"] != "new_workspace" {
		t.Fatalf("workspace_mode = %#v, want new_workspace", body["workspace_mode"])
	}
}

// @covers AC-OFFICE-AGENT-TASK-RELATIONS-004.4
func TestTaskCreate_RejectsEmptyBlockedByEntryBeforeRequest(t *testing.T) {
	for _, value := range []string{"task-a,,task-b", "task-a, ", ","} {
		t.Run(value, func(t *testing.T) {
			captured := setupMockTransport(t, http.StatusCreated, `{"task_id":"join-1"}`)
			t.Setenv("KANDEV_API_URL", "http://kandev.test")
			t.Setenv("KANDEV_API_KEY", "signed-run-token")

			if code := runKandevCLI([]string{"task", "create", "--title", "Join", "--blocked-by", value}); code == 0 {
				t.Fatal("expected an empty blocked-by entry to fail")
			}
			if captured.Method != "" {
				t.Fatalf("server contacted for empty blocked-by entry: %s %s", captured.Method, captured.Path)
			}
		})
	}
}

// Server-side validation stays authoritative: a mode the CLI does not check,
// such as shared_group, reaches the runtime and its 400 fails the command.
func TestTaskCreate_SurfacesServerRelationRejection(t *testing.T) {
	captured := setupMockTransport(t, http.StatusBadRequest, `{"error":"invalid task relations: invalid workspace_mode"}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-run-token")

	if code := runKandevCLI([]string{"task", "create", "--title", "Join", "--workspace-mode", "shared_group"}); code == 0 {
		t.Fatal("expected the server's 400 to fail the command")
	}
	if body := taskCreateBody(t, captured); body["workspace_mode"] != "shared_group" {
		t.Fatalf("workspace_mode = %#v, want shared_group passed through", body["workspace_mode"])
	}
}
