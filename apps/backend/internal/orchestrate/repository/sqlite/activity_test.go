package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

func TestActivityEntry_CreateAndList(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	entry := &models.ActivityEntry{
		WorkspaceID: "ws-1",
		ActorType:   "user",
		ActorID:     "user-1",
		Action:      "created_agent",
		TargetType:  "agent",
		TargetID:    "agent-1",
		Details:     `{"name":"test-agent"}`,
	}
	if err := repo.CreateActivityEntry(ctx, entry); err != nil {
		t.Fatalf("create: %v", err)
	}

	entries, err := repo.ListActivityEntries(ctx, "ws-1", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("count = %d, want 1", len(entries))
	}
	if entries[0].Action != "created_agent" {
		t.Errorf("action = %q, want %q", entries[0].Action, "created_agent")
	}
}
