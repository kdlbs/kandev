package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestSetSessionMetadataKeyIfStampIsAtomicAndPreservesOtherKeys(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, "session-cas-write-task", "session-cas-write", "turn-cas-write")
	ctx := context.Background()
	if err := repo.SetSessionMetadataKey(ctx, "session-cas-write", "other_key", "keep me"); err != nil {
		t.Fatalf("seed session metadata: %v", err)
	}
	if err := repo.SetSessionMetadataKey(ctx, "session-cas-write", models.SessionMetaKeyLastAgentError, models.LastAgentError{
		Message:    "old error",
		OccurredAt: time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC),
		StampValue: "old-stamp",
	}); err != nil {
		t.Fatalf("seed session error: %v", err)
	}

	stored, err := repo.SetSessionMetadataKeyIfStamp(ctx, "session-cas-write", models.SessionMetaKeyLastAgentError, "old-stamp", models.LastAgentError{
		Message:    "new error",
		OccurredAt: time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC),
		StampValue: "new-stamp",
	})
	if err != nil {
		t.Fatalf("stamped session write: %v", err)
	}
	if !stored {
		t.Fatal("current session error was not replaced")
	}

	stored, err = repo.SetSessionMetadataKeyIfStamp(ctx, "session-cas-write", models.SessionMetaKeyLastAgentError, "old-stamp", models.LastAgentError{
		Message:    "stale error",
		OccurredAt: time.Date(2026, 8, 20, 11, 0, 0, 0, time.UTC),
		StampValue: "stale-stamp",
	})
	if err != nil {
		t.Fatalf("stale stamped session write: %v", err)
	}
	if stored {
		t.Fatal("stale session write replaced the newer error")
	}

	session, err := repo.GetTaskSession(ctx, "session-cas-write")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	lastError, ok := models.LoadLastAgentError(session.Metadata)
	if !ok || lastError.Stamp() != "new-stamp" {
		t.Fatalf("session error = %#v, want new-stamp", lastError)
	}
	if session.Metadata["other_key"] != "keep me" {
		t.Fatalf("other session metadata = %#v, want preserved value", session.Metadata["other_key"])
	}
}

func TestRemoveSessionMetadataKeyIfStampDoesNotEraseNewerError(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, "task-session-error-cas", "session-error-cas", "turn-error-cas")
	ctx := context.Background()
	if err := repo.SetSessionMetadataKey(ctx, "session-error-cas", "last_agent_error", map[string]interface{}{
		"message":     "new error",
		"stamp":       "new-stamp",
		"occurred_at": "2026-08-19T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed session error: %v", err)
	}

	removed, err := repo.RemoveSessionMetadataKeyIfStamp(ctx, "session-error-cas", "last_agent_error", "old-stamp")
	if err != nil {
		t.Fatalf("RemoveSessionMetadataKeyIfStamp: %v", err)
	}
	if removed {
		t.Fatal("stale stamp removed a newer session error")
	}

	removed, err = repo.RemoveSessionMetadataKeyIfStamp(ctx, "session-error-cas", "last_agent_error", "new-stamp")
	if err != nil {
		t.Fatalf("RemoveSessionMetadataKeyIfStamp with current stamp: %v", err)
	}
	if !removed {
		t.Fatal("current stamp did not remove the session error")
	}
}
