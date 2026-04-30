package jira

import (
	"context"
	"testing"
	"time"
)

func newTestIssueWatch(workspaceID string) *IssueWatch {
	return &IssueWatch{
		WorkspaceID:    workspaceID,
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		JQL:            `project = PROJ AND status = "Open"`,
		Prompt:         "Investigate {{issue.key}}",
		Enabled:        true,
	}
}

func TestStore_IssueWatch_CreateGet(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	w := newTestIssueWatch("ws-1")
	if err := store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("create: %v", err)
	}
	if w.ID == "" {
		t.Fatal("expected ID assigned")
	}
	if w.PollIntervalSeconds != DefaultIssueWatchPollInterval {
		t.Errorf("expected default poll interval %d, got %d", DefaultIssueWatchPollInterval, w.PollIntervalSeconds)
	}
	if w.CreatedAt.IsZero() || w.UpdatedAt.IsZero() {
		t.Error("expected timestamps assigned")
	}

	got, err := store.GetIssueWatch(ctx, w.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected watch, got nil")
	}
	if got.JQL != w.JQL || got.WorkspaceID != w.WorkspaceID || got.Prompt != w.Prompt {
		t.Errorf("round-trip mismatch: %+v vs %+v", got, w)
	}
}

func TestStore_IssueWatch_ListByWorkspace(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	w1 := newTestIssueWatch("ws-1")
	w1.JQL = "project = A"
	w2 := newTestIssueWatch("ws-1")
	w2.JQL = "project = B"
	w3 := newTestIssueWatch("ws-2")

	for _, w := range []*IssueWatch{w1, w2, w3} {
		if err := store.CreateIssueWatch(ctx, w); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	got, err := store.ListIssueWatches(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 watches for ws-1, got %d", len(got))
	}
	// ws-2's watch must not appear.
	for _, w := range got {
		if w.WorkspaceID != "ws-1" {
			t.Errorf("workspace leaked into list: %s", w.WorkspaceID)
		}
	}
}

func TestStore_IssueWatch_ListEnabled(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	enabled := newTestIssueWatch("ws-1")
	disabled := newTestIssueWatch("ws-2")
	disabled.Enabled = false
	for _, w := range []*IssueWatch{enabled, disabled} {
		if err := store.CreateIssueWatch(ctx, w); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	got, err := store.ListEnabledIssueWatches(ctx)
	if err != nil {
		t.Fatalf("list enabled: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected only the enabled watch, got %d", len(got))
	}
	if got[0].ID != enabled.ID {
		t.Errorf("expected enabled watch returned, got %s", got[0].ID)
	}
}

func TestStore_IssueWatch_Update(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	w := newTestIssueWatch("ws-1")
	if err := store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("create: %v", err)
	}
	originalCreated := w.CreatedAt
	w.JQL = "project = NEW"
	w.Enabled = false
	w.PollIntervalSeconds = 60
	if err := store.UpdateIssueWatch(ctx, w); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := store.GetIssueWatch(ctx, w.ID)
	if got.JQL != "project = NEW" || got.Enabled || got.PollIntervalSeconds != 60 {
		t.Errorf("update did not persist: %+v", got)
	}
	if !got.CreatedAt.Equal(originalCreated) {
		t.Errorf("update must not change created_at: %v vs %v", got.CreatedAt, originalCreated)
	}
}

func TestStore_IssueWatch_LastPolledStamp(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	w := newTestIssueWatch("ws-1")
	if err := store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("create: %v", err)
	}
	t1 := time.Now().UTC().Truncate(time.Second)
	if err := store.UpdateIssueWatchLastPolled(ctx, w.ID, t1); err != nil {
		t.Fatalf("stamp: %v", err)
	}
	got, _ := store.GetIssueWatch(ctx, w.ID)
	if got.LastPolledAt == nil || !got.LastPolledAt.Equal(t1) {
		t.Errorf("expected last_polled_at=%v, got %v", t1, got.LastPolledAt)
	}
}

func TestStore_IssueWatch_DeleteCascadesDedupRows(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	w := newTestIssueWatch("ws-1")
	if err := store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := store.ReserveIssueWatchTask(ctx, w.ID, "PROJ-1", "https://example.atlassian.net/browse/PROJ-1"); err != nil {
		t.Fatalf("reserve: %v", err)
	}

	if err := store.DeleteIssueWatch(ctx, w.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	gone, _ := store.GetIssueWatch(ctx, w.ID)
	if gone != nil {
		t.Errorf("expected watch deleted, got %+v", gone)
	}
	// Dedup row must be gone too — re-creating a watch with the same ID and
	// reserving the same key should succeed without a UNIQUE collision.
	w2 := newTestIssueWatch("ws-1")
	w2.ID = w.ID
	if err := store.CreateIssueWatch(ctx, w2); err != nil {
		t.Fatalf("recreate watch: %v", err)
	}
	ok, err := store.ReserveIssueWatchTask(ctx, w2.ID, "PROJ-1", "https://example.atlassian.net/browse/PROJ-1")
	if err != nil {
		t.Fatalf("re-reserve: %v", err)
	}
	if !ok {
		t.Error("expected re-reserve to succeed after cascade delete")
	}
}

func TestStore_IssueWatchTask_ReserveDedup(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	w := newTestIssueWatch("ws-1")
	if err := store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("create: %v", err)
	}

	// First reservation wins.
	first, err := store.ReserveIssueWatchTask(ctx, w.ID, "PROJ-7", "https://example.atlassian.net/browse/PROJ-7")
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	if !first {
		t.Error("expected first reservation to win")
	}

	// Second reservation for the same (watch, key) loses (dedup).
	second, err := store.ReserveIssueWatchTask(ctx, w.ID, "PROJ-7", "https://example.atlassian.net/browse/PROJ-7")
	if err != nil {
		t.Fatalf("second reserve: %v", err)
	}
	if second {
		t.Error("expected second reservation to lose due to UNIQUE constraint")
	}

	// HasIssueWatchTask reflects the reservation regardless of who won.
	has, err := store.HasIssueWatchTask(ctx, w.ID, "PROJ-7")
	if err != nil {
		t.Fatalf("has: %v", err)
	}
	if !has {
		t.Error("expected has=true after reservation")
	}
}

func TestStore_IssueWatchTask_AssignTaskID(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	w := newTestIssueWatch("ws-1")
	if err := store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := store.ReserveIssueWatchTask(ctx, w.ID, "PROJ-1", "https://example.atlassian.net/browse/PROJ-1"); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := store.AssignIssueWatchTaskID(ctx, w.ID, "PROJ-1", "task-abc"); err != nil {
		t.Fatalf("assign: %v", err)
	}

	// Assigning to a missing reservation surfaces a clear error.
	err := store.AssignIssueWatchTaskID(ctx, w.ID, "PROJ-NOPE", "task-zzz")
	if err == nil {
		t.Error("expected error for missing reservation, got nil")
	}
}

func TestStore_IssueWatchTask_ListSeenIssueKeys(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	w := newTestIssueWatch("ws-1")
	if err := store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("create: %v", err)
	}
	for _, k := range []string{"PROJ-1", "PROJ-2", "PROJ-3"} {
		if _, err := store.ReserveIssueWatchTask(ctx, w.ID, k, "https://x/"+k); err != nil {
			t.Fatalf("reserve %s: %v", k, err)
		}
	}

	seen, err := store.ListSeenIssueKeys(ctx, w.ID)
	if err != nil {
		t.Fatalf("list seen: %v", err)
	}
	for _, k := range []string{"PROJ-1", "PROJ-2", "PROJ-3"} {
		if _, ok := seen[k]; !ok {
			t.Errorf("expected %s in seen set, got %v", k, seen)
		}
	}
	if _, ok := seen["PROJ-99"]; ok {
		t.Error("expected PROJ-99 not in seen set")
	}
}

func TestStore_IssueWatchTask_Release(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	w := newTestIssueWatch("ws-1")
	if err := store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := store.ReserveIssueWatchTask(ctx, w.ID, "PROJ-1", "https://example.atlassian.net/browse/PROJ-1"); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := store.ReleaseIssueWatchTask(ctx, w.ID, "PROJ-1"); err != nil {
		t.Fatalf("release: %v", err)
	}
	// After release, the slot can be reserved again.
	again, err := store.ReserveIssueWatchTask(ctx, w.ID, "PROJ-1", "https://example.atlassian.net/browse/PROJ-1")
	if err != nil {
		t.Fatalf("re-reserve: %v", err)
	}
	if !again {
		t.Error("expected reservation to succeed after release")
	}
}
