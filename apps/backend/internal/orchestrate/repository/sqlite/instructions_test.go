package sqlite_test

import (
	"context"
	"testing"
)

func TestInstructions_UpsertAndList(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.UpsertInstruction(ctx, "agent-1", "AGENTS.md", "# CEO", true); err != nil {
		t.Fatalf("upsert AGENTS.md: %v", err)
	}
	if err := repo.UpsertInstruction(ctx, "agent-1", "HEARTBEAT.md", "# Checklist", false); err != nil {
		t.Fatalf("upsert HEARTBEAT.md: %v", err)
	}

	files, err := repo.ListInstructions(ctx, "agent-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("count = %d, want 2", len(files))
	}
	// Entry file (is_entry=1) should sort first.
	if files[0].Filename != "AGENTS.md" {
		t.Errorf("first file = %q, want AGENTS.md", files[0].Filename)
	}
	if !files[0].IsEntry {
		t.Error("AGENTS.md should have is_entry=true")
	}
}

func TestInstructions_Get(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.UpsertInstruction(ctx, "agent-1", "AGENTS.md", "# Agent", true); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	file, err := repo.GetInstruction(ctx, "agent-1", "AGENTS.md")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if file.Content != "# Agent" {
		t.Errorf("content = %q, want %q", file.Content, "# Agent")
	}
	if file.AgentInstanceID != "agent-1" {
		t.Errorf("agent_instance_id = %q", file.AgentInstanceID)
	}
}

func TestInstructions_GetNotFound(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	_, err := repo.GetInstruction(ctx, "agent-1", "nonexistent.md")
	if err == nil {
		t.Fatal("expected error for missing instruction")
	}
}

func TestInstructions_UpsertUpdates(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.UpsertInstruction(ctx, "agent-1", "AGENTS.md", "v1", true); err != nil {
		t.Fatalf("upsert v1: %v", err)
	}
	if err := repo.UpsertInstruction(ctx, "agent-1", "AGENTS.md", "v2", true); err != nil {
		t.Fatalf("upsert v2: %v", err)
	}

	files, err := repo.ListInstructions(ctx, "agent-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("count = %d, want 1 after upsert", len(files))
	}
	if files[0].Content != "v2" {
		t.Errorf("content = %q, want v2", files[0].Content)
	}
}

func TestInstructions_Delete(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.UpsertInstruction(ctx, "agent-1", "AGENTS.md", "content", true); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := repo.DeleteInstruction(ctx, "agent-1", "AGENTS.md"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	files, err := repo.ListInstructions(ctx, "agent-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("count after delete = %d, want 0", len(files))
	}
}

func TestInstructions_IsolatedByAgent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.UpsertInstruction(ctx, "agent-1", "AGENTS.md", "agent 1", true); err != nil {
		t.Fatalf("upsert agent-1: %v", err)
	}
	if err := repo.UpsertInstruction(ctx, "agent-2", "AGENTS.md", "agent 2", true); err != nil {
		t.Fatalf("upsert agent-2: %v", err)
	}

	files1, _ := repo.ListInstructions(ctx, "agent-1")
	files2, _ := repo.ListInstructions(ctx, "agent-2")

	if len(files1) != 1 || len(files2) != 1 {
		t.Fatalf("expected 1 file per agent, got %d and %d", len(files1), len(files2))
	}
	if files1[0].Content != "agent 1" {
		t.Errorf("agent-1 content = %q", files1[0].Content)
	}
	if files2[0].Content != "agent 2" {
		t.Errorf("agent-2 content = %q", files2[0].Content)
	}
}
