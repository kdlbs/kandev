package plugins

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestProjectWebAppEventDropsInternalPayloadFields(t *testing.T) {
	event := bus.NewEvent(events.CanvasReleaseActivated, "canvas", map[string]any{
		"canvas_id":         "canvas-1",
		"workspace_id":      "workspace-1",
		"release_id":        "release-1",
		"active_release_id": "release-1",
		"secret":            strings.Repeat("s", 128),
		"manifest_json":     "must-not-leak",
	})

	projected, ok := projectWebAppEvent(event)
	if !ok {
		t.Fatal("projectWebAppEvent() rejected a public lifecycle event")
	}
	raw, err := json.Marshal(projected.Data)
	if err != nil {
		t.Fatalf("marshal projected data: %v", err)
	}
	encoded := string(raw)
	for _, forbidden := range []string{"secret", "manifest_json", "must-not-leak"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("projected event contains forbidden field %q: %s", forbidden, encoded)
		}
	}
	if projected.Scope.WorkspaceID != "workspace-1" || projected.Scope.InstanceID != "" {
		t.Fatalf("projected scope = %+v", projected.Scope)
	}
}

// TestProjectWebAppEventDropsDependencyProjectionFields covers the refresh
// contract's test-strategy commitment (task-dependency-refresh.md): the
// canvas event projection for task.updated, task.dependencies_resolved and
// task.dependency_failed carries no dependency projection field, no edge
// list, and no edge end's title or state. Asserted directly against a
// payload that carries every projection field, not inferred from the field
// allowlist, so a future allowlist edit that accidentally widens it fails
// this test instead of leaking silently.
func TestProjectWebAppEventDropsDependencyProjectionFields(t *testing.T) {
	dependencyFields := map[string]any{
		"blocked":              true,
		"blocked_reason":       "unknown",
		"depends_on":           []map[string]any{{"id": "task-a", "title": "Blocker A", "state": "in_progress", "status": "pending"}},
		"blocks":               []map[string]any{{"id": "task-b", "title": "Dependent B", "state": "backlog"}},
		"depends_on_truncated": true,
		"blocks_truncated":     false,
		"start_when_unblocked": true,
	}
	forbidden := []string{
		"blocked", "blocked_reason", "depends_on", "blocks",
		"depends_on_truncated", "blocks_truncated", "start_when_unblocked",
		"Blocker A", "Dependent B",
	}

	cases := []struct {
		name    string
		event   string
		payload map[string]any
	}{
		{
			name:  "task.updated",
			event: events.TaskUpdated,
			payload: map[string]any{
				"task_id":      "task-1",
				"workspace_id": "workspace-1",
				"title":        "Task one",
				"state":        "in_progress",
			},
		},
		{
			name:  "task.dependencies_resolved",
			event: events.TaskDependenciesResolved,
			payload: map[string]any{
				"task_id":             "task-1",
				"resolved_by_task_id": "task-a",
			},
		},
		{
			name:  "task.dependency_failed",
			event: events.TaskDependencyFailed,
			payload: map[string]any{
				"task_id":        "task-1",
				"failed_task_id": "task-a",
				"failed_state":   "cancelled",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{}
			for k, v := range tc.payload {
				payload[k] = v
			}
			for k, v := range dependencyFields {
				payload[k] = v
			}

			busEvent := bus.NewEvent(tc.event, "task-service", payload)
			projected, ok := projectWebAppEvent(busEvent)
			if !ok {
				t.Fatalf("projectWebAppEvent() rejected %s", tc.event)
			}
			raw, err := json.Marshal(projected.Data)
			if err != nil {
				t.Fatalf("marshal projected data: %v", err)
			}
			encoded := string(raw)
			for _, field := range forbidden {
				if strings.Contains(encoded, field) {
					t.Fatalf("%s projection leaked dependency data %q: %s", tc.event, field, encoded)
				}
			}
			data, ok := projected.Data.(map[string]any)
			if !ok {
				t.Fatalf("%s projected data is %T, want map[string]any", tc.event, projected.Data)
			}
			for _, key := range []string{"blocked", "blocked_reason", "depends_on", "blocks", "depends_on_truncated", "blocks_truncated", "start_when_unblocked"} {
				if _, exists := data[key]; exists {
					t.Fatalf("%s projection kept dependency key %q: %+v", tc.event, key, data)
				}
			}
		})
	}
}
