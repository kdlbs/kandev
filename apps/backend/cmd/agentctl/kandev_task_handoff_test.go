package main

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestTaskHandoff_PostsRequiredFieldsToRuntimeEndpoint(t *testing.T) {
	captured := setupMockTransport(t, http.StatusOK, `{"task_id":"task-new","outcome":"created"}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-office-run-token")
	t.Setenv("KANDEV_RUN_ID", "run-456")

	code := runKandevCLI([]string{
		"task", "handoff",
		"--target-workspace-id", "ws-target",
		"--workflow-id", "wf-target",
		"--title", "Delivery task",
		"--prompt", "Do the delivery work",
		"--agent-profile-id", "profile-worker",
		"--executor-profile-id", "executor-local",
	})
	if code != 0 {
		t.Fatalf("task handoff exit = %d, want 0", code)
	}
	if captured.Method != http.MethodPost || captured.Path != "/api/v1/office/runtime/handoffs" {
		t.Fatalf("request = %s %s, want POST /api/v1/office/runtime/handoffs", captured.Method, captured.Path)
	}
	assertRunIDHeader(t, captured, "run-456")

	var body map[string]any
	if err := json.Unmarshal([]byte(captured.Body), &body); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	want := map[string]any{
		"target_workspace_id": "ws-target",
		"workflow_id":         "wf-target",
		"title":               "Delivery task",
		"prompt":              "Do the delivery work",
		"agent_profile_id":    "profile-worker",
		"executor_profile_id": "executor-local",
	}
	if len(body) != len(want) {
		t.Fatalf("request body = %#v, want exactly %#v", body, want)
	}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("body[%q] = %#v, want %#v", k, body[k], v)
		}
	}
}

func TestTaskHandoff_IncludesOptionalFlagsWhenSupplied(t *testing.T) {
	captured := setupMockTransport(t, http.StatusOK, `{}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-office-run-token")

	code := runKandevCLI([]string{
		"task", "handoff",
		"--target-workspace-id", "ws-target",
		"--workflow-id", "wf-target",
		"--title", "Delivery task",
		"--prompt", "Do the delivery work",
		"--agent-profile-id", "profile-worker",
		"--executor-profile-id", "executor-local",
		"--repository-id", "repo-1",
		"--base-branch", "main",
		"--start-agent",
		"--external-id", "ext-123",
	})
	if code != 0 {
		t.Fatalf("task handoff exit = %d, want 0", code)
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(captured.Body), &body); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if body["repository_id"] != "repo-1" || body["base_branch"] != "main" {
		t.Errorf("optional repository fields missing: %#v", body)
	}
	if body["start_agent"] != true {
		t.Errorf("start_agent = %#v, want true", body["start_agent"])
	}
	if body["external_id"] != "ext-123" {
		t.Errorf("external_id = %#v, want ext-123", body["external_id"])
	}
}

// TestTaskHandoff_SuppliedButEmptyFlagReachesRequestBody is AC-5e: the
// command must distinguish a flag that was not supplied from one supplied
// with an empty value, and must NOT drop a supplied-but-empty value from the
// request body — dropping it would make AC-5a (the route's blank-vs-absent
// rejection) unreachable through the command.
func TestTaskHandoff_SuppliedButEmptyFlagReachesRequestBody(t *testing.T) {
	captured := setupMockTransport(t, http.StatusBadRequest, `{"error":"repository_id must not be blank"}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-office-run-token")

	runKandevCLI([]string{
		"task", "handoff",
		"--target-workspace-id", "ws-target",
		"--workflow-id", "wf-target",
		"--title", "Delivery task",
		"--prompt", "Do the delivery work",
		"--agent-profile-id", "profile-worker",
		"--executor-profile-id", "executor-local",
		"--repository-id", "",
	})

	var body map[string]any
	if err := json.Unmarshal([]byte(captured.Body), &body); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	repoID, ok := body["repository_id"]
	if !ok {
		t.Fatal("expected repository_id key to reach the request body even though its value is empty")
	}
	if repoID != "" {
		t.Errorf("repository_id = %#v, want empty string", repoID)
	}
}

func TestTaskHandoff_OmittedOptionalFlagsAreAbsentFromRequestBody(t *testing.T) {
	captured := setupMockTransport(t, http.StatusOK, `{}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-office-run-token")

	runKandevCLI([]string{
		"task", "handoff",
		"--target-workspace-id", "ws-target",
		"--workflow-id", "wf-target",
		"--title", "Delivery task",
		"--prompt", "Do the delivery work",
		"--agent-profile-id", "profile-worker",
		"--executor-profile-id", "executor-local",
	})

	var body map[string]any
	if err := json.Unmarshal([]byte(captured.Body), &body); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	for _, key := range []string{"repository_id", "base_branch", "start_agent", "external_id"} {
		if _, ok := body[key]; ok {
			t.Errorf("unsupplied flag %q leaked into request body: %#v", key, body[key])
		}
	}
}

func TestTaskHandoff_RejectsPositionalArguments(t *testing.T) {
	captured := setupMockTransport(t, http.StatusOK, `{}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-office-run-token")

	code := runKandevCLI([]string{
		"task", "handoff",
		"--target-workspace-id", "ws-target",
		"--workflow-id", "wf-target",
		"--title", "Delivery task",
		"--prompt", "Do the delivery work",
		"--agent-profile-id", "profile-worker",
		"--executor-profile-id", "executor-local",
		"unexpected-positional-arg",
	})
	if code == 0 {
		t.Fatal("expected a positional argument to be rejected")
	}
	if captured.Method != "" {
		t.Fatalf("positional argument contacted server: %s %s", captured.Method, captured.Path)
	}
}
