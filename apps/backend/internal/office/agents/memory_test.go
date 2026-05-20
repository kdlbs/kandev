package agents_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/agents"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// newTestAgentService creates an AgentService backed by an in-memory SQLite repo.
func newTestAgentService(t *testing.T) *agents.AgentService {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	log := logger.Default()
	return agents.NewAgentService(repo, log, nil)
}

func TestMemory_CRUD(t *testing.T) {
	svc := newTestAgentService(t)
	ctx := context.Background()

	mem := &models.AgentMemory{
		AgentProfileID: "agent-1",
		Layer:          "knowledge",
		Key:            "project-overview",
		Content:        "A Go project.",
		Metadata:       "{}",
	}
	if err := svc.UpsertAgentMemory(ctx, mem); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// List all.
	entries, err := svc.ListMemory(ctx, "agent-1", "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}

	// List by layer.
	entries, err = svc.ListMemory(ctx, "agent-1", "knowledge")
	if err != nil {
		t.Fatalf("list by layer: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries for knowledge, want 1", len(entries))
	}

	entries, err = svc.ListMemory(ctx, "agent-1", "operating")
	if err != nil {
		t.Fatalf("list operating: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries for operating, want 0", len(entries))
	}
}

func TestMemory_Upsert(t *testing.T) {
	svc := newTestAgentService(t)
	ctx := context.Background()

	mem := &models.AgentMemory{
		AgentProfileID: "agent-1",
		Layer:          "knowledge",
		Key:            "fact-1",
		Content:        "original",
		Metadata:       "{}",
	}
	if err := svc.UpsertAgentMemory(ctx, mem); err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	mem.Content = "updated"
	if err := svc.UpsertAgentMemory(ctx, mem); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}
	entries, _ := svc.ListMemory(ctx, "agent-1", "")
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 (upsert should not duplicate)", len(entries))
	}
	if entries[0].Content != "updated" {
		t.Errorf("content = %q, want updated", entries[0].Content)
	}
}

func TestMemory_DeleteAll(t *testing.T) {
	svc := newTestAgentService(t)
	ctx := context.Background()

	for _, layer := range []string{"knowledge", "operating", "session"} {
		mem := &models.AgentMemory{
			AgentProfileID: "agent-1",
			Layer:          layer,
			Key:            "key-" + layer,
			Content:        "content",
			Metadata:       "{}",
		}
		if err := svc.UpsertAgentMemory(ctx, mem); err != nil {
			t.Fatalf("upsert %s: %v", layer, err)
		}
	}

	entries, _ := svc.ListMemory(ctx, "agent-1", "")
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}

	if err := svc.DeleteAllMemory(ctx, "agent-1"); err != nil {
		t.Fatalf("delete all: %v", err)
	}

	entries, _ = svc.ListMemory(ctx, "agent-1", "")
	if len(entries) != 0 {
		t.Errorf("got %d entries after delete all, want 0", len(entries))
	}
}

func TestMemory_Export(t *testing.T) {
	svc := newTestAgentService(t)
	ctx := context.Background()

	for _, key := range []string{"a", "b", "c"} {
		mem := &models.AgentMemory{
			AgentProfileID: "agent-1",
			Layer:          "knowledge",
			Key:            key,
			Content:        key + "-content",
			Metadata:       "{}",
		}
		if err := svc.UpsertAgentMemory(ctx, mem); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}

	entries, err := svc.ExportMemory(ctx, "agent-1")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("exported %d entries, want 3", len(entries))
	}
}

func TestMemory_Summary(t *testing.T) {
	svc := newTestAgentService(t)
	ctx := context.Background()

	// Create operating entries.
	op := &models.AgentMemory{
		AgentProfileID: "agent-1",
		Layer:          "operating",
		Key:            "pref-1",
		Content:        "Use bullet points",
		Metadata:       "{}",
	}
	if err := svc.UpsertAgentMemory(ctx, op); err != nil {
		t.Fatalf("upsert op: %v", err)
	}

	// Create knowledge entries.
	kn := &models.AgentMemory{
		AgentProfileID: "agent-1",
		Layer:          "knowledge",
		Key:            "fact-1",
		Content:        "Go project",
		Metadata:       "{}",
	}
	if err := svc.UpsertAgentMemory(ctx, kn); err != nil {
		t.Fatalf("upsert kn: %v", err)
	}

	summary, err := svc.GetMemorySummary(ctx, "agent-1")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if len(summary) != 2 {
		t.Errorf("summary has %d entries, want 2", len(summary))
	}
}
