package github

import (
	"context"
	"testing"
)

// TestAssociateExistingPRByURL_CreatesTwoRowsForDifferentTasks regression-tests
// the bug where two tasks created from the same PR via the GitHub page "+
// Task" launcher ended up with only one github_task_prs row. The launcher
// now calls AssociateExistingPRByURL directly so the linkage no longer
// depends on branch-based discovery (which fails for review tasks that use
// synthetic worktree branches).
func TestAssociateExistingPRByURL_CreatesTwoRowsForDifferentTasks(t *testing.T) {
	_, svc, mockClient, _ := setupPollerTest(t)
	ctx := context.Background()

	mockClient.AddPR(&PR{
		Number:     42,
		Title:      "Feature PR",
		State:      "open",
		HeadSHA:    "abc",
		HeadBranch: "feat/x",
		RepoOwner:  "org",
		RepoName:   "repo",
		HTMLURL:    "https://github.com/org/repo/pull/42",
	})

	prURL := "https://github.com/org/repo/pull/42"
	tp1, err := svc.AssociateExistingPRByURL(ctx, "task-A", "repo-1", prURL)
	if err != nil {
		t.Fatalf("first associate: %v", err)
	}
	if tp1 == nil || tp1.TaskID != "task-A" || tp1.PRNumber != 42 {
		t.Fatalf("unexpected first TaskPR: %+v", tp1)
	}

	tp2, err := svc.AssociateExistingPRByURL(ctx, "task-B", "repo-1", prURL)
	if err != nil {
		t.Fatalf("second associate: %v", err)
	}
	if tp2 == nil || tp2.TaskID != "task-B" || tp2.PRNumber != 42 {
		t.Fatalf("unexpected second TaskPR: %+v", tp2)
	}
	if tp1.ID == tp2.ID {
		t.Fatalf("expected distinct rows for distinct tasks, got duplicate id %s", tp1.ID)
	}
}

func TestAssociateExistingPRByURL_RejectsBadURL(t *testing.T) {
	_, svc, _, _ := setupPollerTest(t)
	ctx := context.Background()

	if _, err := svc.AssociateExistingPRByURL(ctx, "t", "r", "not-a-pr-url"); err == nil {
		t.Fatal("expected error for malformed PR URL")
	}
}
