package github

import (
	"context"
	"testing"
	"time"
)

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001.3
func TestAssociatePRWithTaskPersistsHeadSHAImmediately(t *testing.T) {
	svc, store, _ := setupSyncTest(t)
	ctx := context.Background()
	pr := &PR{
		Number: 42, HTMLURL: "https://example.test/42", Title: "queued", HeadBranch: "feature",
		BaseBranch: "main", State: "open", RepoOwner: "owner", RepoName: "repo", HeadSHA: "head-a",
	}

	associated, err := svc.AssociatePRWithTask(ctx, "task-queue-recovery-association", "", pr)
	if err != nil {
		t.Fatalf("associate PR: %v", err)
	}
	if associated.HeadSHA != "head-a" {
		t.Fatalf("associated head SHA = %q, want head-a", associated.HeadSHA)
	}

	persisted := mustGetTaskPR(t, store, ctx, associated.ID)
	if persisted.HeadSHA != "head-a" {
		t.Fatalf("persisted head SHA = %q, want head-a", persisted.HeadSHA)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001.2
// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001.4
func TestSyncTaskPRPreservesRecoveryAfterQueueEntryDisappears(t *testing.T) {
	svc, store, _ := setupSyncTest(t)
	ctx := context.Background()
	seed := &TaskPR{
		TaskID: "task-queue-recovery-sync", Owner: "owner", Repo: "repo", PRNumber: 42,
		PRURL: "https://example.test/42", PRTitle: "queued", State: "open", HeadSHA: "head-a",
		MergeQueueState: "queued", MergeQueueEntryID: "entry-a", MergeQueueEntryHeadSHA: "head-a",
		CreatedAt: nowUTC(),
	}
	if err := store.CreateTaskPR(ctx, seed); err != nil {
		t.Fatalf("seed TaskPR: %v", err)
	}
	removedAt := time.Date(2026, 8, 24, 18, 0, 0, 0, time.UTC)
	if err := svc.SyncTaskPR(ctx, seed.TaskID, &PRStatus{
		PR:              &PR{Number: 42, State: "open", RepoOwner: "owner", RepoName: "repo", HeadSHA: "head-a"},
		MergeQueueState: "queued", MergeQueueEntryID: "entry-a", MergeQueueEntryHeadSHA: "head-a",
		MergeQueueLastRemovalID: "removal-a", MergeQueueLastRemovedAt: &removedAt,
		MergeQueueLastRemovalReason: "CI checks failed", MergeQueueLastRemovalBeforeSHA: "before-a",
		mergeQueuePopulated: true, mergeQueueRecoveryPopulated: true,
	}); err != nil {
		t.Fatalf("initial queue sync: %v", err)
	}

	if err := svc.SyncTaskPR(ctx, seed.TaskID, &PRStatus{
		PR:                  &PR{Number: 42, State: "open", RepoOwner: "owner", RepoName: "repo", HeadSHA: "head-a"},
		mergeQueuePopulated: true, mergeQueueRecoveryPopulated: true,
	}); err != nil {
		t.Fatalf("queue removal sync: %v", err)
	}
	got := mustGetTaskPR(t, store, ctx, seed.ID)
	if got.MergeQueueState != "" || got.MergeQueueEntryID != "" || got.MergeQueueEntryHeadSHA != "" {
		t.Fatalf("active queue snapshot = %#v, want cleared after null entry", got)
	}
	if got.MergeQueueLastRemovalID != "removal-a" || got.MergeQueueLastRemovalReason != "CI checks failed" || got.MergeQueueLastRemovalBeforeSHA != "before-a" {
		t.Fatalf("removal evidence was lost after queue entry disappeared: %#v", got)
	}

	if err := svc.SyncTaskPR(ctx, seed.TaskID, &PRStatus{
		PR: &PR{Number: 42, State: "open", RepoOwner: "owner", RepoName: "repo"},
	}); err != nil {
		t.Fatalf("queue-unaware sync: %v", err)
	}
	got = mustGetTaskPR(t, store, ctx, seed.ID)
	if got.MergeQueueLastRemovalID != "removal-a" || got.HeadSHA != "head-a" {
		t.Fatalf("queue-unaware sync clobbered recovery state: %#v", got)
	}
}

func TestResolveTaskPRMergeQueueStateRejectsOlderOrUntimestampedRemoval(t *testing.T) {
	newerAt := time.Date(2026, 8, 24, 18, 0, 0, 0, time.UTC)
	current := &TaskPR{
		MergeQueueLastRemovalID:     "removal-new",
		MergeQueueLastRemovedAt:     &newerAt,
		MergeQueueLastRemovalReason: "new reason",
	}

	for _, test := range []struct {
		name      string
		removalID string
		removedAt *time.Time
	}{
		{
			name:      "older event",
			removalID: "removal-old",
			removedAt: func() *time.Time { value := newerAt.Add(-time.Minute); return &value }(),
		},
		{name: "missing timestamp", removalID: "removal-unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			queue := resolveTaskPRMergeQueueState(current, &PRStatus{
				PR:                          &PR{State: "open"},
				MergeQueueLastRemovalID:     test.removalID,
				MergeQueueLastRemovedAt:     test.removedAt,
				MergeQueueLastRemovalReason: "old reason",
				mergeQueueRecoveryPopulated: true,
			})
			if queue.lastRemovalID != "removal-new" || queue.lastRemovalReason != "new reason" || !timeEqual(queue.lastRemovedAt, &newerAt) {
				t.Fatalf("queue removal regressed: %+v", queue)
			}
		})
	}
}
