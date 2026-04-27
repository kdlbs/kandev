package configloader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

func TestWriteAndReadAgent(t *testing.T) {
	base := setupTestWorkspace(t)
	loader := NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	writer := NewFileWriter(base, loader)

	agent := &models.AgentInstance{
		Name:               "new-agent",
		Role:               models.AgentRoleSpecialist,
		BudgetMonthlyCents: 7500,
	}

	if err := writer.WriteAgent("default", agent); err != nil {
		t.Fatalf("WriteAgent() error: %v", err)
	}

	// Verify the agent is in the reloaded cache.
	agents := loader.GetAgents("default")
	found := false
	for _, a := range agents {
		if a.Name == "new-agent" {
			found = true
			if a.BudgetMonthlyCents != 7500 {
				t.Errorf("budget = %d, want 7500", a.BudgetMonthlyCents)
			}
		}
	}
	if !found {
		t.Error("new-agent not found after WriteAgent")
	}
}

func TestDeleteAgent(t *testing.T) {
	base := setupTestWorkspace(t)
	loader := NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	writer := NewFileWriter(base, loader)

	if err := writer.DeleteAgent("default", "ceo"); err != nil {
		t.Fatalf("DeleteAgent() error: %v", err)
	}

	agents := loader.GetAgents("default")
	for _, a := range agents {
		if a.Name == "ceo" {
			t.Error("ceo should have been deleted")
		}
	}
}

func TestWriteAndDeleteSkill(t *testing.T) {
	base := setupTestWorkspace(t)
	loader := NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	writer := NewFileWriter(base, loader)

	// Write a new skill.
	if err := writer.WriteSkill("default", "deploy", "# Deploy\nRun deployments."); err != nil {
		t.Fatalf("WriteSkill() error: %v", err)
	}

	skills := loader.GetSkills("default")
	found := false
	for _, s := range skills {
		if s.Slug == "deploy" {
			found = true
			if s.Content != "# Deploy\nRun deployments." {
				t.Errorf("unexpected content: %q", s.Content)
			}
		}
	}
	if !found {
		t.Error("deploy skill not found after WriteSkill")
	}

	// Delete the skill.
	if err := writer.DeleteSkill("default", "deploy"); err != nil {
		t.Fatalf("DeleteSkill() error: %v", err)
	}

	skills = loader.GetSkills("default")
	for _, s := range skills {
		if s.Slug == "deploy" {
			t.Error("deploy skill should have been deleted")
		}
	}
}

func TestWriteAndDeleteProject(t *testing.T) {
	base := setupTestWorkspace(t)
	loader := NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	writer := NewFileWriter(base, loader)

	project := &models.Project{
		Name:        "new-project",
		Description: "A new project",
		Status:      models.ProjectStatusActive,
	}
	if err := writer.WriteProject("default", project); err != nil {
		t.Fatalf("WriteProject() error: %v", err)
	}

	projects := loader.GetProjects("default")
	found := false
	for _, p := range projects {
		if p.Name == "new-project" {
			found = true
		}
	}
	if !found {
		t.Error("new-project not found after WriteProject")
	}

	if err := writer.DeleteProject("default", "new-project"); err != nil {
		t.Fatalf("DeleteProject() error: %v", err)
	}

	projects = loader.GetProjects("default")
	for _, p := range projects {
		if p.Name == "new-project" {
			t.Error("new-project should have been deleted")
		}
	}
}

func TestWriteAndDeleteRoutine(t *testing.T) {
	base := setupTestWorkspace(t)
	loader := NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	writer := NewFileWriter(base, loader)

	routine := &models.Routine{
		Name:              "weekly-report",
		Description:       "Weekly report routine",
		Status:            "active",
		ConcurrencyPolicy: "queue",
	}
	if err := writer.WriteRoutine("default", routine); err != nil {
		t.Fatalf("WriteRoutine() error: %v", err)
	}

	routines := loader.GetRoutines("default")
	found := false
	for _, r := range routines {
		if r.Name == "weekly-report" {
			found = true
		}
	}
	if !found {
		t.Error("weekly-report not found after WriteRoutine")
	}

	if err := writer.DeleteRoutine("default", "weekly-report"); err != nil {
		t.Fatalf("DeleteRoutine() error: %v", err)
	}

	routines = loader.GetRoutines("default")
	for _, r := range routines {
		if r.Name == "weekly-report" {
			t.Error("weekly-report should have been deleted")
		}
	}
}

func TestDeleteNonexistentAgent(t *testing.T) {
	base := setupTestWorkspace(t)
	loader := NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	writer := NewFileWriter(base, loader)

	// Should not error when deleting a file that doesn't exist.
	if err := writer.DeleteAgent("default", "nonexistent"); err != nil {
		t.Fatalf("DeleteAgent() should not error for missing file: %v", err)
	}
}

func TestWriteAgentCreatesDirectory(t *testing.T) {
	base := t.TempDir()
	// Create workspace dir with kandev.yml but no agents subdir.
	wsDir := filepath.Join(base, "workspaces", "new-ws")
	mkdirAll(t, wsDir)
	writeFile(t, filepath.Join(wsDir, "kandev.yml"), "name: new-ws\n")

	loader := NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	writer := NewFileWriter(base, loader)
	agent := &models.AgentInstance{Name: "test-agent", Role: models.AgentRoleWorker}
	if err := writer.WriteAgent("new-ws", agent); err != nil {
		t.Fatalf("WriteAgent() error: %v", err)
	}

	// Verify file was created.
	filePath := filepath.Join(base, "workspaces", "new-ws", "agents", "test-agent.yml")
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("agent file not created: %v", err)
	}
}
