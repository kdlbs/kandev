package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

func TestHandleAgentStalled_PersistsNeutralRunningNotice(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "step-1")
	before, err := repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("get session before handling stall: %v", err)
	}
	svc := createTestServiceWithScheduler(
		repo,
		newMockStepGetter(),
		newMockTaskRepo(),
		&mockAgentManager{repoForExecutionLookup: repo},
	)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	svc.handleAgentStalled(ctx, lifecycle.AgentStalledPayload{
		AgentExecutionID: "execution-1",
		TaskID:           "task-1",
		SessionID:        "session-1",
		PromptGeneration: 7,
		ToolName:         "shell",
		ToolTitle:        "Start dev server",
		ToolStatus:       "in_progress",
	})

	if len(messages.sessionMessages) != 1 {
		t.Fatalf("session messages = %d, want 1", len(messages.sessionMessages))
	}
	message := messages.sessionMessages[0]
	if !strings.Contains(message.content, "Still waiting on Start dev server") {
		t.Fatalf("notice content = %q, want sanitized tool title", message.content)
	}
	if message.metadata["action_visibility"] != "running" {
		t.Fatalf("action visibility = %v, want running", message.metadata["action_visibility"])
	}
	if _, hasVariant := message.metadata["variant"]; hasVariant {
		t.Fatalf("notice metadata unexpectedly set a warning/error variant: %#v", message.metadata)
	}
	actions, ok := message.metadata["actions"].([]map[string]interface{})
	if !ok || len(actions) != 1 {
		t.Fatalf("actions = %#v, want one cancel action", message.metadata["actions"])
	}
	action := actions[0]
	if action["label"] != "Cancel turn" || action["test_id"] != "stall-cancel-turn-button" {
		t.Fatalf("cancel action = %#v", action)
	}
	params, ok := action["params"].(map[string]interface{})
	if !ok || params["method"] != "agent.cancel" {
		t.Fatalf("cancel params = %#v, want agent.cancel", action["params"])
	}

	after, err := repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("get session after handling stall: %v", err)
	}
	if after.State != before.State {
		t.Fatalf("session state changed from %q to %q", before.State, after.State)
	}
}
