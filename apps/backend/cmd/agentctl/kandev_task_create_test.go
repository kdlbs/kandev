package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
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

// captureCLIStderr runs fn with os.Stderr redirected and returns what it wrote.
func captureCLIStderr(t *testing.T, fn func() int) (int, string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	original := os.Stderr
	os.Stderr = w
	code := fn()
	_ = w.Close()
	os.Stderr = original
	out, readErr := io.ReadAll(r)
	_ = r.Close()
	if readErr != nil {
		t.Fatalf("read stderr: %v", readErr)
	}
	return code, string(out)
}

// The runtime still rejects these fields; the CLI keeps defining them and
// refuses them before sending, as it did before blocker and workspace-mode
// support. Whether to support or remove them is tracked separately.
//
// @covers AC-OFFICE-AGENT-TASK-RELATIONS-004.2
func TestTaskCreate_StillRefusesUnsupportedFlagsWithRuntimeMessage(t *testing.T) {
	for _, flagName := range []string{"priority", "workspace-group-id", "default-child-workspace", "default-child-ordering"} {
		t.Run(flagName, func(t *testing.T) {
			captured := setupMockTransport(t, http.StatusCreated, `{"task_id":"new-1"}`)
			t.Setenv("KANDEV_API_URL", "http://kandev.test")
			t.Setenv("KANDEV_API_KEY", "signed-run-token")

			code, stderr := captureCLIStderr(t, func() int {
				return runKandevCLI([]string{"task", "create", "--title", "Unsupported", "--" + flagName, "value"})
			})
			if code == 0 {
				t.Fatal("expected unsupported flag to fail")
			}
			want := "--" + flagName + " is not supported by Office runtime task create"
			if !strings.Contains(stderr, want) {
				t.Fatalf("stderr = %q, want %q", stderr, want)
			}
			if captured.Method != "" {
				t.Fatalf("server contacted for unsupported flag: %s %s", captured.Method, captured.Path)
			}
		})
	}
}
