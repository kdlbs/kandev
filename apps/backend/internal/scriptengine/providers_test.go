package scriptengine

import (
	"strings"
	"testing"
)

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
