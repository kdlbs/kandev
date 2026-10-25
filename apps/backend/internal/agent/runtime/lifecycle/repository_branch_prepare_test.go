package lifecycle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// @covers AC-EXECUTORS-REPOSITORY-BRANCH-001.1
// @covers AC-EXECUTORS-REPOSITORY-BRANCH-001.3
// @covers AC-EXECUTORS-REPOSITORY-BRANCH-001.4
func TestSpritesPrepareScript_OriginBranch(t *testing.T) {
	for _, template := range []string{"current", "saved-clone"} {
		for _, base := range []string{"origin/main", "refs/remotes/origin/main"} {
			t.Run(template+"/"+base, func(t *testing.T) {
				fixture := newBranchPrepareFixture(t)
				script := fixture.resolve(t, template, base)
				if out, err := fixture.run(t, "bash", "-e", "-c", script); err != nil {
					t.Fatalf("prepare failed: %v\n%s", err, out)
				}
				want := fixture.git(t, "--git-dir="+fixture.origin, "rev-parse", "main")
				if got := fixture.git(t, "-C", fixture.workspace, "rev-parse", "HEAD"); got != want {
					t.Fatalf("HEAD = %q, want %q", got, want)
				}
				if got := fixture.git(t, "-C", fixture.workspace, "branch", "--show-current"); got != "feature/task" {
					t.Fatalf("task branch = %q", got)
				}
				if got, err := os.ReadFile(filepath.Join(fixture.root, "base-reference")); err != nil || string(got) != base {
					t.Fatalf("worktree base reference = %q, %v", got, err)
				}
			})
		}
	}
}

// @covers AC-EXECUTORS-REPOSITORY-BRANCH-001.5
func TestSpritesPrepareScript_MissingBranch(t *testing.T) {
	for _, template := range []string{"current", "saved-clone"} {
		t.Run(template, func(t *testing.T) {
			fixture := newBranchPrepareFixture(t)
			script := fixture.resolve(t, template, "origin/missing")
			out, err := fixture.run(t, "bash", "-e", "-c", script)
			if err == nil || !strings.Contains(out, "missing") {
				t.Fatalf("expected missing-branch failure, got %v\n%s", err, out)
			}
			if out, err := fixture.run(t, "git", "-C", fixture.workspace, "rev-parse", "--verify", "HEAD"); err == nil {
				t.Fatalf("unexpected fallback checkout: %s", out)
			}
		})
	}
}

type branchPrepareFixture struct {
	root, origin, workspace string
	env                     []string
}

func newBranchPrepareFixture(t *testing.T) branchPrepareFixture {
	t.Helper()
	for _, tool := range []string{"git", "bash"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " not available")
		}
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "pnpm"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	f := branchPrepareFixture{
		root: root, origin: filepath.Join(root, "origin.git"), workspace: filepath.Join(root, "checkout"),
		env: []string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"),
			"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + filepath.Join(root, "gitconfig")},
	}
	seed := filepath.Join(root, "seed")
	f.git(t, "init", "--bare", "--initial-branch=main", f.origin)
	f.git(t, "init", "--initial-branch=main", seed)
	f.git(t, "-C", seed, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "base")
	f.git(t, "-C", seed, "push", f.origin, "main")
	return f
}

func (f branchPrepareFixture) run(t *testing.T, tool string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), tool, args...)
	cmd.Dir, cmd.Env = f.root, f.env
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (f branchPrepareFixture) git(t *testing.T, args ...string) string {
	t.Helper()
	out, err := f.run(t, "git", args...)
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return out
}

func (f branchPrepareFixture) resolve(t *testing.T, template, base string) string {
	t.Helper()
	script := DefaultPrepareScript("sprites")
	if template == "saved-clone" {
		script = "git clone --depth=1 --branch {{repository.branch}} {{repository.clone_url}} {{workspace.path}}\n"
	}
	for _, key := range []string{"git.identity_setup", "github.auth_setup", "repository.setup_script",
		"kandev.agents.install", "kandev.agentctl.install", "kandev.agentctl.start"} {
		script = strings.ReplaceAll(script, "{{"+key+"}}", ":")
	}
	script += "\nprintf '%s' {{worktree.base_branch}} > " + shellQuote(filepath.Join(f.root, "base-reference")) + "\n"
	req := &ExecutorCreateRequest{Metadata: map[string]any{
		MetadataKeySetupScript: script, MetadataKeyBaseBranch: base,
		MetadataKeyWorktreeBranch: "feature/task", "repository_clone_url": f.origin,
	}}
	resolved, err := newTestSpritesExecutor(nil).resolvePrepareScript(req)
	if err != nil {
		t.Fatal(err)
	}
	if req.Metadata[MetadataKeyBaseBranch] != base || !strings.Contains(resolved, "printf '%s' "+shellQuote(base)) {
		t.Fatal("base reference changed during script resolution")
	}
	return strings.ReplaceAll(resolved, shellQuote(spritesWorkspacePath), shellQuote(f.workspace))
}
