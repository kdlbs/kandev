package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

func TestCreateDefaultInstructions_CEO(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("ceo-instr", models.AgentRoleCEO)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	files, err := svc.ListInstructions(ctx, agent.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(files) < 2 {
		t.Fatalf("expected at least 2 files for CEO, got %d", len(files))
	}

	// Verify AGENTS.md and HEARTBEAT.md exist.
	found := map[string]bool{}
	for _, f := range files {
		found[f.Filename] = true
	}
	if !found["AGENTS.md"] {
		t.Error("missing AGENTS.md")
	}
	if !found["HEARTBEAT.md"] {
		t.Error("missing HEARTBEAT.md")
	}
}

func TestCreateDefaultInstructions_Worker(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-instr", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	files, err := svc.ListInstructions(ctx, agent.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file for worker, got %d", len(files))
	}
	if files[0].Filename != "AGENTS.md" {
		t.Errorf("filename = %q, want AGENTS.md", files[0].Filename)
	}
	if !files[0].IsEntry {
		t.Error("AGENTS.md should be the entry file")
	}
}

func TestCreateDefaultInstructions_CEOContentIsReal(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("ceo-content", models.AgentRoleCEO)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	file, err := svc.GetInstruction(ctx, agent.ID, "AGENTS.md")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(file.Content) < 100 {
		t.Errorf("CEO AGENTS.md content too short (%d bytes), expected real template", len(file.Content))
	}
}

func TestExportInstructionsToDir(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("export-test", models.AgentRoleCEO)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	targetDir := filepath.Join(t.TempDir(), "instructions", agent.ID)
	if err := svc.ExportInstructionsToDir(ctx, agent.ID, targetDir); err != nil {
		t.Fatalf("export: %v", err)
	}

	// Verify files on disk.
	agentsMd := filepath.Join(targetDir, "AGENTS.md")
	data, err := os.ReadFile(agentsMd)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if len(data) == 0 {
		t.Error("AGENTS.md is empty on disk")
	}

	heartbeatMd := filepath.Join(targetDir, "HEARTBEAT.md")
	if _, err := os.Stat(heartbeatMd); os.IsNotExist(err) {
		t.Error("HEARTBEAT.md not exported to disk")
	}
}

func TestInstructionCRUD(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agentID := "crud-agent"

	// Create.
	if err := svc.UpsertInstruction(ctx, agentID, "CUSTOM.md", "custom content", false); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// Read.
	file, err := svc.GetInstruction(ctx, agentID, "CUSTOM.md")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if file.Content != "custom content" {
		t.Errorf("content = %q", file.Content)
	}

	// Update.
	if err := svc.UpsertInstruction(ctx, agentID, "CUSTOM.md", "updated", false); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	file, _ = svc.GetInstruction(ctx, agentID, "CUSTOM.md")
	if file.Content != "updated" {
		t.Errorf("content after update = %q", file.Content)
	}

	// Delete.
	if err := svc.DeleteInstruction(ctx, agentID, "CUSTOM.md"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	files, _ := svc.ListInstructions(ctx, agentID)
	if len(files) != 0 {
		t.Errorf("count after delete = %d, want 0", len(files))
	}
}

func TestCreateDefaultInstructions_NoRoleTemplates(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Specialist has no embedded templates, should not error.
	agent := makeAgent("specialist-instr", models.AgentRoleSpecialist)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	files, err := svc.ListInstructions(ctx, agent.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files for specialist (no templates), got %d", len(files))
	}
}
