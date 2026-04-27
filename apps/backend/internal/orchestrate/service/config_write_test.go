package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/configloader"
	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

// newTestServiceWithWriter creates a service backed by an in-memory DB
// and a real filesystem config writer rooted at a temp directory.
func newTestServiceWithWriter(t *testing.T) (*service.Service, string) {
	t.Helper()
	svc := newTestService(t)

	base := t.TempDir()
	wsDir := filepath.Join(base, "workspaces", "default")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, "kandev.yml"), []byte("name: default\n"), 0o644); err != nil {
		t.Fatalf("write kandev.yml: %v", err)
	}

	loader := configloader.NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("load config: %v", err)
	}
	writer := configloader.NewFileWriter(base, loader)
	svc.SetConfigLoader(loader, writer)

	return svc, base
}

func TestCreateAgent_WritesToFilesystem(t *testing.T) {
	svc, base := newTestServiceWithWriter(t)
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "fs-agent",
		Role:        models.AgentRoleWorker,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create: %v", err)
	}

	filePath := filepath.Join(base, "workspaces", "default", "agents", "fs-agent.yml")
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("agent file not created: %v", err)
	}
}

func TestUpdateAgent_WritesToFilesystem(t *testing.T) {
	svc, base := newTestServiceWithWriter(t)
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "updatable",
		Role:        models.AgentRoleWorker,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create: %v", err)
	}

	agent.BudgetMonthlyCents = 9999
	if err := svc.UpdateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("update: %v", err)
	}

	filePath := filepath.Join(base, "workspaces", "default", "agents", "updatable.yml")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read agent file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("agent file is empty after update")
	}
}

func TestDeleteAgent_RemovesFromFilesystem(t *testing.T) {
	svc, base := newTestServiceWithWriter(t)
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "deletable",
		Role:        models.AgentRoleWorker,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create: %v", err)
	}

	filePath := filepath.Join(base, "workspaces", "default", "agents", "deletable.yml")
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("file should exist before delete: %v", err)
	}

	if err := svc.DeleteAgentInstance(ctx, agent.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatal("agent file should have been deleted")
	}
}

func TestCreateProject_WritesToFilesystem(t *testing.T) {
	svc, base := newTestServiceWithWriter(t)
	ctx := context.Background()

	project := &models.Project{
		WorkspaceID: "ws-1",
		Name:        "fs-project",
		Description: "test project",
	}
	if err := svc.CreateProject(ctx, project); err != nil {
		t.Fatalf("create: %v", err)
	}

	filePath := filepath.Join(base, "workspaces", "default", "projects", "fs-project.yml")
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("project file not created: %v", err)
	}
}

func TestUpdateProject_WritesToFilesystem(t *testing.T) {
	svc, base := newTestServiceWithWriter(t)
	ctx := context.Background()

	project := &models.Project{
		WorkspaceID: "ws-1",
		Name:        "updatable-proj",
		Description: "original",
	}
	if err := svc.CreateProject(ctx, project); err != nil {
		t.Fatalf("create: %v", err)
	}

	project.Description = "updated description"
	if err := svc.UpdateProject(ctx, project); err != nil {
		t.Fatalf("update: %v", err)
	}

	filePath := filepath.Join(base, "workspaces", "default", "projects", "updatable-proj.yml")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read project file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("project file is empty after update")
	}
}

func TestDeleteProject_RemovesFromFilesystem(t *testing.T) {
	svc, base := newTestServiceWithWriter(t)
	ctx := context.Background()

	project := &models.Project{
		WorkspaceID: "ws-1",
		Name:        "deletable-proj",
	}
	if err := svc.CreateProject(ctx, project); err != nil {
		t.Fatalf("create: %v", err)
	}

	filePath := filepath.Join(base, "workspaces", "default", "projects", "deletable-proj.yml")
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("file should exist before delete: %v", err)
	}

	if err := svc.DeleteProject(ctx, project.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatal("project file should have been deleted")
	}
}

func TestCreateRoutine_WritesToFilesystem(t *testing.T) {
	svc, base := newTestServiceWithWriter(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID: "ws-1",
		Name:        "fs-routine",
		Description: "test routine",
		Status:      "active",
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create: %v", err)
	}

	filePath := filepath.Join(base, "workspaces", "default", "routines", "fs-routine.yml")
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("routine file not created: %v", err)
	}
}

func TestDeleteRoutine_RemovesFromFilesystem(t *testing.T) {
	svc, base := newTestServiceWithWriter(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID: "ws-1",
		Name:        "deletable-routine",
		Status:      "active",
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create: %v", err)
	}

	filePath := filepath.Join(base, "workspaces", "default", "routines", "deletable-routine.yml")
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("file should exist before delete: %v", err)
	}

	if err := svc.DeleteRoutine(ctx, routine.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatal("routine file should have been deleted")
	}
}

func TestUpdateSkill_WritesToFilesystem(t *testing.T) {
	svc, base := newTestServiceWithWriter(t)
	ctx := context.Background()

	skill := &models.Skill{
		WorkspaceID: "ws-1",
		Name:        "My Skill",
		Slug:        "my-skill",
		SourceType:  "inline",
		Content:     "# Original",
	}
	if err := svc.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create: %v", err)
	}

	skill.Content = "# Updated content"
	if err := svc.UpdateSkill(ctx, skill); err != nil {
		t.Fatalf("update: %v", err)
	}

	filePath := filepath.Join(base, "workspaces", "default", "skills", "my-skill", "SKILL.md")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read skill file: %v", err)
	}
	if string(data) != "# Updated content" {
		t.Errorf("content = %q, want %q", string(data), "# Updated content")
	}
}

func TestDeleteSkill_RemovesFromFilesystem(t *testing.T) {
	svc, base := newTestServiceWithWriter(t)
	ctx := context.Background()

	skill := &models.Skill{
		WorkspaceID: "ws-1",
		Name:        "Removable",
		Slug:        "removable",
		SourceType:  "inline",
		Content:     "# Remove me",
	}
	if err := svc.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create: %v", err)
	}

	dirPath := filepath.Join(base, "workspaces", "default", "skills", "removable")
	if _, err := os.Stat(dirPath); err != nil {
		t.Fatalf("skill dir should exist before delete: %v", err)
	}

	if err := svc.DeleteSkill(ctx, skill.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := os.Stat(dirPath); !os.IsNotExist(err) {
		t.Fatal("skill directory should have been deleted")
	}
}
