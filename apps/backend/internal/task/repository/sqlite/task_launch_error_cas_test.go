package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestSetTaskMetadataKeyIfStampAndDifferentStampAreAtomic(t *testing.T) {
	repo := newRepoForMetadataCASTests(t)
	seedMetadataCASTask(t, repo, map[string]interface{}{
		"last_launch_error": models.TaskLaunchError{
			Message:    "old error",
			OccurredAt: time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC),
			StampValue: "old-stamp",
		},
		"other_key": "keep me",
	})
	ctx := context.Background()

	stored, err := repo.SetTaskMetadataKeyIfStamp(ctx, casTaskID, models.MetaKeyLastLaunchError, "old-stamp", models.TaskLaunchError{
		Message:    "new error",
		OccurredAt: time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC),
		StampValue: "new-stamp",
	})
	if err != nil {
		t.Fatalf("stamped task write: %v", err)
	}
	if !stored {
		t.Fatal("current task error was not replaced")
	}

	var noOp bool
	stored, noOp, err = repo.SetTaskMetadataKeyIfDifferentStamp(ctx, casTaskID, models.MetaKeyLastLaunchError, "new-stamp", models.TaskLaunchError{
		Message:    "duplicate error",
		OccurredAt: time.Date(2026, 8, 20, 11, 0, 0, 0, time.UTC),
		StampValue: "new-stamp",
	})
	if err != nil {
		t.Fatalf("same-stamp task write: %v", err)
	}
	if stored {
		t.Fatal("same-stamp task write changed the first occurrence")
	}
	if !noOp {
		t.Fatal("same-stamp task write did not report a confirmed no-op")
	}

	task, err := repo.GetTask(ctx, casTaskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	launchError, ok := models.LoadTaskLaunchError(task.Metadata)
	if !ok || launchError.Stamp() != "new-stamp" || !launchError.OccurredAt.Equal(time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("task error = %#v, want first new-stamp occurrence", launchError)
	}
	if task.Metadata["other_key"] != "keep me" {
		t.Fatalf("other task metadata = %#v, want preserved value", task.Metadata["other_key"])
	}
}

func TestRemoveTaskMetadataKeyIfStampDoesNotEraseNewerError(t *testing.T) {
	repo := newRepoForMetadataCASTests(t)
	seedMetadataCASTask(t, repo, map[string]interface{}{
		"last_launch_error": map[string]interface{}{
			"message":     "new error",
			"stamp":       "new-stamp",
			"occurred_at": "2026-08-19T00:00:00Z",
		},
	})

	removed, err := repo.RemoveTaskMetadataKeyIfStamp(
		context.Background(), casTaskID, "last_launch_error", "old-stamp",
	)
	if err != nil {
		t.Fatalf("RemoveTaskMetadataKeyIfStamp: %v", err)
	}
	if removed {
		t.Fatal("stale stamp removed a newer launch error")
	}
	if _, ok := metadataValue(t, repo, "last_launch_error"); !ok {
		t.Fatal("newer launch error disappeared after stale clear")
	}

	removed, err = repo.RemoveTaskMetadataKeyIfStamp(
		context.Background(), casTaskID, "last_launch_error", "new-stamp",
	)
	if err != nil {
		t.Fatalf("RemoveTaskMetadataKeyIfStamp with current stamp: %v", err)
	}
	if !removed {
		t.Fatal("current stamp did not remove the launch error")
	}
}
