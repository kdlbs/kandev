package lifecycle

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
)

func TestResolvePreparerSetupScript_LocalFallback(t *testing.T) {
	req := &EnvPrepareRequest{
		ExecutorType:   executor.NameStandalone,
		RepositoryPath: "/tmp/my-repo",
	}

	got := resolvePreparerSetupScript(req, "/tmp/my-repo")
	if got == "" {
		t.Fatal("expected default local setup script")
	}
	if strings.Contains(got, "{{repository.path}}") {
		t.Fatalf("expected repository.path placeholder to be resolved, got %q", got)
	}
	if !strings.Contains(got, "set to /tmp/my-repo.") {
		t.Fatalf("expected resolved workspace path in script comment, got %q", got)
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
