package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

func TestAgentMemory_UpsertAndList(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	mem := &models.AgentMemory{
		AgentInstanceID: "agent-1",
		Layer:           "knowledge",
		Key:             "project-overview",
		Content:         "This is a Go project.",
		Metadata:        "{}",
	}
	if err := repo.UpsertAgentMemory(ctx, mem); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	entries, err := repo.ListAgentMemory(ctx, "agent-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("count = %d, want 1", len(entries))
	}
	if entries[0].Content != "This is a Go project." {
		t.Errorf("content = %q", entries[0].Content)
	}

	// Upsert same key should update
	mem.Content = "Updated content."
	if err := repo.UpsertAgentMemory(ctx, mem); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	entries, _ = repo.ListAgentMemory(ctx, "agent-1")
	if len(entries) != 1 {
		t.Fatalf("count after upsert = %d, want 1", len(entries))
	}
	if entries[0].Content != "Updated content." {
		t.Errorf("updated content = %q", entries[0].Content)
	}

	// Delete
	if err := repo.DeleteAgentMemory(ctx, entries[0].ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	entries, _ = repo.ListAgentMemory(ctx, "agent-1")
	if len(entries) != 0 {
		t.Errorf("count after delete = %d, want 0", len(entries))
	}
}
