package process

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitOperatorStageRejectsUnprotectedAttachedRepository(t *testing.T) {
	outer, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	attached := filepath.Join(outer, "attached")
	initGitRepoAt(t, attached)
	writeFile(t, attached, "nested.txt", "nested\n")

	tracker := NewWorkspaceTracker(outer, newTestLogger(t))
	tracker.SetAllowedSourceRoots([]string{attached})
	operator := NewGitOperator(outer, newTestLogger(t), tracker)

	result, err := operator.Stage(context.Background(), nil)
	if err != nil {
		t.Fatalf("Stage returned error: %v", err)
	}
	if result.Success || !strings.Contains(result.Error, errAttachedRepositoryStaging.Error()) {
		t.Fatalf("Stage result = %+v, want attached-repository staging refusal", result)
	}
	if staged := strings.TrimSpace(runGit(t, outer, "ls-files", "--stage", "--", "attached")); staged != "" {
		t.Fatalf("unprotected attached repository was staged: %q", staged)
	}
}

func TestGitOperatorStagePreservesProtectedAttachedRepository(t *testing.T) {
	outer, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	attached := filepath.Join(outer, "attached")
	initGitRepoAt(t, attached)
	writeFile(t, attached, "nested.txt", "nested\n")
	writeFile(t, outer, ".git/info/exclude", "/attached/\n")
	writeFile(t, outer, "outer-change.txt", "outer\n")

	tracker := NewWorkspaceTracker(outer, newTestLogger(t))
	tracker.SetAllowedSourceRoots([]string{attached})
	operator := NewGitOperator(outer, newTestLogger(t), tracker)

	result, err := operator.Stage(context.Background(), nil)
	if err != nil {
		t.Fatalf("Stage returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Stage failed: %+v", result)
	}
	if staged := strings.TrimSpace(runGit(t, outer, "ls-files", "--stage", "--", "attached")); staged != "" {
		t.Fatalf("protected attached repository was staged: %q", staged)
	}
	if status := runGit(t, outer, "diff", "--cached", "--name-only"); strings.TrimSpace(status) != "outer-change.txt" {
		t.Fatalf("staged outer files = %q, want outer-change.txt", status)
	}
}

func TestGitOperatorCommitStageAllRejectsUnprotectedAttachedRepository(t *testing.T) {
	outer, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	attached := filepath.Join(outer, "attached")
	initGitRepoAt(t, attached)
	writeFile(t, attached, "nested.txt", "nested\n")

	tracker := NewWorkspaceTracker(outer, newTestLogger(t))
	tracker.SetAllowedSourceRoots([]string{attached})
	operator := NewGitOperator(outer, newTestLogger(t), tracker)

	result, err := operator.Commit(context.Background(), "outer commit", true, false)
	if err != nil {
		t.Fatalf("Commit returned error: %v", err)
	}
	if result.Success || !strings.Contains(result.Error, errAttachedRepositoryStaging.Error()) {
		t.Fatalf("Commit result = %+v, want attached-repository staging refusal", result)
	}
	if staged := strings.TrimSpace(runGit(t, outer, "ls-files", "--stage", "--", "attached")); staged != "" {
		t.Fatalf("unprotected attached repository was staged by Commit: %q", staged)
	}
}
