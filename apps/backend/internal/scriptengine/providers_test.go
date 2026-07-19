package scriptengine

import (
	"os/exec"
	"strings"
	"testing"
)

// shellUnquote runs `printf %s <arg>` through /bin/sh so the test can prove that
// a single-quoted, shell-escaped value parses back to the intended literal
// without any command substitution firing.
func shellUnquote(t *testing.T, arg string) string {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	out, err := exec.Command("sh", "-c", "printf %s "+arg).CombinedOutput()
	if err != nil {
		t.Fatalf("sh failed for %q: %v\n%s", arg, err, out)
	}
	return string(out)
}

func TestAgentInstallProvider(t *testing.T) {
	t.Run("empty scripts produce empty placeholder", func(t *testing.T) {
		provider := AgentInstallProvider(nil)
		vars := provider()
		if got := vars["kandev.agents.install"]; got != "" {
			t.Fatalf("expected empty, got %q", got)
		}
	})

	t.Run("deduplicates scripts", func(t *testing.T) {
		scripts := []string{
			"npm install -g @anthropic-ai/claude-code@2.1.50",
			"npm install -g @openai/codex@0.104.0",
			"npm install -g @anthropic-ai/claude-code@2.1.50",
		}
		provider := AgentInstallProvider(scripts)
		vars := provider()
		want := "npm install -g @anthropic-ai/claude-code@2.1.50\nnpm install -g @openai/codex@0.104.0"
		if got := vars["kandev.agents.install"]; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("trims whitespace and skips empty", func(t *testing.T) {
		scripts := []string{"  npm install -g foo  ", "", "   ", "npm install -g bar"}
		provider := AgentInstallProvider(scripts)
		vars := provider()
		want := "npm install -g foo\nnpm install -g bar"
		if got := vars["kandev.agents.install"]; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

func TestRepositoryProvider_UsesRepositorySetupScriptKey(t *testing.T) {
	provider := RepositoryProvider(map[string]any{
		"repository_path":         "/tmp/repo",
		"base_branch":             "main",
		"repository_setup_script": "npm ci",
		"setup_script":            "should-not-be-used",
	}, nil, nil, nil)

	vars := provider()
	if got := vars["repository.setup_script"]; got != "npm ci" {
		t.Fatalf("repository.setup_script = %q, want %q", got, "npm ci")
	}
}

// TestProviders_ShellEscapeDataPlaceholders is the scriptengine-level
// regression guard for the branch-name command-injection RCE. Every data
// placeholder that lands in shell text (branch/url/path) must be emitted as a
// fully self-contained single-quoted token (shellQuote), so a hostile value
// like "$(touch pwned)" or "a;b" is a literal even when a template — including
// a stored/legacy prepare_script — references the placeholder BARE.
func TestProviders_ShellEscapeDataPlaceholders(t *testing.T) {
	// A payload containing a single quote exercises the escape sequence: the
	// embedded `'` becomes `'"'"'`, and shellQuote wraps the whole in `'...'`.
	const evil = `x'$(touch pwned)`
	const wantQuoted = `'x'"'"'$(touch pwned)'` // '  x  '"'"'  $(touch pwned)  '

	// assertQuoted checks the provider emitted the fully-quoted token AND that
	// the token parses back to the original literal through a real shell (no
	// command substitution fires).
	assertQuoted := func(t *testing.T, key, got string) {
		t.Helper()
		if got != wantQuoted {
			t.Errorf("%s = %q, want %q", key, got, wantQuoted)
		}
		if out := shellUnquote(t, got); out != evil {
			t.Errorf("%s token %q evaluated to %q, want literal %q", key, got, out, evil)
		}
	}

	t.Run("WorktreeProvider quotes branch and paths", func(t *testing.T) {
		vars := WorktreeProvider(evil, evil, "wt-id", evil, evil)()
		for _, key := range []string{"worktree.base_path", "worktree.path", "worktree.branch", "worktree.base_branch"} {
			assertQuoted(t, key, vars[key])
		}
		// worktree.id is a kandev UUID, intentionally not quoted.
		if got := vars["worktree.id"]; got != "wt-id" {
			t.Errorf("worktree.id = %q, want unmodified", got)
		}
	})

	t.Run("WorkspaceProvider quotes path", func(t *testing.T) {
		assertQuoted(t, "workspace.path", WorkspaceProvider(evil)()["workspace.path"])
	})

	t.Run("RepositoryProvider quotes branch, paths and clone url", func(t *testing.T) {
		vars := RepositoryProvider(map[string]any{
			"base_branch":          evil,
			"repository_clone_url": evil,
			"repository_path":      evil,
		}, nil, nil, nil)()
		assertQuoted(t, "repository.branch", vars["repository.branch"])
		assertQuoted(t, "repository.clone_url", vars["repository.clone_url"])
		assertQuoted(t, "repository.path", vars["repository.path"])
		// repository.name is derived from the (hostile) path's last segment and
		// must also be quoted, since it can land in shell text.
		assertQuoted(t, "repository.name", vars["repository.name"])
	})

	t.Run("resolver-derived repository.ssh_url is quoted", func(t *testing.T) {
		// When no explicit clone URL is set, the provider resolves the remote
		// via repoURLResolver; that value is attacker-influenceable (a remote
		// URL) and must be quoted too.
		resolver := func(string) (string, error) { return evil, nil }
		vars := RepositoryProvider(map[string]any{
			"repository_path": "/tmp/repo",
		}, nil, resolver, nil)()
		assertQuoted(t, "repository.ssh_url", vars["repository.ssh_url"])
		assertQuoted(t, "repository.clone_url", vars["repository.clone_url"])
	})

	t.Run("repository.setup_script is NOT quoted (script fragment)", func(t *testing.T) {
		fragment := "npm ci\necho 'hi'"
		vars := RepositoryProvider(map[string]any{
			"repository_setup_script": fragment,
		}, nil, nil, nil)()
		if got := vars["repository.setup_script"]; got != fragment {
			t.Errorf("repository.setup_script = %q, want unmodified %q", got, fragment)
		}
	})
}

func TestGitHubAuthProvider(t *testing.T) {
	t.Run("nil env returns fallback commands", func(t *testing.T) {
		provider := GitHubAuthProvider(nil)
		vars := provider()
		setup := vars["github.auth_setup"]
		if !strings.Contains(setup, "fallback") {
			t.Errorf("expected fallback comment, got %q", setup)
		}
		if strings.Contains(setup, "credential") {
			t.Error("expected no credential helper when no token")
		}
	})

	t.Run("empty env returns fallback commands", func(t *testing.T) {
		provider := GitHubAuthProvider(map[string]string{})
		vars := provider()
		setup := vars["github.auth_setup"]
		if !strings.Contains(setup, "fallback") {
			t.Errorf("expected fallback comment, got %q", setup)
		}
	})

	t.Run("GH_TOKEN present configures credential helper", func(t *testing.T) {
		provider := GitHubAuthProvider(map[string]string{"GH_TOKEN": "ghp_test123"})
		vars := provider()
		setup := vars["github.auth_setup"]
		if !strings.Contains(setup, "credential.https://github.com.helper") {
			t.Errorf("expected credential helper, got %q", setup)
		}
		// Must use /bin/sh, not /bin/bash
		if strings.Contains(setup, "/bin/bash") {
			t.Error("expected /bin/sh, not /bin/bash")
		}
		if !strings.Contains(setup, "/bin/sh") {
			t.Error("expected /bin/sh in credential helper")
		}
		// Token must NOT be hardcoded in the output script
		if strings.Contains(setup, "ghp_test123") {
			t.Error("token value must not appear literally in the script")
		}
		// Must use env var fallback pattern
		if !strings.Contains(setup, "${GH_TOKEN:-${GITHUB_TOKEN}}") {
			t.Error("expected ${GH_TOKEN:-${GITHUB_TOKEN}} fallback pattern")
		}
	})

	t.Run("GITHUB_TOKEN fallback configures credential helper", func(t *testing.T) {
		provider := GitHubAuthProvider(map[string]string{"GITHUB_TOKEN": "ghp_fallback"})
		vars := provider()
		setup := vars["github.auth_setup"]
		if !strings.Contains(setup, "credential.https://github.com.helper") {
			t.Errorf("expected credential helper with GITHUB_TOKEN fallback, got %q", setup)
		}
		if strings.Contains(setup, "ghp_fallback") {
			t.Error("token value must not appear literally in the script")
		}
	})

	t.Run("includes gh CLI setup", func(t *testing.T) {
		provider := GitHubAuthProvider(map[string]string{"GH_TOKEN": "ghp_test"})
		vars := provider()
		setup := vars["github.auth_setup"]
		if !strings.Contains(setup, "gh config set git_protocol https") {
			t.Error("expected gh CLI protocol config")
		}
		if !strings.Contains(setup, "gh auth setup-git") {
			t.Error("expected gh auth setup-git backup")
		}
	})
}
