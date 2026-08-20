package sqlite

import (
	"context"
	"testing"
)

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
