package main

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestTaskDecision_PostsOnlyDecisionAndReasonToRuntimeEndpoint(t *testing.T) {
	captured := setupMockTransport(t, http.StatusOK, `{"decision":"approved"}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-office-run-token")
	t.Setenv("KANDEV_RUN_ID", "run-456")
	t.Setenv("KANDEV_TASK_ID", "task-from-environment")
	t.Setenv("KANDEV_AGENT_ID", "agent-from-environment")

	code := runKandevCLI([]string{
		"task", "decision", "--decision", "approved", "--reason", "looks good",
	})
	if code != 0 {
		t.Fatalf("task decision exit = %d, want 0", code)
	}
	if captured.Method != http.MethodPost || captured.Path != "/api/v1/office/runtime/task/decision" {
		t.Fatalf("request = %s %s, want POST /api/v1/office/runtime/task/decision", captured.Method, captured.Path)
	}
	assertRunIDHeader(t, captured, "run-456")

	var body map[string]any
	if err := json.Unmarshal([]byte(captured.Body), &body); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if len(body) != 2 || body["decision"] != "approved" || body["reason"] != "looks good" {
		t.Fatalf("request body = %#v, want only decision and reason", body)
	}
}

func TestTaskDecision_RejectsInvalidInputBeforeRequest(t *testing.T) {
	captured := setupMockTransport(t, http.StatusOK, `{}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-office-run-token")

	if code := runKandevCLI([]string{"task", "decision", "--decision", "approve", "--reason", "ok"}); code == 0 {
		t.Fatal("expected invalid decision to fail")
	}
	if captured.Method != "" {
		t.Fatalf("invalid decision contacted server: %s %s", captured.Method, captured.Path)
	}

	if code := runKandevCLI([]string{"task", "decision", "--decision", "approved", "--reason", "  "}); code == 0 {
		t.Fatal("expected blank reason to fail")
	}
	if captured.Method != "" {
		t.Fatalf("blank reason contacted server: %s %s", captured.Method, captured.Path)
	}
}
