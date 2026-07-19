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
// regression guard for the branch-name command-injection RCE. Data
// placeholders that land in shell text (branch/url/path) must be
// shell-single-quoted so a hostile value like "$(touch pwned)" or "a;b" cannot
// break out of the surrounding single quotes in the prepare-script templates.
func TestProviders_ShellEscapeDataPlaceholders(t *testing.T) {
	// A payload containing a single quote exercises the escape sequence: the
	// only character shellSingleQuote transforms is `'` -> `'"'"'`.
	const evil = `x'$(touch pwned)`
	const wantEscaped = `x'"'"'$(touch pwned)` // ' closed, "'" literal quote, ' reopened

	t.Run("WorktreeProvider escapes branch and paths", func(t *testing.T) {
		vars := WorktreeProvider("/base", "/wt", "wt-id", evil, evil)()
		for _, key := range []string{"worktree.branch", "worktree.base_branch"} {
			if got := vars[key]; got != wantEscaped {
				t.Errorf("%s = %q, want %q", key, got, wantEscaped)
			}
		}
		// worktree.id is a kandev UUID, intentionally not escaped.
		if got := vars["worktree.id"]; got != "wt-id" {
			t.Errorf("worktree.id = %q, want unmodified", got)
		}
	})

	t.Run("WorkspaceProvider escapes path", func(t *testing.T) {
		if got := WorkspaceProvider(evil)()["workspace.path"]; got != wantEscaped {
			t.Errorf("workspace.path = %q, want %q", got, wantEscaped)
		}
	})

	t.Run("RepositoryProvider escapes branch and clone url", func(t *testing.T) {
		vars := RepositoryProvider(map[string]any{
			"base_branch":          evil,
			"repository_clone_url": evil,
			"repository_path":      "/tmp/repo",
		}, nil, nil, nil)()
		if got := vars["repository.branch"]; got != wantEscaped {
			t.Errorf("repository.branch = %q, want %q", got, wantEscaped)
		}
		if got := vars["repository.clone_url"]; got != wantEscaped {
			t.Errorf("repository.clone_url = %q, want %q", got, wantEscaped)
		}
	})

	t.Run("repository.setup_script is NOT escaped (script fragment)", func(t *testing.T) {
		fragment := "npm ci\necho 'hi'"
		vars := RepositoryProvider(map[string]any{
			"repository_setup_script": fragment,
		}, nil, nil, nil)()
		if got := vars["repository.setup_script"]; got != fragment {
			t.Errorf("repository.setup_script = %q, want unmodified %q", got, fragment)
		}
	})

	t.Run("wrapping escaped value in single quotes yields the literal", func(t *testing.T) {
		// This is the invariant the templates rely on: '<escaped>' parses back
		// to the original string with no command substitution.
		wrapped := "'" + wantEscaped + "'"
		if out := shellUnquote(t, wrapped); out != evil {
			t.Errorf("'%s' evaluated to %q, want literal %q", wantEscaped, out, evil)
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
