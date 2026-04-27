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

func TestListAgentsFromConfig(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	if err := svc.CreateAgentInstance(ctx, &models.AgentInstance{
		Name: "test-agent", Role: models.AgentRoleWorker, WorkspaceID: "default",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	agents, err := svc.ListAgentsFromConfig(ctx, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(agents) != 1 || agents[0].Name != "test-agent" {
		t.Errorf("got %d agents, want 1 named test-agent", len(agents))
	}
}

func TestListSkillsFromConfig(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	if err := svc.CreateSkill(ctx, &models.Skill{
		Name: "My Skill", Slug: "my-skill", WorkspaceID: "default", Content: "# My Skill",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	skills, err := svc.ListSkillsFromConfig(ctx, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(skills) != 1 || skills[0].Slug != "my-skill" {
		t.Errorf("got %d skills, want 1 with slug my-skill", len(skills))
	}
}

func TestListRoutinesFromConfig(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	if err := svc.CreateRoutine(ctx, &models.Routine{
		ID: "routine-daily", Name: "daily", WorkspaceID: "default",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	routines, err := svc.ListRoutinesFromConfig(ctx, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(routines) != 1 || routines[0].Name != "daily" {
		t.Errorf("got %d routines, want 1 named daily", len(routines))
	}
}

func TestGetAgentFromConfig_ByName(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	if err := svc.CreateAgentInstance(ctx, &models.AgentInstance{
		Name: "lookup", Role: models.AgentRoleWorker, WorkspaceID: "default",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	agent, err := svc.GetAgentFromConfig(ctx, "lookup")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if agent.Name != "lookup" {
		t.Errorf("name = %q, want lookup", agent.Name)
	}
}

func TestMemoryFilesystem(t *testing.T) {
	svc, tmpDir := newTestServiceWithConfig(t)

	if err := svc.UpsertMemoryFromConfig("test-agent", "knowledge", "greeting", "Hello"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	memPath := filepath.Join(tmpDir, "workspaces", "default", "agents", "test-agent", "memory", "knowledge", "greeting.md")
	if _, err := os.Stat(memPath); err != nil {
		t.Fatalf("file not created: %v", err)
	}
	content, err := svc.GetMemoryFromConfig("test-agent", "knowledge", "greeting")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if content != "Hello" {
		t.Errorf("content = %q, want Hello", content)
	}
	entries, err := svc.ListMemoryFromConfig("test-agent", "knowledge")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if err := svc.DeleteMemoryFromConfig("test-agent", "knowledge", "greeting"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(memPath); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}
}

// newTestServiceWithConfig wires a service with a filesystem ConfigLoader+FileWriter.
// Only used by tests that exercise the memory-on-FS path.
func newTestServiceWithConfig(t *testing.T) (*service.Service, string) {
	t.Helper()
	s := newTestService(t)
	tmpDir := t.TempDir()
	loader := configloader.NewConfigLoader(tmpDir)
	if err := loader.EnsureDefaultWorkspace(); err != nil {
		t.Fatalf("ensure default workspace: %v", err)
	}
	writer := configloader.NewFileWriter(tmpDir, loader)
	s.SetConfigLoader(loader, writer)
	return s, tmpDir
}
