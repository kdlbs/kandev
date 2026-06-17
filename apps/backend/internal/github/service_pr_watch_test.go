package github

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// countEventsOfType returns the number of mockEventBus events whose Type
// matches the supplied subject. The bus carries every github.* event the
// service emits, so PR-merge tests have to filter rather than trust the raw
// total — a sync that publishes both task_pr.updated and pr_merged otherwise
// looks like 2 merge events.
func countEventsOfType(eb *mockEventBus, eventType string) int {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	n := 0
	for _, e := range eb.events {
		if e != nil && e.Type == eventType {
			n++
		}
	}
	return n
}

// findEventOfType returns the first event of the given type, or nil if none.
func findEventOfType(eb *mockEventBus, eventType string) *bus.Event {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	for _, e := range eb.events {
		if e != nil && e.Type == eventType {
			return e
		}
	}
	return nil
}

// TestSyncTaskPR_PublishesPRMergedOnNilToTimeTransition exercises the
// acceptance criteria of the GitHubPRMerged publisher:
//   - First sync with merged_at=nil publishes no merge event.
//   - Second sync where merged_at goes from nil to a concrete time publishes
//     exactly one GitHubPRMerged event.
//   - Third sync with an unchanged merged_at must publish no further merge
//     event (idempotency — without this guarantee, every poll cycle after a
//     merge would re-fire the trigger downstream).
func TestSyncTaskPR_PublishesPRMergedOnNilToTimeTransition(t *testing.T) {
	svc, store, eb := setupSyncTest(t)
	ctx := context.Background()

	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID:     "t1",
		Owner:      "owner",
		Repo:       "repo",
		PRNumber:   42,
		PRURL:      "https://github.com/owner/repo/pull/42",
		PRTitle:    "Feature",
		HeadBranch: "feat",
		BaseBranch: "main",
		State:      "open",
	}); err != nil {
		t.Fatalf("create task PR: %v", err)
	}

	// First sync: PR still open, merged_at=nil. The task_pr.updated event
	// fires because the row is brand new with no review state, but no
	// pr_merged event should appear.
	openStatus := &PRStatus{
		PR: &PR{
			Number:    42,
			Title:     "Feature",
			State:     "open",
			RepoOwner: "owner",
			RepoName:  "repo",
		},
	}
	if err := svc.SyncTaskPR(ctx, "t1", openStatus); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if got := countEventsOfType(eb, events.GitHubPRMerged); got != 0 {
		t.Fatalf("expected no GitHubPRMerged event when PR is still open, got %d", got)
	}

	// Second sync: merged_at transitions from nil to a concrete time.
	mergedAt := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	mergedStatus := &PRStatus{
		PR: &PR{
			Number:    42,
			Title:     "Feature",
			State:     "merged",
			RepoOwner: "owner",
			RepoName:  "repo",
			MergedAt:  &mergedAt,
		},
	}
	if err := svc.SyncTaskPR(ctx, "t1", mergedStatus); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if got := countEventsOfType(eb, events.GitHubPRMerged); got != 1 {
		t.Fatalf("expected exactly 1 GitHubPRMerged event on nil->time transition, got %d", got)
	}

	// Inspect the payload so downstream subscribers can rely on the contract.
	evt := findEventOfType(eb, events.GitHubPRMerged)
	if evt == nil {
		t.Fatal("expected to find the GitHubPRMerged event")
	}
	payload, ok := evt.Data.(events.GitHubPRMergedEvent)
	if !ok {
		t.Fatalf("expected payload of type events.GitHubPRMergedEvent, got %T", evt.Data)
	}
	if payload.TaskID != "t1" {
		t.Errorf("payload TaskID = %q, want %q", payload.TaskID, "t1")
	}
	if payload.PRNumber != 42 {
		t.Errorf("payload PRNumber = %d, want %d", payload.PRNumber, 42)
	}
	if !payload.MergedAt.Equal(mergedAt) {
		t.Errorf("payload MergedAt = %v, want %v", payload.MergedAt, mergedAt)
	}

	// Third sync: identical merged_at — no additional merge event.
	if err := svc.SyncTaskPR(ctx, "t1", mergedStatus); err != nil {
		t.Fatalf("third sync: %v", err)
	}
	if got := countEventsOfType(eb, events.GitHubPRMerged); got != 1 {
		t.Errorf("expected idempotency: still 1 GitHubPRMerged event after unchanged sync, got %d", got)
	}
}

// TestSyncTaskPR_PRMergedNotPublishedWhenAlreadyMerged covers the case where
// the row's MergedAt was already populated (e.g. PR imported via URL after a
// merge had already happened). The transition guard is "nil -> non-nil", so
// even though the value is non-nil after this sync, no event should fire.
func TestSyncTaskPR_PRMergedNotPublishedWhenAlreadyMerged(t *testing.T) {
	svc, store, eb := setupSyncTest(t)
	ctx := context.Background()

	mergedAt := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID:     "t1",
		Owner:      "owner",
		Repo:       "repo",
		PRNumber:   7,
		PRURL:      "https://github.com/owner/repo/pull/7",
		PRTitle:    "Already merged",
		HeadBranch: "feat",
		BaseBranch: "main",
		State:      "merged",
		MergedAt:   &mergedAt,
	}); err != nil {
		t.Fatalf("create task PR: %v", err)
	}

	status := &PRStatus{
		PR: &PR{
			Number:    7,
			Title:     "Already merged",
			State:     "merged",
			RepoOwner: "owner",
			RepoName:  "repo",
			MergedAt:  &mergedAt,
		},
	}
	if err := svc.SyncTaskPR(ctx, "t1", status); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := countEventsOfType(eb, events.GitHubPRMerged); got != 0 {
		t.Errorf("expected no merge event when MergedAt was already set, got %d", got)
	}
}
