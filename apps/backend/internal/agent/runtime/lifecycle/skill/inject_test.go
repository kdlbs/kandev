package skill

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

// ensureGit creates a minimal .git directory structure.
func ensureGit(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "info"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// testLogger returns a logger.Logger discarding output, for tests that
// need to satisfy injectSkills' logger parameter without asserting on
// log content.
func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger.NewLogger: %v", err)
	}
	return log
}

func TestInjectSkills_WritesUnderProjectSkillDir(t *testing.T) {
	worktree := t.TempDir()
	ensureGit(t, worktree)

	skills := []Skill{
		{Slug: "code-review", Content: "# Code Review"},
		{Slug: "memory", Content: "# Memory"},
	}
	if err := injectSkills(worktree, ".claude/skills", skills, testLogger(t)); err != nil {
		t.Fatalf("injectSkills: %v", err)
	}

	for _, slug := range []string{"code-review", "memory"} {
		path := filepath.Join(worktree, ".claude", "skills", "kandev-"+slug, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("missing %s: %v", path, err)
		}
	}
}

func TestInjectSkills_SkipsSupportFilesUnderSymlinkParent(t *testing.T) {
	worktree := t.TempDir()
	ensureGit(t, worktree)
	outside := t.TempDir()

	skillDir := filepath.Join(worktree, ".agents", "skills", "kandev-code-review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(skillDir, "references")); err != nil {
		t.Fatal(err)
	}

	err := writeSkillFiles(skillDir, []SkillFile{{
		Path:    "references/escape.md",
		Content: "outside",
	}})
	if err != nil {
		t.Fatalf("writeSkillFiles: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "escape.md")); !os.IsNotExist(err) {
		t.Fatalf("support file escaped through symlink parent: %v", err)
	}
}

func TestInjectSkills_WritesSupportFilesUnderSymlinkedSkillDir(t *testing.T) {
	realSkillDir := filepath.Join(t.TempDir(), "real", "kandev-code-review")
	if err := os.MkdirAll(realSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	linkParent := filepath.Join(t.TempDir(), "links")
	if err := os.MkdirAll(linkParent, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkSkillDir := filepath.Join(linkParent, "kandev-code-review")
	if err := os.Symlink(realSkillDir, symlinkSkillDir); err != nil {
		t.Fatal(err)
	}

	err := writeSkillFiles(symlinkSkillDir, []SkillFile{{
		Path:    "references/tasks.md",
		Content: "tasks",
	}})
	if err != nil {
		t.Fatalf("writeSkillFiles: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(realSkillDir, "references", "tasks.md"))
	if err != nil {
		t.Fatalf("read support file: %v", err)
	}
	if string(got) != "tasks" {
		t.Fatalf("support file content = %q, want tasks", got)
	}
}

func TestInjectSkills_AddsFrontmatterWhenMissing(t *testing.T) {
	worktree := t.TempDir()
	ensureGit(t, worktree)

	if err := injectSkills(worktree, ".agents/skills", []Skill{
		{Slug: "kandev-team-admin", Content: "# Team\n\nUse the team commands."},
	}, testLogger(t)); err != nil {
		t.Fatalf("injectSkills: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(worktree, ".agents", "skills", "kandev-team-admin", "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	got := string(data)
	if !strings.HasPrefix(got, "---\nname: kandev-team-admin\ndescription: kandev-team-admin\n---\n") {
		t.Fatalf("SKILL.md missing synthesized frontmatter:\n%s", got)
	}
	if !strings.Contains(got, "# Team\n\nUse the team commands.") {
		t.Errorf("SKILL.md missing original body:\n%s", got)
	}
}

func TestDirName_IsIdempotentOnKandevPrefix(t *testing.T) {
	cases := []struct {
		slug string
		want string
	}{
		{slug: "code-review", want: "kandev-code-review"},
		{slug: "kandev-task-ops", want: "kandev-task-ops"},
	}
	for _, tc := range cases {
		if got := DirName(tc.slug); got != tc.want {
			t.Errorf("DirName(%q) = %q, want %q", tc.slug, got, tc.want)
		}
	}
}

func TestInjectSkills_DoesNotDoublePrefixAlreadyPrefixedSlug(t *testing.T) {
	worktree := t.TempDir()
	ensureGit(t, worktree)

	if err := injectSkills(worktree, ".agents/skills", []Skill{
		{Slug: "kandev-protocol", Content: "# Protocol"},
	}, testLogger(t)); err != nil {
		t.Fatalf("injectSkills: %v", err)
	}

	path := filepath.Join(worktree, ".agents", "skills", "kandev-protocol", "SKILL.md")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected single-prefixed dir %s: %v", path, err)
	}
	doublePrefixed := filepath.Join(worktree, ".agents", "skills", "kandev-kandev-protocol")
	if _, err := os.Stat(doublePrefixed); !os.IsNotExist(err) {
		t.Errorf("skill dir should not be double-prefixed: %s", doublePrefixed)
	}
}

func TestInjectSkills_SkipsCollidingSlugKeepsFirstDeterministically(t *testing.T) {
	worktree := t.TempDir()
	ensureGit(t, worktree)

	if err := injectSkills(worktree, ".agents/skills", []Skill{
		{Slug: "protocol", Content: "# USER protocol skill"},
		{Slug: "kandev-protocol", Content: "# BUNDLED kandev-protocol skill"},
	}, testLogger(t)); err != nil {
		t.Fatalf("injectSkills: %v", err)
	}

	skillsDir := filepath.Join(worktree, ".agents", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		t.Fatalf("read skills dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one directory for colliding slugs, got %d: %v", len(entries), entries)
	}

	data, err := os.ReadFile(filepath.Join(skillsDir, "kandev-protocol", "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if !strings.Contains(string(data), "USER protocol skill") {
		t.Errorf("first-claimed slug's content should survive, got %q", string(data))
	}
	if strings.Contains(string(data), "BUNDLED kandev-protocol skill") {
		t.Errorf("colliding second slug must not overwrite the first: %q", string(data))
	}
}

func TestInjectSkills_CollidingSlugsDoNotMixSupportFiles(t *testing.T) {
	worktree := t.TempDir()
	ensureGit(t, worktree)

	if err := injectSkills(worktree, ".agents/skills", []Skill{
		{
			Slug:    "kandev-protocol",
			Content: "# BUNDLED",
			Files:   []SkillFile{{Path: "bundled.md", Content: "bundled ref"}},
		},
		{
			Slug:    "protocol",
			Content: "# USER",
			Files:   []SkillFile{{Path: "user.md", Content: "user ref"}},
		},
	}, testLogger(t)); err != nil {
		t.Fatalf("injectSkills: %v", err)
	}

	dir := filepath.Join(worktree, ".agents", "skills", "kandev-protocol")
	if _, err := os.Stat(filepath.Join(dir, "bundled.md")); err != nil {
		t.Errorf("first-claimed skill's support file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "user.md")); !os.IsNotExist(err) {
		t.Errorf("colliding skill's support file should not have been written")
	}
}

func TestInjectSkills_PreservesExistingFrontmatter(t *testing.T) {
	worktree := t.TempDir()
	ensureGit(t, worktree)

	content := "---\nname: custom\ndescription: existing\n---\n# Body"
	if err := injectSkills(worktree, ".agents/skills", []Skill{
		{Slug: "custom", Content: content},
	}, testLogger(t)); err != nil {
		t.Fatalf("injectSkills: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(worktree, ".agents", "skills", "kandev-custom", "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if string(data) != content {
		t.Errorf("existing frontmatter should be preserved, got %q", string(data))
	}
}

func TestInjectSkills_CleanSlateRemovesPreviousKandevDirs(t *testing.T) {
	worktree := t.TempDir()
	ensureGit(t, worktree)

	if err := injectSkills(worktree, ".agents/skills", []Skill{
		{Slug: "skill-a", Content: "# A"},
		{Slug: "skill-b", Content: "# B"},
	}, testLogger(t)); err != nil {
		t.Fatalf("first inject: %v", err)
	}

	// Re-inject with only A — B's directory must be gone.
	if err := injectSkills(worktree, ".agents/skills", []Skill{
		{Slug: "skill-a", Content: "# A"},
	}, testLogger(t)); err != nil {
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
	}, testLogger(t)); err != nil {
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
	}, testLogger(t)); err != nil {
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
	// Mirrors real git's linked-worktree layout: <repo>/.git is the
	// common dir, <repo>/.git/worktrees/<name> is the per-worktree
	// gitdir, and the latter's "commondir" file points back at the
	// former (typically "../.." relative to the gitdir). Git reads
	// info/exclude from the common dir, not the per-worktree gitdir —
	// so the fix must land the pattern in commonGitDir, not
	// worktreeGitDir.
	repoRoot := t.TempDir()
	commonGitDir := filepath.Join(repoRoot, ".git")
	if err := os.MkdirAll(filepath.Join(commonGitDir, "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	worktreeGitDir := filepath.Join(commonGitDir, "worktrees", "wt1")
	if err := os.MkdirAll(worktreeGitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktreeGitDir, "commondir"), []byte("../..\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	worktree := t.TempDir()
	gitFile := filepath.Join(worktree, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: "+worktreeGitDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ensureGitExclude(worktree, ".agents/skills"); err != nil {
		t.Fatalf("ensureGitExclude: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(commonGitDir, "info", "exclude"))
	if err != nil {
		t.Fatalf("read exclude from common dir: %v", err)
	}
	if !strings.Contains(string(data), ".agents/skills/kandev-*") {
		t.Errorf("pattern not appended to common dir's exclude: %q", string(data))
	}

	if _, err := os.Stat(filepath.Join(worktreeGitDir, "info", "exclude")); !os.IsNotExist(err) {
		t.Errorf("pattern must not be written to the per-worktree gitdir, which git never reads for info/exclude")
	}
}

func TestInjectSkills_NoGitDirIsNotFatal(t *testing.T) {
	worktree := t.TempDir() // no .git
	if err := injectSkills(worktree, ".agents/skills", []Skill{
		{Slug: "ok", Content: "# ok"},
	}, testLogger(t)); err != nil {
		t.Fatalf("injectSkills: %v", err)
	}
	if _, err := os.Stat(filepath.Join(worktree, ".agents", "skills", "kandev-ok", "SKILL.md")); err != nil {
		t.Errorf("skill should still land without .git: %v", err)
	}
}

// requireGit skips the test if a git binary isn't available.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func gitStatusPorcelain(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status: %v\n%s", err, out)
	}
	return string(out)
}

// initRepoWithSymlinkedSkillDir builds a repo mirroring this repo's own
// layout: ".claude/skills" is a tracked symlink to "../.agents/skills".
func initRepoWithSymlinkedSkillDir(t *testing.T, repo string) {
	t.Helper()
	runGit(t, repo, "init", "-q", ".")
	if err := os.MkdirAll(filepath.Join(repo, ".agents", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(
		filepath.Join("..", ".agents", "skills"),
		filepath.Join(repo, ".claude", "skills"),
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".agents", "skills", ".gitkeep"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-q", "-m", "init")
}

// TestEnsureGitExclude_RealGit_SymlinkedSkillDir reproduces defect A:
// injecting through a symlinked project skill dir must not leave the
// resolved target directory untracked.
func TestEnsureGitExclude_RealGit_SymlinkedSkillDir(t *testing.T) {
	requireGit(t)
	repo := t.TempDir()
	initRepoWithSymlinkedSkillDir(t, repo)

	if err := injectSkills(repo, ".claude/skills", []Skill{
		{Slug: "demo", Content: "# Demo"},
	}, testLogger(t)); err != nil {
		t.Fatalf("injectSkills: %v", err)
	}

	if status := gitStatusPorcelain(t, repo); status != "" {
		t.Errorf("git status not clean after injecting through symlinked skill dir:\n%s", status)
	}
}

// TestEnsureGitExclude_RealGit_LinkedWorktree reproduces defect B: a
// real `git worktree add` linked worktree, non-symlinked skill dir.
func TestEnsureGitExclude_RealGit_LinkedWorktree(t *testing.T) {
	requireGit(t)
	repo := t.TempDir()
	runGit(t, repo, "init", "-q", ".")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# repo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-q", "-m", "init")
	runGit(t, repo, "branch", "-q", "wt-branch")

	worktree := filepath.Join(t.TempDir(), "wt")
	runGit(t, repo, "worktree", "add", "-q", worktree, "wt-branch")

	if err := injectSkills(worktree, ".agents/skills", []Skill{
		{Slug: "demo", Content: "# Demo"},
	}, testLogger(t)); err != nil {
		t.Fatalf("injectSkills: %v", err)
	}

	if status := gitStatusPorcelain(t, worktree); status != "" {
		t.Errorf("git status not clean in linked worktree after injecting skills:\n%s", status)
	}
}

// TestEnsureGitExclude_RealGit_SymlinkedSkillDirInLinkedWorktree covers
// defect A and B together: this repo's own layout, where a Claude
// session runs inside a Kandev task worktree (always a linked
// worktree) against the tracked ".claude/skills" symlink.
func TestEnsureGitExclude_RealGit_SymlinkedSkillDirInLinkedWorktree(t *testing.T) {
	requireGit(t)
	repo := t.TempDir()
	initRepoWithSymlinkedSkillDir(t, repo)
	runGit(t, repo, "branch", "-q", "wt-branch")

	worktree := filepath.Join(t.TempDir(), "wt")
	runGit(t, repo, "worktree", "add", "-q", worktree, "wt-branch")

	if err := injectSkills(worktree, ".claude/skills", []Skill{
		{Slug: "demo", Content: "# Demo"},
	}, testLogger(t)); err != nil {
		t.Fatalf("injectSkills: %v", err)
	}

	if status := gitStatusPorcelain(t, worktree); status != "" {
		t.Errorf("git status not clean for symlinked skill dir in linked worktree:\n%s", status)
	}
}
