package backendapp

import (
	"context"
	"encoding/json"
	"testing"

	cursorcloudruntime "github.com/kandev/kandev/internal/agent/runtime/cursorcloud"
	provider "github.com/kandev/kandev/internal/cursorcloud"
	"github.com/kandev/kandev/internal/task/models"
)

func TestProjectCursorCloudStreamPreservesMessageAndToolIdentity(t *testing.T) {
	binding := &models.ManagedAgentBinding{ID: "binding", SessionID: "session", TaskID: "task", ExecutionID: "execution"}
	operation := &models.ManagedAgentOperation{ID: "operation", RequestSnapshot: models.ManagedAgentRequestSnapshot{TurnID: "turn"}}
	project := func(eventType string, body map[string]any) *cursorcloudruntime.StreamProjection {
		t.Helper()
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		projection, err := projectCursorCloudStream(context.Background(), binding, operation, provider.StreamEvent{
			Type: eventType, Data: encoded,
		})
		if err != nil {
			t.Fatalf("project %s event: %v", eventType, err)
		}
		return projection
	}

	first := project("assistant", map[string]any{"text": "hello"})
	if first.Message.ID != "cursor-cloud-assistant-operation" || first.Message.TurnID != "turn" || !first.AppendMessage {
		t.Fatalf("assistant projection = %+v, want stable append message", first)
	}
	tool := project("tool_call", map[string]any{
		"callId": "call-1", "name": "read_file", "status": "running",
		"truncated": map[string]any{"args": true, "output": false},
		"args":      map[string]any{"secret": "excluded"},
	})
	if tool.Message.ID != "cursor-cloud-tool-operation-call-1" || tool.Payload.Data.ToolCallID != "call-1" {
		t.Fatalf("tool projection identity = %+v, want call-1", tool)
	}
	markers, ok := tool.Message.Metadata["truncated"].(map[string]bool)
	if !ok || !markers["args"] {
		t.Fatalf("tool truncation markers = %#v", tool.Message.Metadata)
	}
	if _, leaked := tool.Message.Metadata["args"]; leaked {
		t.Fatal("tool projection persisted provider arguments")
	}

	result := project("result", map[string]any{"status": "FINISHED", "text": "final"})
	if result.Message.ID != first.Message.ID || result.Message.Content != "final" || result.AppendMessage || result.TerminalStatus != "FINISHED" {
		t.Fatalf("result projection = %+v, want replacement of the assistant message", result)
	}
	if ignored, err := projectCursorCloudStream(context.Background(), binding, operation, provider.StreamEvent{Type: "interaction_update"}); err != nil || ignored != nil {
		t.Fatalf("interaction update = %+v, %v; want ignored duplicate event", ignored, err)
	}
}
