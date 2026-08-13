package github

import (
	"context"
	"testing"
	"time"
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

// TestAssociateExistingPRByURL_LinkingAlreadyMergedPRPopulatesOutcomeFields
// is the regression for AC-36 on the "+Task" URL link path: this flow
// fetches via GetPR, a full single-PR fetch (AC-10), so a PR that is already
// merged at the moment of linking must not create a row with merged_at set
// and merged_by_login left NULL — that combination is exactly the
// writer-fault state AC-36 forbids.
func TestAssociateExistingPRByURL_LinkingAlreadyMergedPRPopulatesOutcomeFields(t *testing.T) {
	_, svc, mockClient, _ := setupPollerTest(t)
	ctx := context.Background()

	mergedAt := time.Now().UTC()
	mockClient.AddPR(&PR{
		Number: 43, Title: "Already merged", State: prStateMerged,
		HeadSHA: "abc", HeadBranch: "feat/y", RepoOwner: "org", RepoName: "repo",
		HTMLURL:  "https://github.com/org/repo/pull/43",
		MergedAt: &mergedAt, Draft: false, ChangedFiles: 7, MergedByLogin: "carlosflorencio",
	})

	tp, err := svc.AssociateExistingPRByURL(ctx, "task-C", "repo-1", "https://github.com/org/repo/pull/43")
	if err != nil {
		t.Fatalf("associate: %v", err)
	}
	if tp == nil || tp.MergedAt == nil {
		t.Fatalf("unexpected TaskPR: %+v", tp)
	}
	if tp.MergedByLogin == nil || *tp.MergedByLogin != "carlosflorencio" {
		t.Errorf("merged_by_login = %v, want %q (AC-36 requires non-NULL once merged_at is set)",
			tp.MergedByLogin, "carlosflorencio")
	}
	if tp.IsDraft == nil || *tp.IsDraft != false {
		t.Errorf("is_draft = %v, want false observed, not NULL", tp.IsDraft)
	}
	if tp.ChangedFiles == nil || *tp.ChangedFiles != 7 {
		t.Errorf("changed_files = %v, want 7 observed, not NULL", tp.ChangedFiles)
	}
}
