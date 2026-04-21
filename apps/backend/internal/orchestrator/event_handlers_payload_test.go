package orchestrator

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestBuildTaskEventPayload(t *testing.T) {
	now := time.Now().UTC()
	base := &models.Task{
		ID:             "task-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "title",
		Description:    "desc",
		State:          v1.TaskStateTODO,
		Priority:       3,
		Position:       7,
		CreatedAt:      now,
		UpdatedAt:      now,
		IsEphemeral:    false,
	}

	t.Run("omits metadata and parent_id when unset", func(t *testing.T) {
		p := buildTaskEventPayload(base)
		if _, ok := p["metadata"]; ok {
			t.Fatalf("metadata should be absent when Task.Metadata is nil")
		}
		if _, ok := p["parent_id"]; ok {
			t.Fatalf("parent_id should be absent when Task.ParentID is empty")
		}
		if p["task_id"] != "task-1" || p["state"] != string(v1.TaskStateTODO) {
			t.Fatalf("unexpected payload: %#v", p)
		}
	})

	t.Run("includes metadata with review_watch_id for PR review tasks", func(t *testing.T) {
		task := *base
		task.Metadata = map[string]interface{}{"review_watch_id": "watch-123"}
		task.ParentID = "parent-1"

		p := buildTaskEventPayload(&task)

		meta, ok := p["metadata"].(map[string]interface{})
		if !ok {
			t.Fatalf("metadata missing or wrong type: %#v", p["metadata"])
		}
		if meta["review_watch_id"] != "watch-123" {
			t.Fatalf("review_watch_id lost in payload: %#v", meta)
		}
		if p["parent_id"] != "parent-1" {
			t.Fatalf("parent_id missing: %#v", p["parent_id"])
		}
	})
}
