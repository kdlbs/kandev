package lifecycle

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
)

func TestSpritesPreparer_GeneratesDeterministicTaskBranch(t *testing.T) {
	preparer := NewSpritesPreparer(newTestLogger())

	first, err := preparer.Prepare(context.Background(), &EnvPrepareRequest{
		TaskID:               "task-abc-123",
		SessionID:            "session-1",
		TaskTitle:            "Sprite Branch Fix",
		ExecutorType:         executor.NameSprites,
		WorkspacePath:        "/workspace",
		BaseBranch:           "main",
		WorktreeBranchPrefix: "feature/",
	}, nil)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if first.WorktreeBranch == "" {
		t.Fatal("expected Sprites preparer to populate WorktreeBranch")
	}
	if !strings.HasPrefix(first.WorktreeBranch, "feature/sprite-branch-fix-") {
		t.Fatalf("WorktreeBranch = %q, want feature/sprite-branch-fix-*", first.WorktreeBranch)
	}

	// Re-running with the same task fields must yield the same branch so
	// resumes (legacy env rows with empty worktree_branch) don't churn.
	second, err := preparer.Prepare(context.Background(), &EnvPrepareRequest{
		TaskID:               "task-abc-123",
		SessionID:            "session-2",
		TaskTitle:            "Sprite Branch Fix",
		ExecutorType:         executor.NameSprites,
		WorkspacePath:        "/workspace",
		BaseBranch:           "main",
		WorktreeBranchPrefix: "feature/",
	}, nil)
	if err != nil {
		t.Fatalf("Prepare (second call): %v", err)
	}
	if second.WorktreeBranch != first.WorktreeBranch {
		t.Fatalf("non-deterministic branch: first=%q second=%q", first.WorktreeBranch, second.WorktreeBranch)
	}
}

func TestSpritesPreparer_ReusesExistingTaskBranch(t *testing.T) {
	preparer := NewSpritesPreparer(newTestLogger())

	result, err := preparer.Prepare(context.Background(), &EnvPrepareRequest{
		TaskID:         "task-abc-123",
		SessionID:      "session-1",
		TaskTitle:      "Sprite Branch Fix",
		ExecutorType:   executor.NameSprites,
		WorkspacePath:  "/workspace",
		BaseBranch:     "main",
		WorktreeBranch: "feature/already-set-xyz",
	}, nil)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if result.WorktreeBranch != "feature/already-set-xyz" {
		t.Fatalf("WorktreeBranch = %q, want existing branch passthrough", result.WorktreeBranch)
	}
}
