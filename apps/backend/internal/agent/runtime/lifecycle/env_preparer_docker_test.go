package lifecycle

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
)

func TestDockerPreparer_GeneratesTaskBranch(t *testing.T) {
	preparer := NewDockerPreparer(newTestLogger())

	result, err := preparer.Prepare(context.Background(), &EnvPrepareRequest{
		TaskID:               "task-123",
		SessionID:            "session-123",
		TaskTitle:            "Manual Docker Fix",
		ExecutorType:         executor.NameDocker,
		WorkspacePath:        "/workspace",
		RepositoryPath:       "/tmp/repo",
		BaseBranch:           "main",
		WorktreeBranchPrefix: "feature/",
	}, nil)
	if err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}
	if result.WorktreeBranch == "" {
		t.Fatal("expected Docker preparer to choose a task branch")
	}
	if !strings.HasPrefix(result.WorktreeBranch, "feature/manual-docker-fix-") {
		t.Fatalf("WorktreeBranch = %q, want feature/manual-docker-fix-*", result.WorktreeBranch)
	}
	if result.WorktreeBranch == "main" {
		t.Fatal("Docker task branch must not be the base branch")
	}
}

func TestDockerPreparer_ReusesExistingTaskBranch(t *testing.T) {
	preparer := NewDockerPreparer(newTestLogger())

	result, err := preparer.Prepare(context.Background(), &EnvPrepareRequest{
		TaskID:         "task-123",
		SessionID:      "session-123",
		TaskTitle:      "Manual Docker Fix",
		ExecutorType:   executor.NameDocker,
		WorkspacePath:  "/workspace",
		RepositoryPath: "/tmp/repo",
		BaseBranch:     "main",
		WorktreeBranch: "feature/existing-task-abc",
	}, nil)
	if err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}
	if result.WorktreeBranch != "feature/existing-task-abc" {
		t.Fatalf("WorktreeBranch = %q, want existing branch", result.WorktreeBranch)
	}
}
