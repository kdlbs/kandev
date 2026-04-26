package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAgentSkillDirs_Claude(t *testing.T) {
	dirs := agentSkillDirs("claude-acp")
	if len(dirs) != 2 {
		t.Fatalf("expected 2 dirs, got %d", len(dirs))
	}
}

func TestAgentSkillDirs_Unknown(t *testing.T) {
	dirs := agentSkillDirs("unknown-agent")
	if len(dirs) != 0 {
		t.Fatalf("expected 0 dirs, got %d", len(dirs))
	}
}

func TestParseDesiredSlugs_JSON(t *testing.T) {
	slugs := ParseDesiredSlugs(`["code-review","memory"]`)
	if len(slugs) != 2 || slugs[0] != "code-review" || slugs[1] != "memory" {
		t.Errorf("unexpected slugs: %v", slugs)
	}
}

func TestParseDesiredSlugs_Comma(t *testing.T) {
	slugs := ParseDesiredSlugs("code-review,memory")
	if len(slugs) != 2 {
		t.Errorf("unexpected slugs: %v", slugs)
	}
}

func TestParseDesiredSlugs_Empty(t *testing.T) {
	if slugs := ParseDesiredSlugs(""); slugs != nil {
		t.Errorf("expected nil, got %v", slugs)
	}
	if slugs := ParseDesiredSlugs("[]"); slugs != nil {
		t.Errorf("expected nil, got %v", slugs)
	}
}

func TestSymlinkSkillsSafe_Creates(t *testing.T) {
	targetDir := t.TempDir()
	sourceDir := t.TempDir()
	skillPath := filepath.Join(sourceDir, "test-skill")
	os.MkdirAll(skillPath, 0o755)

	err := symlinkSkillsSafe(targetDir, []SkillDir{{Slug: "test-skill", Path: skillPath}}, sourceDir)
	if err != nil {
		t.Fatalf("symlink: %v", err)
	}
	target, err := os.Readlink(filepath.Join(targetDir, "test-skill"))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != skillPath {
		t.Errorf("target = %q, want %q", target, skillPath)
	}
}

func TestSymlinkSkillsSafe_SkipsRealDir(t *testing.T) {
	targetDir := t.TempDir()
	os.MkdirAll(filepath.Join(targetDir, "my-skill"), 0o755)

	err := symlinkSkillsSafe(targetDir, []SkillDir{{Slug: "my-skill", Path: "/other"}}, "/kandev")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	info, _ := os.Lstat(filepath.Join(targetDir, "my-skill"))
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("real dir should not be replaced")
	}
}

func TestSymlinkSkillsSafe_SkipsForeignSymlink(t *testing.T) {
	targetDir := t.TempDir()
	foreignDir := t.TempDir()
	link := filepath.Join(targetDir, "foreign")
	os.Symlink(foreignDir, link)

	err := symlinkSkillsSafe(targetDir, []SkillDir{{Slug: "foreign", Path: "/kandev/x"}}, "/kandev")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	target, _ := os.Readlink(link)
	if target != foreignDir {
		t.Errorf("foreign symlink replaced: %q", target)
	}
}

func TestSymlinkSkillsSafe_ReplacesOwned(t *testing.T) {
	targetDir := t.TempDir()
	kandevBase := t.TempDir()
	oldPath := filepath.Join(kandevBase, "old")
	newPath := filepath.Join(kandevBase, "new")
	os.MkdirAll(oldPath, 0o755)
	os.MkdirAll(newPath, 0o755)

	link := filepath.Join(targetDir, "skill")
	os.Symlink(oldPath, link)

	err := symlinkSkillsSafe(targetDir, []SkillDir{{Slug: "skill", Path: newPath}}, kandevBase)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	target, _ := os.Readlink(link)
	if target != newPath {
		t.Errorf("not replaced: %q", target)
	}
}

func TestCleanupOwned(t *testing.T) {
	dir := t.TempDir()
	kandevBase := t.TempDir()
	foreignDir := t.TempDir()

	ownedLink := filepath.Join(dir, "owned")
	os.Symlink(filepath.Join(kandevBase, "x"), ownedLink)
	foreignLink := filepath.Join(dir, "foreign")
	os.Symlink(foreignDir, foreignLink)

	removeOwnedSymlinksInDir(dir, kandevBase)

	if _, err := os.Lstat(ownedLink); !os.IsNotExist(err) {
		t.Error("owned should be removed")
	}
	if _, err := os.Lstat(foreignLink); err != nil {
		t.Error("foreign should remain")
	}
}

func TestCleanDangling(t *testing.T) {
	dir := t.TempDir()
	kandevBase := t.TempDir()

	danglingLink := filepath.Join(dir, "dangling")
	os.Symlink(filepath.Join(kandevBase, "deleted"), danglingLink)

	validTarget := filepath.Join(kandevBase, "valid")
	os.MkdirAll(validTarget, 0o755)
	validLink := filepath.Join(dir, "valid")
	os.Symlink(validTarget, validLink)

	cleanDanglingInDir(dir, kandevBase)

	if _, err := os.Lstat(danglingLink); !os.IsNotExist(err) {
		t.Error("dangling should be removed")
	}
	if _, err := os.Lstat(validLink); err != nil {
		t.Error("valid should remain")
	}
}
