package sqlite

import (
	"context"
	"testing"
)

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
