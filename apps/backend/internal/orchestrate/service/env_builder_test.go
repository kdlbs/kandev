package service_test

import (
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

func TestBuildEnvVars_AllFieldsPopulated(t *testing.T) {
	svc := newTestService(t)
	svc.SetAPIBaseURL("http://localhost:8080/api/v1")
	si := service.NewSchedulerIntegration(svc, 0)

	agent := &models.AgentInstance{
		ID:          "agent-1",
		Name:        "CEO",
		WorkspaceID: "ws-1",
	}
	wakeup := &models.WakeupRequest{
		ID:      "wake-1",
		Reason:  "task_assigned",
		Payload: `{"task_id":"t-42","comment_id":"c-99"}`,
	}

	env := service.BuildEnvVarsForTest(si, wakeup, agent, "jwt-token", "ws-1")

	expected := map[string]string{
		"KANDEV_API_URL":         "http://localhost:8080/api/v1",
		"KANDEV_API_KEY":         "jwt-token",
		"KANDEV_AGENT_ID":        "agent-1",
		"KANDEV_AGENT_NAME":      "CEO",
		"KANDEV_WORKSPACE_ID":    "ws-1",
		"KANDEV_RUN_ID":          "wake-1",
		"KANDEV_WAKE_REASON":     "task_assigned",
		"KANDEV_TASK_ID":         "t-42",
		"KANDEV_WAKE_COMMENT_ID": "c-99",
	}

	for k, want := range expected {
		got, ok := env[k]
		if !ok {
			t.Errorf("missing env var %s", k)
			continue
		}
		if got != want {
			t.Errorf("env[%s] = %q, want %q", k, got, want)
		}
	}

	if len(env) != len(expected) {
		t.Errorf("env has %d entries, want %d", len(env), len(expected))
	}
}

func TestBuildEnvVars_NoOptionalFields(t *testing.T) {
	svc := newTestService(t)
	svc.SetAPIBaseURL("http://localhost:8080/api/v1")
	si := service.NewSchedulerIntegration(svc, 0)

	agent := &models.AgentInstance{
		ID:          "agent-2",
		Name:        "Worker",
		WorkspaceID: "ws-1",
	}
	wakeup := &models.WakeupRequest{
		ID:      "wake-2",
		Reason:  "heartbeat",
		Payload: `{}`,
	}

	env := service.BuildEnvVarsForTest(si, wakeup, agent, "jwt-2", "ws-1")

	if _, ok := env["KANDEV_TASK_ID"]; ok {
		t.Error("KANDEV_TASK_ID should not be set for empty payload")
	}
	if _, ok := env["KANDEV_WAKE_COMMENT_ID"]; ok {
		t.Error("KANDEV_WAKE_COMMENT_ID should not be set for empty payload")
	}

	// Core vars must still be present.
	for _, key := range []string{
		"KANDEV_API_URL", "KANDEV_API_KEY", "KANDEV_AGENT_ID",
		"KANDEV_AGENT_NAME", "KANDEV_WORKSPACE_ID", "KANDEV_RUN_ID",
		"KANDEV_WAKE_REASON",
	} {
		if _, ok := env[key]; !ok {
			t.Errorf("missing required env var %s", key)
		}
	}
}

func TestBuildEnvVars_TaskIDExtractedFromPayload(t *testing.T) {
	svc := newTestService(t)
	si := service.NewSchedulerIntegration(svc, 0)

	agent := &models.AgentInstance{ID: "a1", Name: "W"}
	wakeup := &models.WakeupRequest{
		ID:      "w1",
		Reason:  "task_comment",
		Payload: `{"task_id":"task-xyz"}`,
	}

	env := service.BuildEnvVarsForTest(si, wakeup, agent, "", "")
	if env["KANDEV_TASK_ID"] != "task-xyz" {
		t.Errorf("KANDEV_TASK_ID = %q, want %q", env["KANDEV_TASK_ID"], "task-xyz")
	}
}
