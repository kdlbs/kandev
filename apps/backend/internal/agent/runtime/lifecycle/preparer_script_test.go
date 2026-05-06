package lifecycle

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
)

func TestResolvePreparerSetupScript_LocalFallbackCommentOnly(t *testing.T) {
	req := &EnvPrepareRequest{
		ExecutorType:   executor.NameStandalone,
		RepositoryPath: "/tmp/my-repo",
	}

	got := resolvePreparerSetupScript(req, "/tmp/my-repo")
	if got != "" {
		t.Fatalf("expected comment-only default script to be treated as empty, got %q", got)
	}
}

func TestIsScriptEffectivelyEmpty(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty", "", true},
		{"shebang only", "#!/bin/bash\n", true},
		{"shebang and comments", "#!/bin/bash\n# comment\n# another\n", true},
		{"blank lines and comments", "\n# comment\n\n# more\n\n", true},
		{"has command", "#!/bin/bash\necho hello\n", false},
		{"command after comments", "#!/bin/bash\n# setup\napt-get install git\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isScriptEffectivelyEmpty(tt.input)
			if got != tt.want {
				t.Fatalf("isScriptEffectivelyEmpty(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolvePreparerSetupScript_LocalWithRepoSetupScript(t *testing.T) {
	req := &EnvPrepareRequest{
		ExecutorType:    executor.NameStandalone,
		RepositoryPath:  "/tmp/my-repo",
		RepoSetupScript: "make install",
	}

	got := resolvePreparerSetupScript(req, "/tmp/my-repo")
	if got == "" {
		t.Fatal("expected non-empty script when repo setup script is set")
	}
	if !strings.Contains(got, "make install") {
		t.Fatalf("expected repo setup script in resolved output, got %q", got)
	}
}

func TestResolvePreparerSetupScript_WorktreeWithRepoSetupScript(t *testing.T) {
	req := &EnvPrepareRequest{
		ExecutorType:    executor.NameStandalone,
		UseWorktree:     true,
		RepositoryPath:  "/tmp/my-repo",
		RepoSetupScript: "npm ci",
	}

	got := resolvePreparerSetupScript(req, "/tmp/worktrees/wt-1")
	if got == "" {
		t.Fatal("expected non-empty script when repo setup script is set")
	}
	if !strings.Contains(got, "npm ci") {
		t.Fatalf("expected repo setup script in resolved output, got %q", got)
	}
}

func TestResolvePreparerSetupScript_UsesExplicitScript(t *testing.T) {
	req := &EnvPrepareRequest{
		ExecutorType:   executor.NameStandalone,
		RepositoryPath: "/tmp/my-repo",
		SetupScript:    "echo {{repository.path}}",
	}

	got := resolvePreparerSetupScript(req, "/tmp/my-repo")
	if strings.TrimSpace(got) != "echo /tmp/my-repo" {
		t.Fatalf("expected explicit script to be used and resolved, got %q", got)
	}
}

func TestResolvePreparerSetupScript_WorktreePlaceholders(t *testing.T) {
	req := &EnvPrepareRequest{
		ExecutorType:   executor.NameStandalone,
		UseWorktree:    true,
		RepositoryPath: "/tmp/main-repo",
		BaseBranch:     "main",
		WorktreeID:     "wt-123",
		WorktreeBranch: "feature/test-abc",
		SetupScript: strings.Join([]string{
			"echo {{worktree.base_path}}",
			"echo {{worktree.path}}",
			"echo {{worktree.id}}",
			"echo {{worktree.branch}}",
			"echo {{worktree.base_branch}}",
		}, "\n"),
	}

	got := resolvePreparerSetupScript(req, "/tmp/worktrees/wt-123")
	expected := []string{
		"echo /tmp/worktrees",
		"echo /tmp/worktrees/wt-123",
		"echo wt-123",
		"echo feature/test-abc",
		"echo main",
	}
	for _, want := range expected {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in resolved script, got %q", want, got)
		}
	}
	if strings.Contains(got, "{{worktree.path}}") {
		t.Fatalf("expected worktree placeholders to be resolved, got %q", got)
	}
}
