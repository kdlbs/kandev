package github

import (
	"context"
	"testing"
	"time"
)

// TestDisableIssueWatchWithError_StampsCauseAndDisables pins the self-heal
// contract for github_issue_watches: orphaned watcher is flipped to
// enabled=0 with a human-readable LastError + LastErrorAt timestamp.
func TestDisableIssueWatchWithError_StampsCauseAndDisables(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	iw := &IssueWatch{
		WorkspaceID:       "ws-1",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "step-1",
		AgentProfileID:    "deleted-profile",
		ExecutorProfileID: "exec-1",
		Enabled:           true,
	}
	if err := store.CreateIssueWatch(ctx, iw); err != nil {
		t.Fatalf("create: %v", err)
	}

	const cause = `agent profile "Removed Kilo" (deleted-profile) was removed`
	// Widen the window by 1s on each side to absorb SQLite second-precision
	// timestamp rounding.
	before := time.Now().UTC().Add(-time.Second)
	if err := store.DisableIssueWatchWithError(ctx, iw.ID, cause); err != nil {
		t.Fatalf("disable: %v", err)
	}
	after := time.Now().UTC().Add(time.Second)

	got, err := store.GetIssueWatch(ctx, iw.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected row, got nil")
	}
	if got.Enabled {
		t.Error("Enabled should be false after self-heal")
	}
	if got.LastError != cause {
		t.Errorf("LastError = %q, want %q", got.LastError, cause)
	}
	if got.LastErrorAt == nil {
		t.Fatal("LastErrorAt should be set")
	}
	if got.LastErrorAt.Before(before) || got.LastErrorAt.After(after) {
		t.Errorf("LastErrorAt %v outside [%v, %v]", got.LastErrorAt, before, after)
	}
}

// TestDisableReviewWatchWithError_StampsCauseAndDisables mirrors the issue
// test for review watches.
func TestDisableReviewWatchWithError_StampsCauseAndDisables(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	rw := &ReviewWatch{
		WorkspaceID:       "ws-1",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "step-1",
		AgentProfileID:    "deleted-profile",
		ExecutorProfileID: "exec-1",
		ReviewScope:       "user_and_teams",
		Enabled:           true,
	}
	if err := store.CreateReviewWatch(ctx, rw); err != nil {
		t.Fatalf("create: %v", err)
	}

	const cause = `agent profile "Removed Opencode" (deleted-profile) was removed`
	before := time.Now().UTC().Add(-time.Second)
	if err := store.DisableReviewWatchWithError(ctx, rw.ID, cause); err != nil {
		t.Fatalf("disable: %v", err)
	}
	after := time.Now().UTC().Add(time.Second)

	got, err := store.GetReviewWatch(ctx, rw.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected row, got nil")
	}
	if got.Enabled {
		t.Error("Enabled should be false after self-heal")
	}
	if got.LastError != cause {
		t.Errorf("LastError = %q, want %q", got.LastError, cause)
	}
	if got.LastErrorAt == nil {
		t.Fatal("LastErrorAt should be set")
	}
	if got.LastErrorAt.Before(before) || got.LastErrorAt.After(after) {
		t.Errorf("LastErrorAt %v outside [%v, %v]", got.LastErrorAt, before, after)
	}
}
