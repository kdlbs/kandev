package scriptengine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const checkoutSentinelFile = "sentinel.bin"

var selectedSentinel = []byte("selected branch\n{{workspace.path}}\x00end\n")

type literalGitFixture struct {
	root, origin, selectedHead string
	env                        []string
}

// @covers AC-EXECUTORS-REPOSITORY-BRANCH-001.2
// @covers AC-EXECUTORS-REPOSITORY-BRANCH-001.4
func TestResolverLiteralBranchCheckout(t *testing.T) {
	for _, form := range []string{"current-fetch", "saved-clone"} {
		t.Run(form, func(t *testing.T) {
			fixture := newLiteralGitFixture(t)
			control := filepath.Join(fixture.root, "literal-control")
			literalCommand := fixture.literalCommand(form, control, literalTokenBranch)
			if out, err := fixture.run(t, "sh", "-eu", "-c", literalCommand); err != nil {
				t.Fatalf("literal control failed: %v\n%s", err, out)
			}
			fixture.assertSelected(t, control)
			t.Logf("literal control: exit 0; branch %s; HEAD %s; sentinel bytes exact", literalTokenBranch, fixture.selectedHead)
			checkout := filepath.Join(fixture.root, "resolved-checkout")
			want := fixture.literalCommand(form, checkout, literalTokenBranch)
			samples := sampleLiteralRenders(fixture.resolver(checkout, literalTokenBranch), checkoutTemplate(form), want)
			t.Logf("mutated renders: %d/128; selected command: %s", samples.mutations, samples.command)
			if out, err := fixture.run(t, "sh", "-eu", "-c", samples.command); err != nil {
				t.Fatalf("rendered checkout failed: %v (mutated %d/128)\n%s", err, samples.mutations, out)
			}
			fixture.assertSelected(t, checkout)
			if samples.mutations != 0 {
				t.Fatalf("command differed from literal expectation in %d/128 renders", samples.mutations)
			}
		})
	}
}

// @covers AC-EXECUTORS-REPOSITORY-BRANCH-001.5
func TestResolverLiteralBranchMissing(t *testing.T) {
	for _, form := range []string{"current-fetch", "saved-clone"} {
		t.Run(form, func(t *testing.T) {
			fixture := newLiteralGitFixture(t)
			checkout := filepath.Join(fixture.root, "missing-checkout")
			branch := "missing/{{workspace.path}}"
			command := fixture.resolver(checkout, branch).Resolve(checkoutTemplate(form))
			out, err := fixture.run(t, "sh", "-eu", "-c", command)
			if err == nil {
				t.Fatalf("missing branch checkout succeeded: %s", out)
			}
			if head, err := fixture.run(t, "git", "-C", checkout, "rev-parse", "--verify", "HEAD"); err == nil {
				t.Fatalf("missing branch fell back to HEAD %s", head)
			}
			if _, err := os.Stat(filepath.Join(checkout, checkoutSentinelFile)); !os.IsNotExist(err) {
				t.Fatalf("unexpected fallback sentinel: %v", err)
			}
			if want := fixture.literalCommand(form, checkout, branch); command != want {
				t.Errorf("missing branch command mutated: %q, want %q", command, want)
			}
		})
	}
}

func newLiteralGitFixture(t *testing.T) literalGitFixture {
	t.Helper()
	for _, tool := range []string{"git", "sh"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " is unavailable")
		}
	}
	root := t.TempDir()
	t.Logf("native Git fixture: %s", root)
	fixture := literalGitFixture{root: root, origin: filepath.Join(root, "origin.git")}
	for _, variable := range os.Environ() {
		key, _, _ := strings.Cut(variable, "=")
		if strings.HasPrefix(key, "GIT_") || key == "HOME" || key == "XDG_CONFIG_HOME" || key == "GH_TOKEN" || key == "GITHUB_TOKEN" {
			continue
		}
		fixture.env = append(fixture.env, variable)
	}
	fixture.env = append(fixture.env, "HOME="+root, "XDG_CONFIG_HOME="+root,
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+filepath.Join(root, "gitconfig"), "GIT_TERMINAL_PROMPT=0")
	seed := filepath.Join(root, "seed")
	fixture.git(t, "init", "--bare", "--initial-branch=main", fixture.origin)
	fixture.git(t, "init", "--initial-branch=main", seed)
	fixture.commitSentinel(t, seed, []byte("default branch\n"))
	fixture.git(t, "-C", seed, "checkout", "-b", literalTokenBranch)
	fixture.commitSentinel(t, seed, selectedSentinel)
	fixture.selectedHead = fixture.git(t, "-C", seed, "rev-parse", "HEAD")
	fixture.git(t, "-C", seed, "push", fixture.origin, "main", literalTokenBranch)
	return fixture
}

func (f literalGitFixture) commitSentinel(t *testing.T, seed string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(seed, checkoutSentinelFile), data, 0o600); err != nil {
		t.Fatal(err)
	}
	f.git(t, "-C", seed, "add", checkoutSentinelFile)
	f.git(t, "-C", seed, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "commit", "-m", "sentinel")
}

func (f literalGitFixture) run(t *testing.T, tool string, args ...string) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, tool, args...)
	command.Dir, command.Env = f.root, f.env
	return command.CombinedOutput()
}

func (f literalGitFixture) git(t *testing.T, args ...string) string {
	t.Helper()
	out, err := f.run(t, "git", args...)
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (f literalGitFixture) resolver(checkout, branch string) *Resolver {
	return NewResolver().
		WithProvider(RepositoryProvider(map[string]any{"base_branch": branch, "repository_clone_url": f.origin}, nil, nil, nil)).
		WithProvider(WorkspaceProvider(checkout))
}

func (f literalGitFixture) assertSelected(t *testing.T, checkout string) {
	t.Helper()
	if got := f.git(t, "-C", checkout, "branch", "--show-current"); got != literalTokenBranch {
		t.Fatalf("branch = %q, want %q", got, literalTokenBranch)
	}
	if got := f.git(t, "-C", checkout, "rev-parse", "HEAD"); got != f.selectedHead {
		t.Fatalf("HEAD = %q, want %q", got, f.selectedHead)
	}
	data, err := os.ReadFile(filepath.Join(checkout, checkoutSentinelFile))
	if err != nil || !bytes.Equal(data, selectedSentinel) {
		t.Fatalf("sentinel = %q, error %v, want %q", data, err, selectedSentinel)
	}
}

func checkoutTemplate(form string) string {
	if form == "saved-clone" {
		return "git clone --branch {{repository.branch}} {{repository.clone_url}} {{workspace.path}}\n"
	}
	return `workspace={{workspace.path}}
repository_url={{repository.clone_url}}
repository_branch={{repository.branch}}
git init -q "$workspace"
git -C "$workspace" remote add origin "$repository_url"
git -C "$workspace" fetch --no-tags origin "+refs/heads/${repository_branch}:refs/remotes/origin/${repository_branch}"
git -C "$workspace" checkout -q -b "$repository_branch" "refs/remotes/origin/$repository_branch"
`
}

func (f literalGitFixture) literalCommand(form, checkout, branch string) string {
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
	if form == "saved-clone" {
		return fmt.Sprintf("git clone --branch %s %s %s\n", quote(branch), quote(f.origin), quote(checkout))
	}
	return fmt.Sprintf(`workspace=%s
repository_url=%s
repository_branch=%s
git init -q "$workspace"
git -C "$workspace" remote add origin "$repository_url"
git -C "$workspace" fetch --no-tags origin "+refs/heads/${repository_branch}:refs/remotes/origin/${repository_branch}"
git -C "$workspace" checkout -q -b "$repository_branch" "refs/remotes/origin/$repository_branch"
`, quote(checkout), quote(f.origin), quote(branch))
}
