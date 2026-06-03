package jira

import (
	"context"
	"testing"
	"time"
)

// TestDisableIssueWatchWithError_SetsDisabledStateAndStampsError mirrors the
// Linear self-heal contract: orphaned watcher is disabled with the cause
// stamped onto last_error / last_error_at.
func TestDisableIssueWatchWithError_SetsDisabledStateAndStampsError(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	watch := &IssueWatch{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		JQL:            "project = ENG",
		AgentProfileID: "deleted-profile",
		Enabled:        true,
	}
	if err := store.CreateIssueWatch(ctx, watch); err != nil {
		t.Fatalf("create: %v", err)
	}

	const cause = `agent profile "Removed Kilo" (deleted-profile) was removed`
	// Widen the window by 1s on each side to absorb SQLite second-precision
	// timestamp rounding.
	before := time.Now().UTC().Add(-time.Second)
	if err := store.DisableIssueWatchWithError(ctx, watch.ID, cause); err != nil {
		t.Fatalf("disable: %v", err)
	}
	after := time.Now().UTC().Add(time.Second)

	got, err := store.GetIssueWatch(ctx, watch.ID)
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
		t.Errorf("LastErrorAt %v outside expected window [%v, %v]", got.LastErrorAt, before, after)
	}
}
