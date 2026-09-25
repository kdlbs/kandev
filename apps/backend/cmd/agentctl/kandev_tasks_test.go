package main

import (
	"net/http"
	"net/url"
	"testing"
)

func TestTasksList_CallsBoardReadEndpoint(t *testing.T) {
	srv, captured := setupMockServer(t, http.StatusOK, `{"tasks":[],"next_cursor":"","next_id":""}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{"tasks", "list"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if captured.Method != http.MethodGet {
		t.Errorf("expected GET, got %s", captured.Method)
	}
	if captured.Path != "/api/v1/office/runtime/tasks" {
		t.Errorf("unexpected path: %s", captured.Path)
	}
	assertAuthHeader(t, captured)
}

// TestTasksList_DoesNotRequireWorkspaceID pins the board-read endpoint's
// authority model: the workspace comes from the run token's claim, not the
// KANDEV_WORKSPACE_ID environment variable, so a taskless run with that
// variable unset must still succeed.
func TestTasksList_DoesNotRequireWorkspaceID(t *testing.T) {
	srv, captured := setupMockServer(t, http.StatusOK, `{"tasks":[]}`)
	setEnvVars(t, srv)
	t.Setenv("KANDEV_WORKSPACE_ID", "")

	code := runKandevCLI([]string{"tasks", "list"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if captured.Path != "/api/v1/office/runtime/tasks" {
		t.Fatalf("request path = %q, want runtime board-read endpoint", captured.Path)
	}
}

func TestTasksList_RepeatableStatusAndPriorityReachServerAsMultipleValues(t *testing.T) {
	srv, captured := setupMockServer(t, http.StatusOK, `{"tasks":[]}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{
		"tasks", "list",
		"--status", "TODO", "--status", "IN_PROGRESS",
		"--priority", "high", "--priority", "medium",
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	parsed, err := url.ParseQuery(captured.Query)
	if err != nil {
		t.Fatalf("parse query %q: %v", captured.Query, err)
	}
	gotStatus := parsed["status"]
	if len(gotStatus) != 2 || gotStatus[0] != "TODO" || gotStatus[1] != "IN_PROGRESS" {
		t.Fatalf("status query values = %v, want [TODO IN_PROGRESS]", gotStatus)
	}
	gotPriority := parsed["priority"]
	if len(gotPriority) != 2 || gotPriority[0] != "high" || gotPriority[1] != "medium" {
		t.Fatalf("priority query values = %v, want [high medium]", gotPriority)
	}
}

func TestTasksList_SingleValuedFlagsReachServer(t *testing.T) {
	srv, captured := setupMockServer(t, http.StatusOK, `{"tasks":[]}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{
		"tasks", "list",
		"--assignee", "agent-1", "--project", "project-1",
		"--sort", "priority", "--order", "asc",
		"--limit", "50", "--cursor", "v1", "--cursor-id", "task-9",
		"--include-system", "true",
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	parsed, err := url.ParseQuery(captured.Query)
	if err != nil {
		t.Fatalf("parse query %q: %v", captured.Query, err)
	}
	want := map[string]string{
		"assignee":       "agent-1",
		"project":        "project-1",
		"sort":           "priority",
		"order":          "asc",
		"limit":          "50",
		"cursor":         "v1",
		"cursor_id":      "task-9",
		"include_system": "true",
	}
	for key, value := range want {
		if got := parsed.Get(key); got != value {
			t.Errorf("query[%s] = %q, want %q (raw=%q)", key, got, value, captured.Query)
		}
	}
}

func TestTasksList_OmittedFlagsAreAbsentFromQuery(t *testing.T) {
	srv, captured := setupMockServer(t, http.StatusOK, `{"tasks":[]}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{"tasks", "list"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if captured.Query != "" {
		t.Fatalf("query = %q, want empty when no flags are passed", captured.Query)
	}
}

func TestTasksList_PropagatesRuntimeScopeDenial(t *testing.T) {
	srv, _ := setupMockServer(t, http.StatusForbidden, `{"error":"capability denied"}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{"tasks", "list"})
	if code != 1 {
		t.Fatalf("tasks list exit = %d, want 1 for HTTP 403", code)
	}
}
