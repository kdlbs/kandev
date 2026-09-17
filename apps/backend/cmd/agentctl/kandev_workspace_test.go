package main

import (
	"encoding/json"
	"testing"
)

func TestWorkspaceInspectUsesSignedRuntimeEndpoint(t *testing.T) {
	srv, request := setupMockServer(t, 200, `{}`)
	setEnvVars(t, srv)
	if code := runKandevCLI([]string{"task", "inspect", "--id", "board-task"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if request.Method != "GET" || request.Path != "/api/v1/office/runtime/tasks/board-task/details" {
		t.Fatalf("unexpected request: %+v", request)
	}
	assertAuthHeader(t, request)
}

func TestWorkspaceMessageRetainsExplicitSession(t *testing.T) {
	srv, request := setupMockServer(t, 200, `{}`)
	setEnvVars(t, srv)
	if code := runKandevCLI([]string{"task", "manage", "--id", "board-task", "--action", "message", "--session", "work-session", "--prompt", "Check the failing test"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	var body map[string]string
	if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
		t.Fatal(err)
	}
	if request.Method != "POST" || request.Path != "/api/v1/office/runtime/tasks/board-task/manage" || body["session_id"] != "work-session" || body["prompt"] != "Check the failing test" {
		t.Fatalf("unexpected request: %+v", request)
	}
	assertAuthHeader(t, request)
}
