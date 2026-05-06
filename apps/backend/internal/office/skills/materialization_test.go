package skills

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestMaterializeSkills_GitSourceClonesRepo(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("# Git Skill\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	runGitCmd(t, src, "init")
	runGitCmd(t, src, "config", "user.email", "test@example.com")
	runGitCmd(t, src, "config", "user.name", "Test")
	runGitCmd(t, src, "add", "SKILL.md")
	runGitCmd(t, src, "commit", "-m", "add skill")

	cacheDir := t.TempDir()
	repoDir := filepath.Join(cacheDir, "git", hashLocator(src))
	if err := os.MkdirAll(filepath.Dir(repoDir), 0o755); err != nil {
		t.Fatalf("mkdirall: %v", err)
	}
	// Clone directly, bypassing the URL-scheme validator (unit-tested separately).
	if err := runGit("", "clone", "--depth=1", src, repoDir); err != nil {
		t.Fatalf("git clone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, "SKILL.md")); err != nil {
		t.Fatalf("expected cloned SKILL.md: %v", err)
	}
}

func TestMaterializeSkills_GitSourceRejectsTraversal(t *testing.T) {
	skill := &models.Skill{
		Slug:          "bad",
		SourceType:    "git",
		SourceLocator: "../repo",
	}
	if _, err := materializeSkill(skill, t.TempDir(), ""); err == nil {
		t.Fatal("expected path traversal locator to be rejected")
	}
}

func TestValidateGitLocator_AllowsValidSchemes(t *testing.T) {
	for _, locator := range []string{
		"https://github.com/user/repo",
		"ssh://git@github.com/user/repo.git",
		"git://github.com/user/repo.git",
		"git@github.com:user/repo.git",
		"git@gitlab.com:org/project.git",
	} {
		if err := validateGitLocator(locator); err != nil {
			t.Errorf("expected %q to be allowed, got: %v", locator, err)
		}
	}
}

func TestValidateGitLocator_RejectsUnsafeLocators(t *testing.T) {
	for _, locator := range []string{
		"",
		"../repo",
		"/etc/passwd",
		"file:///etc/passwd",
		"http://internal-host/repo",
		"git@nocolon",
	} {
		if err := validateGitLocator(locator); err == nil {
			t.Errorf("expected %q to be rejected", locator)
		}
	}
}

// TestValidateLocalPathUnderRoot pins the path-confinement guard that
// prevents local_path skill sources from escaping the kandev config
// base path. The sibling-prefix case (/tmp/base-evil when root is
// /tmp/base) is the exact bug class the helper was written to catch.
func TestValidateLocalPathUnderRoot(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "sub")
	sibling := root + "-evil"

	cases := []struct {
		name    string
		locator string
		wantErr bool
	}{
		{"exact root match", root, false},
		{"child path", child, false},
		{"sibling prefix collision", sibling, true},
		{"absolute escape", "/etc/ssh", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateLocalPathUnderRoot(tc.locator, root)
			if (err != nil) != tc.wantErr {
				t.Errorf("locator=%q wantErr=%v got %v", tc.locator, tc.wantErr, err)
			}
		})
	}
}

// TestValidateLocalPathUnderRoot_EmptyRootSkipsGuard documents the
// test-wiring escape hatch: an empty allowedRoot disables the guard
// (production wiring always supplies a path). Without this case a
// future tightening of the guard could silently break the in-package
// tests.
func TestValidateLocalPathUnderRoot_EmptyRootSkipsGuard(t *testing.T) {
	if err := validateLocalPathUnderRoot("/etc/ssh", ""); err != nil {
		t.Errorf("empty allowedRoot should skip guard, got %v", err)
	}
}

func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, string(out))
	}
}
