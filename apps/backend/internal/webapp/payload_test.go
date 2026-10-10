package webapp

import (
	"encoding/json"
	"testing"
)

func TestNormalizeBootPayloadGraphDeduplicatesTaskAndSessionEntities(t *testing.T) {
	task := map[string]any{
		"id": "task-1", "title": "Task", "workspace_id": "workspace-1", "workflow_id": "workflow-1",
	}
	session := map[string]any{
		"id": "session-1", "task_id": "task-1", "state": "WAITING_FOR_INPUT", "started_at": "2026-01-01T00:00:00Z",
	}
	payload := NewBootPayload(RouteClassification{}, RuntimeConfig{}, map[string]any{
		"kanban": map[string]any{"workflowId": "workflow-1", "tasks": []any{task}},
		"kanbanMulti": map[string]any{"snapshots": map[string]any{
			"workflow-1": map[string]any{"tasks": []any{task}},
		}},
		"taskSessions": map[string]any{"items": map[string]any{"session-1": session}},
		"taskSessionsByTask": map[string]any{"itemsByTaskId": map[string]any{
			"task-1": []any{session},
		}},
	})
	payload.RouteData = map[string]any{"taskDetail": map[string]any{
		"task":            task,
		"sidebarTaskPage": map[string]any{"entries": []any{map[string]any{"kind": "task", "task": task}}},
	}}

	if err := NormalizeBootPayloadGraph(&payload); err != nil {
		t.Fatalf("NormalizeBootPayloadGraph: %v", err)
	}
	if payload.Version != 2 {
		t.Fatalf("version = %d, want 2", payload.Version)
	}
	if len(payload.Entities.Tasks) != 1 || len(payload.Entities.Sessions) != 1 {
		t.Fatalf("entity counts = tasks:%d sessions:%d, want 1 each", len(payload.Entities.Tasks), len(payload.Entities.Sessions))
	}

	var state map[string]any
	if err := json.Unmarshal(mustMarshal(t, payload.InitialState), &state); err != nil {
		t.Fatalf("decode normalized state: %v", err)
	}
	kanban := state["kanban"].(map[string]any)
	if got := kanban["taskIds"].([]any); len(got) != 1 || got[0] != "task-1" {
		t.Fatalf("kanban taskIds = %#v", got)
	}
	if _, exists := kanban["tasks"]; exists {
		t.Fatal("normalized kanban state still contains task objects")
	}
	sessions := state["taskSessions"].(map[string]any)
	if got := sessions["sessionIds"].([]any); len(got) != 1 || got[0] != "session-1" {
		t.Fatalf("task session IDs = %#v", got)
	}
	if _, exists := sessions["items"]; exists {
		t.Fatal("normalized task session state still contains session objects")
	}
	taskDetail := payload.RouteData["taskDetail"].(map[string]any)
	if taskDetail["taskId"] != "task-1" {
		t.Fatalf("task detail taskId = %#v", taskDetail["taskId"])
	}
	if _, exists := taskDetail["task"]; exists {
		t.Fatal("normalized task detail still contains its task object")
	}
	page := taskDetail["sidebarTaskPage"].(map[string]any)
	entries := page["entries"].([]any)
	entry := entries[0].(map[string]any)
	if entry["taskId"] != "task-1" {
		t.Fatalf("normalized sidebar task reference = %#v", entry["taskId"])
	}
}

func TestNormalizeBootPayloadGraphKeepsOmittedOptionalState(t *testing.T) {
	payload := NewBootPayload(RouteClassification{}, RuntimeConfig{}, nil)
	if err := NormalizeBootPayloadGraph(&payload); err != nil {
		t.Fatalf("NormalizeBootPayloadGraph: %v", err)
	}
	if payload.InitialState == nil {
		t.Fatal("normalized payload must retain an empty initialState")
	}
	if payload.Entities == nil || payload.Entities.Tasks == nil || payload.Entities.Sessions == nil {
		t.Fatalf("normalized entity graph is incomplete: %#v", payload.Entities)
	}
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}
