package configloader

import (
	"os"
	"path/filepath"
	"testing"
)

// setupTestWorkspace creates a temporary directory structure mimicking ~/.kandev.
func setupTestWorkspace(t *testing.T) string {
	t.Helper()
	base := t.TempDir()

	// Create default workspace with kandev.yml
	wsDir := filepath.Join(base, "workspaces", "default")
	mkdirAll(t, wsDir)
	writeFile(t, filepath.Join(wsDir, "kandev.yml"), `
name: default
description: Test workspace
task_prefix: TST
`)

	// Create agents
	mkdirAll(t, filepath.Join(wsDir, "agents"))
	writeFile(t, filepath.Join(wsDir, "agents", "ceo.yml"), `
name: ceo
role: ceo
icon: crown
budget_monthly_cents: 10000
max_concurrent_sessions: 2
desired_skills: code-review,memory
`)
	writeFile(t, filepath.Join(wsDir, "agents", "worker.yml"), `
name: worker
role: worker
reports_to: ceo
budget_monthly_cents: 5000
`)

	// Create skills
	skillDir := filepath.Join(wsDir, "skills", "code-review")
	mkdirAll(t, skillDir)
	writeFile(t, filepath.Join(skillDir, "SKILL.md"), "# Code Review\nCheck code for quality.")

	// Create projects
	mkdirAll(t, filepath.Join(wsDir, "projects"))
	writeFile(t, filepath.Join(wsDir, "projects", "api-migration.yml"), `
name: api-migration
description: Migrate v1 to v2
status: active
color: "#ff0000"
budget_cents: 50000
`)

	// Create routines
	mkdirAll(t, filepath.Join(wsDir, "routines"))
	writeFile(t, filepath.Join(wsDir, "routines", "daily-digest.yml"), `
name: daily-digest
description: Send daily digest
task_template: Summarize today's activity
status: active
concurrency_policy: skip
`)

	return base
}

func TestLoadWorkspaces(t *testing.T) {
	base := setupTestWorkspace(t)
	loader := NewConfigLoader(base)

	if err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	workspaces := loader.GetWorkspaces()
	if len(workspaces) != 1 {
		t.Fatalf("got %d workspaces, want 1", len(workspaces))
	}

	ws := workspaces[0]
	if ws.Name != "default" {
		t.Errorf("workspace name = %q, want %q", ws.Name, "default")
	}
	if ws.Settings.TaskPrefix != "TST" {
		t.Errorf("task_prefix = %q, want %q", ws.Settings.TaskPrefix, "TST")
	}
}

func TestLoadAgents(t *testing.T) {
	base := setupTestWorkspace(t)
	loader := NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	agents := loader.GetAgents("default")
	if len(agents) != 2 {
		t.Fatalf("got %d agents, want 2", len(agents))
	}

	var ceo, worker bool
	for _, a := range agents {
		switch a.Name {
		case "ceo":
			ceo = true
			if a.BudgetMonthlyCents != 10000 {
				t.Errorf("ceo budget = %d, want 10000", a.BudgetMonthlyCents)
			}
		case "worker":
			worker = true
			if a.ReportsTo != "ceo" {
				t.Errorf("worker reports_to = %q, want %q", a.ReportsTo, "ceo")
			}
		}
	}
	if !ceo || !worker {
		t.Error("missing ceo or worker agent")
	}
}

func TestLoadSkills(t *testing.T) {
	base := setupTestWorkspace(t)
	loader := NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	skills := loader.GetSkills("default")
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if skills[0].Slug != "code-review" {
		t.Errorf("skill slug = %q, want %q", skills[0].Slug, "code-review")
	}
	if skills[0].Content == "" {
		t.Error("skill content is empty")
	}
}

func TestLoadProjects(t *testing.T) {
	base := setupTestWorkspace(t)
	loader := NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	projects := loader.GetProjects("default")
	if len(projects) != 1 {
		t.Fatalf("got %d projects, want 1", len(projects))
	}
	if projects[0].Name != "api-migration" {
		t.Errorf("project name = %q, want %q", projects[0].Name, "api-migration")
	}
}

func TestLoadRoutines(t *testing.T) {
	base := setupTestWorkspace(t)
	loader := NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	routines := loader.GetRoutines("default")
	if len(routines) != 1 {
		t.Fatalf("got %d routines, want 1", len(routines))
	}
	if routines[0].Name != "daily-digest" {
		t.Errorf("routine name = %q, want %q", routines[0].Name, "daily-digest")
	}
}

func TestGetWorkspaceNotFound(t *testing.T) {
	loader := NewConfigLoader(t.TempDir())
	_, err := loader.GetWorkspace("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent workspace")
	}
}

func TestLoadEmptyDir(t *testing.T) {
	base := t.TempDir()
	// No workspaces dir at all
	loader := NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() should succeed on missing workspaces dir: %v", err)
	}
	if len(loader.GetWorkspaces()) != 0 {
		t.Error("expected zero workspaces")
	}
}

func TestLoadInvalidYAMLRecordsError(t *testing.T) {
	base := t.TempDir()
	wsDir := filepath.Join(base, "workspaces", "broken")
	mkdirAll(t, filepath.Join(wsDir, "agents"))
	writeFile(t, filepath.Join(wsDir, "kandev.yml"), "name: broken\n")
	writeFile(t, filepath.Join(wsDir, "agents", "bad.yml"), ":::invalid yaml")

	loader := NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	errors := loader.GetErrors()
	if len(errors) != 1 {
		t.Fatalf("got %d errors, want 1", len(errors))
	}
	if errors[0].WorkspaceID != "broken" {
		t.Errorf("error workspace = %q, want %q", errors[0].WorkspaceID, "broken")
	}
}

func TestReloadWorkspace(t *testing.T) {
	base := setupTestWorkspace(t)
	loader := NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Add a new agent file
	agentsDir := filepath.Join(base, "workspaces", "default", "agents")
	writeFile(t, filepath.Join(agentsDir, "specialist.yml"), `
name: specialist
role: specialist
budget_monthly_cents: 3000
`)

	if err := loader.Reload("default"); err != nil {
		t.Fatalf("Reload() error: %v", err)
	}

	agents := loader.GetAgents("default")
	if len(agents) != 3 {
		t.Errorf("got %d agents after reload, want 3", len(agents))
	}
}

func TestReloadDeletedWorkspace(t *testing.T) {
	base := setupTestWorkspace(t)
	loader := NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Delete the workspace directory
	wsDir := filepath.Join(base, "workspaces", "default")
	if err := os.RemoveAll(wsDir); err != nil {
		t.Fatalf("remove workspace: %v", err)
	}

	if err := loader.Reload("default"); err != nil {
		t.Fatalf("Reload() error: %v", err)
	}

	if len(loader.GetWorkspaces()) != 0 {
		t.Error("expected zero workspaces after deletion")
	}
}

func TestEnsureDefaultWorkspace(t *testing.T) {
	base := t.TempDir()
	loader := NewConfigLoader(base)

	if err := loader.EnsureDefaultWorkspace(); err != nil {
		t.Fatalf("EnsureDefaultWorkspace() error: %v", err)
	}

	settingsPath := filepath.Join(base, "workspaces", "default", "kandev.yml")
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("kandev.yml not created: %v", err)
	}

	// Calling again should not fail (idempotent).
	if err := loader.EnsureDefaultWorkspace(); err != nil {
		t.Fatalf("second EnsureDefaultWorkspace() error: %v", err)
	}
}

func TestWorkspaceNameFromPath(t *testing.T) {
	loader := NewConfigLoader("/home/user/.kandev")

	tests := []struct {
		path string
		want string
	}{
		{"/home/user/.kandev/workspaces/default/agents/ceo.yml", "default"},
		{"/home/user/.kandev/workspaces/my-team/kandev.yml", "my-team"},
		{"/home/user/.kandev/skills/memory/SKILL.md", ""},
		{"/other/path", ""},
	}
	for _, tt := range tests {
		got := loader.WorkspaceNameFromPath(tt.path)
		if got != tt.want {
			t.Errorf("WorkspaceNameFromPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestGetAgentsNonexistentWorkspace(t *testing.T) {
	loader := NewConfigLoader(t.TempDir())
	agents := loader.GetAgents("nope")
	if agents != nil {
		t.Errorf("expected nil for nonexistent workspace, got %d agents", len(agents))
	}
}

// mkdirAll is a test helper that creates directories.
func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

// writeFile is a test helper that writes content to a file.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
