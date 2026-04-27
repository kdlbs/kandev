package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

func TestMaterializeSkills_Inline(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	skill := &models.Skill{
		WorkspaceID: "ws-1",
		Name:        "Test Inline",
		Slug:        "test-inline",
		SourceType:  "inline",
		Content:     "# Test\nDo the thing.",
	}
	if err := svc.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create: %v", err)
	}

	cacheDir := t.TempDir()
	dirs, err := svc.MaterializeSkills(ctx, []string{skill.ID}, cacheDir)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if len(dirs) != 1 {
		t.Fatalf("dirs count = %d, want 1", len(dirs))
	}
	if dirs[0].Slug != "test-inline" {
		t.Errorf("slug = %q, want %q", dirs[0].Slug, "test-inline")
	}

	content, err := os.ReadFile(filepath.Join(dirs[0].Path, "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if string(content) != "# Test\nDo the thing." {
		t.Errorf("content = %q", string(content))
	}
}

func TestMaterializeSkills_LocalPath(t *testing.T) {
	// local_path skills are stored as filesystem skills after creation.
	// Materialization treats them as inline (writing content to cache dir).
	svc := newTestService(t)
	ctx := context.Background()

	skill := &models.Skill{
		WorkspaceID: "ws-1",
		Name:        "Local Skill",
		Slug:        "local-skill",
		SourceType:  "inline",
		Content:     "# Local skill content",
	}
	if err := svc.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create: %v", err)
	}

	cacheDir := t.TempDir()
	dirs, err := svc.MaterializeSkills(ctx, []string{skill.ID}, cacheDir)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if len(dirs) != 1 {
		t.Fatalf("dirs count = %d, want 1", len(dirs))
	}
	if dirs[0].Slug != "local-skill" {
		t.Errorf("slug = %q, want %q", dirs[0].Slug, "local-skill")
	}
}

func TestMaterializeSkills_NonexistentSkillID(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	cacheDir := t.TempDir()
	dirs, err := svc.MaterializeSkills(ctx, []string{"nonexistent-skill"}, cacheDir)
	if err != nil {
		t.Fatalf("materialize should not fail overall: %v", err)
	}
	if len(dirs) != 0 {
		t.Errorf("dirs count = %d, want 0 (nonexistent skill should be skipped)", len(dirs))
	}
}

func TestSymlinkSkills(t *testing.T) {
	agentHome := t.TempDir()
	skillDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("test"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	dirs := []service.SkillDir{
		{Slug: "my-skill", Path: skillDir},
	}
	if err := service.SymlinkSkills(agentHome, dirs); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	link := filepath.Join(agentHome, ".claude", "skills", "my-skill")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != skillDir {
		t.Errorf("symlink target = %q, want %q", target, skillDir)
	}
}

func TestCleanupSymlinks(t *testing.T) {
	agentHome := t.TempDir()
	skillsDir := filepath.Join(agentHome, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	target := t.TempDir()
	link := filepath.Join(skillsDir, "test-skill")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if err := service.CleanupSymlinks(agentHome, []string{"test-skill"}); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("symlink should be removed, got err: %v", err)
	}
}

func TestCleanupSymlinks_NonexistentIsNoop(t *testing.T) {
	agentHome := t.TempDir()
	skillsDir := filepath.Join(agentHome, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := service.CleanupSymlinks(agentHome, []string{"nonexistent"}); err != nil {
		t.Fatalf("cleanup nonexistent should not error: %v", err)
	}
}
