package dto

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestTaskNoteFromModel(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	out := TaskNoteFromModel(&models.TaskNote{
		ID:        "note-1",
		TaskID:    "task-1",
		Content:   "notes",
		UpdatedBy: "user",
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now,
	})

	if out == nil {
		t.Fatal("expected dto")
	}
	if out.ID != "note-1" || out.TaskID != "task-1" || out.Content != "notes" || out.UpdatedBy != "user" {
		t.Fatalf("unexpected dto: %+v", out)
	}
	if !out.CreatedAt.Equal(now.Add(-time.Hour)) || !out.UpdatedAt.Equal(now) {
		t.Fatalf("unexpected timestamps: %+v", out)
	}
}
