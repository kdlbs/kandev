package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

func TestExportInstructionsToDir_WritesFiles(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Create an agent (CreateAgentInstance creates default instructions).
	agent := makeAgent("worker-export", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Update the default AGENTS.md with known content for verification.
	svc.ExecSQL(t, `UPDATE orchestrate_agent_instructions SET content = ?
		WHERE agent_instance_id = ? AND filename = ?`,
		"# You are a worker agent.", agent.ID, "AGENTS.md")

	// Add an extra instruction file.
	svc.ExecSQL(t, `INSERT INTO orchestrate_agent_instructions
		(id, agent_instance_id, filename, content, is_entry, created_at, updated_at)
		VALUES (?, ?, ?, ?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"inst-extra", agent.ID, "CUSTOM.md", "## Custom instructions")

	// Export.
	targetDir := filepath.Join(t.TempDir(), "instructions", agent.ID)
	if err := svc.ExportInstructionsToDir(ctx, agent.ID, targetDir); err != nil {
		t.Fatalf("ExportInstructionsToDir: %v", err)
	}

	// Verify AGENTS.md.
	agentsMd, err := os.ReadFile(filepath.Join(targetDir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if string(agentsMd) != "# You are a worker agent." {
		t.Errorf("AGENTS.md content = %q, want %q", string(agentsMd), "# You are a worker agent.")
	}

	// Verify CUSTOM.md.
	custom, err := os.ReadFile(filepath.Join(targetDir, "CUSTOM.md"))
	if err != nil {
		t.Fatalf("read CUSTOM.md: %v", err)
	}
	if string(custom) != "## Custom instructions" {
		t.Errorf("CUSTOM.md content = %q", string(custom))
	}
}

func TestExportInstructionsToDir_NoFiles(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	targetDir := filepath.Join(t.TempDir(), "empty")
	if err := svc.ExportInstructionsToDir(ctx, "nonexistent-agent", targetDir); err != nil {
		t.Fatalf("ExportInstructionsToDir: %v", err)
	}

	// Directory should not be created when there are no files.
	if _, err := os.Stat(targetDir); !os.IsNotExist(err) {
		t.Error("directory should not exist when no instruction files are present")
	}
}

func TestPrepareRuntime_ExportsInstructionsAndSkills(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-runtime", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Update the default AGENTS.md so we can verify the export content.
	svc.ExecSQL(t, `UPDATE orchestrate_agent_instructions SET content = ?
		WHERE agent_instance_id = ? AND filename = ?`,
		"# Runtime test", agent.ID, "AGENTS.md")

	si := service.NewSchedulerIntegration(svc, 0)
	dir, err := service.PrepareRuntimeForTest(si, ctx, agent, "default")
	if err != nil {
		t.Fatalf("prepareRuntime: %v", err)
	}

	if dir == "" {
		t.Fatal("expected non-empty instructions dir")
	}

	data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md from runtime dir: %v", err)
	}
	if string(data) != "# Runtime test" {
		t.Errorf("AGENTS.md = %q, want %q", string(data), "# Runtime test")
	}
}
