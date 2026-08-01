package orchestrator

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/gitlab"
)

// TestDecodeTaskMRUpdatedEvent_TypedPointer is the in-memory event bus shape:
// event.Data is the original *gitlab.TaskMRUpdatedEvent pointer.
func TestDecodeTaskMRUpdatedEvent_TypedPointer(t *testing.T) {
	original := &gitlab.TaskMRUpdatedEvent{
		WorkspaceID: "ws-1",
		TaskMR:      &gitlab.TaskMR{TaskID: "task-1", ProjectPath: "group/a", MRIID: 1},
	}
	got, ok := decodeTaskMRUpdatedEvent(original)
	if !ok || got != original {
		t.Fatalf("expected the original pointer to pass through unchanged, got %+v ok=%v", got, ok)
	}
}

// TestDecodeTaskMRUpdatedEvent_MapShape is the NATS event bus shape: Publish
// JSON-marshals the event and Subscribe unmarshals into the untyped Data
// field, which decodes any JSON object into a plain map — losing the
// concrete *gitlab.TaskMRUpdatedEvent type. Without decodeTaskMRUpdatedEvent
// normalizing this back, handleGitLabTaskMRUpdated's type assertion would
// always fail and every GitLab lifecycle notification would be silently
// dropped whenever the deployment uses NATS.
func TestDecodeTaskMRUpdatedEvent_MapShape(t *testing.T) {
	original := &gitlab.TaskMRUpdatedEvent{
		WorkspaceID: "ws-1",
		TaskMR:      &gitlab.TaskMR{TaskID: "task-1", RepositoryID: "repo-1", ProjectPath: "group/a", MRIID: 1, State: gitlabMRStateMerged},
	}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var asMap map[string]interface{}
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}

	got, ok := decodeTaskMRUpdatedEvent(asMap)
	if !ok || got == nil || got.TaskMR == nil {
		t.Fatalf("expected a decoded event, got %+v ok=%v", got, ok)
	}
	if got.WorkspaceID != "ws-1" || got.TaskID != "task-1" || got.RepositoryID != "repo-1" ||
		got.ProjectPath != "group/a" || got.MRIID != 1 || got.State != gitlabMRStateMerged {
		t.Fatalf("decoded event does not match original: %+v", got)
	}
}

// TestDecodeTaskMRUpdatedEvent_UnknownShape covers the same-defensive-default
// as the original direct type assertion: an unrelated payload is ignored,
// not misinterpreted.
func TestDecodeTaskMRUpdatedEvent_UnknownShape(t *testing.T) {
	got, ok := decodeTaskMRUpdatedEvent("not an event")
	if ok || got != nil {
		t.Fatalf("expected no decode for an unrelated payload, got %+v ok=%v", got, ok)
	}
}
