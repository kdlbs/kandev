package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/configloader"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

func newTestServiceWithConfig(t *testing.T) (*service.Service, string) {
	t.Helper()
	svc := newTestService(t)
	tmpDir := t.TempDir()
	loader := configloader.NewConfigLoader(tmpDir)
	if err := loader.EnsureDefaultWorkspace(); err != nil {
		t.Fatalf("ensure default workspace: %v", err)
	}
	writer := configloader.NewFileWriter(tmpDir, loader)
	svc.SetConfigLoader(loader, writer)
	return svc, tmpDir
}

func TestListAgentsFromConfig(t *testing.T) {
	svc, tmpDir := newTestServiceWithConfig(t)
	agentsDir := filepath.Join(tmpDir, "workspaces", "default", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "test-agent.yml"), []byte("name: test-agent\nrole: worker\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := svc.ConfigLoader().Reload("default"); err != nil {
		t.Fatalf("reload: %v", err)
	}
	agents, err := svc.ListAgentsFromConfig(context.Background(), "any")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(agents) != 1 || agents[0].Name != "test-agent" {
		t.Errorf("got %d agents, want 1 named test-agent", len(agents))
	}
}

func TestListAgentsFromConfig_NoLoader(t *testing.T) {
	svc := newTestService(t) // no config loader
	_, err := svc.ListAgentsFromConfig(context.Background(), "any")
	if err == nil {
		t.Error("expected error when config loader not initialized")
	}
}

func TestListSkillsFromConfig(t *testing.T) {
	svc, tmpDir := newTestServiceWithConfig(t)
	skillDir := filepath.Join(tmpDir, "workspaces", "default", "skills", "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# My Skill"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := svc.ConfigLoader().Reload("default"); err != nil {
		t.Fatalf("reload: %v", err)
	}
	skills, err := svc.ListSkillsFromConfig(context.Background(), "any")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(skills) != 1 || skills[0].Slug != "my-skill" {
		t.Errorf("got %d skills, want 1 with slug my-skill", len(skills))
	}
}

func TestListRoutinesFromConfig(t *testing.T) {
	svc, tmpDir := newTestServiceWithConfig(t)
	routinesDir := filepath.Join(tmpDir, "workspaces", "default", "routines")
	if err := os.MkdirAll(routinesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(routinesDir, "daily.yml"), []byte("name: daily\ndescription: daily\ntask_template: run\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := svc.ConfigLoader().Reload("default"); err != nil {
		t.Fatalf("reload: %v", err)
	}
	routines, err := svc.ListRoutinesFromConfig(context.Background(), "any")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(routines) != 1 || routines[0].Name != "daily" {
		t.Errorf("got %d routines, want 1 named daily", len(routines))
	}
}

func TestGetAgentFromConfig_ByName(t *testing.T) {
	svc, tmpDir := newTestServiceWithConfig(t)
	agentsDir := filepath.Join(tmpDir, "workspaces", "default", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "lookup.yml"), []byte("name: lookup\nrole: worker\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := svc.ConfigLoader().Reload("default"); err != nil {
		t.Fatalf("reload: %v", err)
	}
	agent, err := svc.GetAgentFromConfig(context.Background(), "lookup")
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
