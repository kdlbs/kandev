package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ensureGit creates a minimal .git directory structure.
func ensureGit(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "info"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestInjectSkills_WritesUnderProjectSkillDir(t *testing.T) {
	worktree := t.TempDir()
	ensureGit(t, worktree)

	skills := []Skill{
		{Slug: "code-review", Content: "# Code Review"},
		{Slug: "memory", Content: "# Memory"},
	}
	if err := injectSkills(worktree, ".claude/skills", skills); err != nil {
		t.Fatalf("injectSkills: %v", err)
	}

	for _, slug := range []string{"code-review", "memory"} {
		path := filepath.Join(worktree, ".claude", "skills", "kandev-"+slug, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("missing %s: %v", path, err)
		}
	}
}

func TestInjectSkills_CleanSlateRemovesPreviousKandevDirs(t *testing.T) {
	worktree := t.TempDir()
	ensureGit(t, worktree)

	if err := injectSkills(worktree, ".agents/skills", []Skill{
		{Slug: "skill-a", Content: "# A"},
		{Slug: "skill-b", Content: "# B"},
	}); err != nil {
		t.Fatalf("first inject: %v", err)
	}

	// Re-inject with only A — B's directory must be gone.
	if err := injectSkills(worktree, ".agents/skills", []Skill{
		{Slug: "skill-a", Content: "# A"},
	}); err != nil {
		t.Fatalf("second inject: %v", err)
	}

	skillsDir := filepath.Join(worktree, ".agents", "skills")
	if _, err := os.Stat(filepath.Join(skillsDir, "kandev-skill-b")); !os.IsNotExist(err) {
		t.Errorf("deassigned kandev-skill-b should be removed")
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "kandev-skill-a", "SKILL.md")); err != nil {
		t.Errorf("still-assigned kandev-skill-a should exist: %v", err)
	}
}

func TestInjectSkills_PreservesUserSkills(t *testing.T) {
	worktree := t.TempDir()
	ensureGit(t, worktree)
	userSkill := filepath.Join(worktree, ".claude", "skills", "team-skill")
	if err := os.MkdirAll(userSkill, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := injectSkills(worktree, ".claude/skills", []Skill{
		{Slug: "code-review", Content: "# CR"},
	}); err != nil {
		t.Fatalf("injectSkills: %v", err)
	}

	if _, err := os.Stat(userSkill); err != nil {
		t.Errorf("user-managed skill should be preserved: %v", err)
	}
}

func TestInjectSkills_SkipsInvalidSlug(t *testing.T) {
	worktree := t.TempDir()
	ensureGit(t, worktree)

	if err := injectSkills(worktree, ".agents/skills", []Skill{
		{Slug: "../escape", Content: "evil"},
		{Slug: "ok-slug", Content: "# ok"},
	}); err != nil {
		t.Fatalf("injectSkills: %v", err)
	}
	if _, err := os.Stat(filepath.Join(worktree, ".agents", "skills", "kandev-ok-slug", "SKILL.md")); err != nil {
		t.Errorf("valid slug should land: %v", err)
	}
	// The invalid slug must not have produced any directory.
	entries, _ := os.ReadDir(filepath.Join(worktree, ".agents", "skills"))
	for _, e := range entries {
		if strings.Contains(e.Name(), "escape") {
			t.Errorf("invalid slug %q wrote a directory", e.Name())
		}
	}
}

func TestEnsureGitExclude_AppendsPatternIdempotent(t *testing.T) {
	worktree := t.TempDir()
	ensureGit(t, worktree)

	for i := 0; i < 3; i++ {
		if err := ensureGitExclude(worktree, ".claude/skills"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(worktree, ".git", "info", "exclude"))
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	pattern := ".claude/skills/kandev-*"
	if got := strings.Count(string(data), pattern); got != 1 {
		t.Errorf("pattern appears %d times, want 1; got %q", got, string(data))
	}
}

func TestEnsureGitExclude_LinkedWorktreeGitFile(t *testing.T) {
	mainGitDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(mainGitDir, "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	worktree := t.TempDir()
	gitFile := filepath.Join(worktree, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: "+mainGitDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ensureGitExclude(worktree, ".agents/skills"); err != nil {
		t.Fatalf("ensureGitExclude: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(mainGitDir, "info", "exclude"))
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	if !strings.Contains(string(data), ".agents/skills/kandev-*") {
		t.Errorf("pattern not appended: %q", string(data))
	}
}

func TestInjectSkills_NoGitDirIsNotFatal(t *testing.T) {
	worktree := t.TempDir() // no .git
	if err := injectSkills(worktree, ".agents/skills", []Skill{
		{Slug: "ok", Content: "# ok"},
	}); err != nil {
		t.Fatalf("injectSkills: %v", err)
	}
	if _, err := os.Stat(filepath.Join(worktree, ".agents", "skills", "kandev-ok", "SKILL.md")); err != nil {
		t.Errorf("skill should still land without .git: %v", err)
	}
}
