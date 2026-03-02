package scriptengine

import (
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
