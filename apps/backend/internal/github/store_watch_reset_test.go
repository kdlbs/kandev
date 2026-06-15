package github

import (
	"context"
	"testing"
	"time"
)

// TestStore_ReviewWatch_ListTaskIDsAndReset pins the contract used by the
// review watch reset flow: every dedup row's task_id (including empty
// reservations) is enumerable, and ResetReviewWatchState wipes those rows
// atomically alongside last_polled_at.
func TestStore_ReviewWatch_ListTaskIDsAndReset(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	rw := &ReviewWatch{
		WorkspaceID:       "ws-1",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "step-1",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
		ReviewScope:       "user_and_teams",
		Enabled:           true,
	}
	if err := store.CreateReviewWatch(ctx, rw); err != nil {
		t.Fatalf("create review watch: %v", err)
	}
	now := time.Now().UTC()
	rw.LastPolledAt = &now
	rw.UpdatedAt = now
	if err := store.UpdateReviewWatch(ctx, rw); err != nil {
		t.Fatalf("stamp last polled: %v", err)
	}

	tasks := []*ReviewPRTask{
		{ReviewWatchID: rw.ID, RepoOwner: "acme", RepoName: "widget", PRNumber: 1, PRURL: "https://github.com/acme/widget/pull/1", TaskID: "task-a", CreatedAt: now},
		{ReviewWatchID: rw.ID, RepoOwner: "acme", RepoName: "widget", PRNumber: 2, PRURL: "https://github.com/acme/widget/pull/2", TaskID: "task-b", CreatedAt: now},
		{ReviewWatchID: rw.ID, RepoOwner: "acme", RepoName: "widget", PRNumber: 3, PRURL: "https://github.com/acme/widget/pull/3", TaskID: "", CreatedAt: now},
	}
	for _, tp := range tasks {
		if err := store.CreateReviewPRTask(ctx, tp); err != nil {
			t.Fatalf("create review pr task: %v", err)
		}
	}

	ids, err := store.ListReviewPRTaskIDsByWatch(ctx, rw.ID)
	if err != nil {
		t.Fatalf("list ids: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("ListReviewPRTaskIDsByWatch returned %d rows, want 3 (including empty reservation)", len(ids))
	}
	nonEmpty := 0
	for _, id := range ids {
		if id != "" {
			nonEmpty++
		}
	}
	if nonEmpty != 2 {
		t.Errorf("expected 2 non-empty task IDs, got %d", nonEmpty)
	}

	if err := store.ResetReviewWatchState(ctx, rw.ID); err != nil {
		t.Fatalf("reset: %v", err)
	}

	idsAfter, err := store.ListReviewPRTaskIDsByWatch(ctx, rw.ID)
	if err != nil {
		t.Fatalf("list ids after reset: %v", err)
	}
	if len(idsAfter) != 0 {
		t.Errorf("expected 0 dedup rows after reset, got %d", len(idsAfter))
	}
	got, err := store.GetReviewWatch(ctx, rw.ID)
	if err != nil {
		t.Fatalf("get watch: %v", err)
	}
	if got.LastPolledAt != nil {
		t.Errorf("expected LastPolledAt to be nil after reset, got %v", got.LastPolledAt)
	}
}

// TestStore_IssueWatch_ListTaskIDsAndReset mirrors the review watch test for
// the issue watch reset path.
func TestStore_IssueWatch_ListTaskIDsAndReset(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	iw := &IssueWatch{
		WorkspaceID:       "ws-1",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "step-1",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
		Enabled:           true,
	}
	if err := store.CreateIssueWatch(ctx, iw); err != nil {
		t.Fatalf("create issue watch: %v", err)
	}
	now := time.Now().UTC()
	iw.LastPolledAt = &now
	iw.UpdatedAt = now
	if err := store.UpdateIssueWatch(ctx, iw); err != nil {
		t.Fatalf("stamp last polled: %v", err)
	}

	// Three reservations, only two get task IDs assigned so the third
	// exercises the empty-ID inclusion the reset flow depends on.
	for _, n := range []int{1, 2, 3} {
		if _, err := store.ReserveIssueWatchTask(ctx, iw.ID, "acme", "widget", n, "https://github.com/acme/widget/issues/0"); err != nil {
			t.Fatalf("reserve %d: %v", n, err)
		}
	}
	if err := store.AssignIssueWatchTaskID(ctx, iw.ID, "acme", "widget", 1, "task-a"); err != nil {
		t.Fatalf("assign 1: %v", err)
	}
	if err := store.AssignIssueWatchTaskID(ctx, iw.ID, "acme", "widget", 2, "task-b"); err != nil {
		t.Fatalf("assign 2: %v", err)
	}

	ids, err := store.ListIssueWatchTaskIDsByWatch(ctx, iw.ID)
	if err != nil {
		t.Fatalf("list ids: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("ListIssueWatchTaskIDsByWatch returned %d rows, want 3 (including empty reservation)", len(ids))
	}

	if err := store.ResetIssueWatchState(ctx, iw.ID); err != nil {
		t.Fatalf("reset: %v", err)
	}

	idsAfter, err := store.ListIssueWatchTaskIDsByWatch(ctx, iw.ID)
	if err != nil {
		t.Fatalf("list ids after reset: %v", err)
	}
	if len(idsAfter) != 0 {
		t.Errorf("expected 0 dedup rows after reset, got %d", len(idsAfter))
	}
	got, err := store.GetIssueWatch(ctx, iw.ID)
	if err != nil {
		t.Fatalf("get watch: %v", err)
	}
	if got.LastPolledAt != nil {
		t.Errorf("expected LastPolledAt to be nil after reset, got %v", got.LastPolledAt)
	}
}
