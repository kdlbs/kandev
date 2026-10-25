package github

import (
	"context"
	"strings"
	"testing"
	"time"
)

var taskPRMergeQueueRecoveryColumnNames = []string{
	"head_sha",
	"merge_queue_entry_id",
	"merge_queue_entry_head_sha",
	"merge_queue_last_removal_id",
	"merge_queue_last_removed_at",
	"merge_queue_last_removal_reason",
	"merge_queue_last_removal_before_sha",
}

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001.3
func TestTaskPRMergeQueueRecoverySchemaIsReplayable(t *testing.T) {
	store := newTestStore(t)
	columns, err := store.tableColumns("github_task_prs")
	if err != nil {
		t.Fatalf("read github_task_prs columns: %v", err)
	}
	for _, name := range taskPRMergeQueueRecoveryColumnNames {
		if _, ok := columns[name]; !ok {
			t.Errorf("github_task_prs is missing %q", name)
		}
	}
	if err := store.initSchema(false); err != nil {
		t.Fatalf("replay github schema: %v", err)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001.1
// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001.3
func TestTaskPRMergeQueueRecoveryRoundTripsThroughWritePaths(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	removedAt := time.Date(2026, 8, 24, 18, 0, 0, 0, time.UTC)
	seed := &TaskPR{
		TaskID: "task-queue-recovery", WorkspaceID: "workspace-queue-recovery",
		Owner: "owner", Repo: "repo", PRNumber: 42, PRURL: "https://example.test/42",
		PRTitle: "queued", HeadBranch: "feature", BaseBranch: "main", State: "open",
		HeadSHA: "head-a", MergeQueueState: "queued", MergeQueueEntryID: "entry-a",
		MergeQueueEntryHeadSHA: "head-a", MergeQueueLastRemovalID: "removal-a",
		MergeQueueLastRemovedAt: &removedAt, MergeQueueLastRemovalReason: "CI checks failed",
		MergeQueueLastRemovalBeforeSHA: "before-a", CreatedAt: removedAt,
	}
	if err := store.CreateTaskPR(ctx, seed); err != nil {
		t.Fatalf("CreateTaskPR: %v", err)
	}
	assertTaskPRMergeQueueRecoveryFields(t, "CreateTaskPR", mustGetTaskPR(t, store, ctx, seed.ID), seed)

	seed.HeadSHA = "head-b"
	seed.MergeQueueState = ""
	seed.MergeQueueEntryID = ""
	seed.MergeQueueEntryHeadSHA = ""
	if err := store.UpdateTaskPR(ctx, seed); err != nil {
		t.Fatalf("UpdateTaskPR: %v", err)
	}
	assertTaskPRMergeQueueRecoveryFields(t, "UpdateTaskPR", mustGetTaskPR(t, store, ctx, seed.ID), seed)

	replaced := &TaskPR{
		TaskID: seed.TaskID, WorkspaceID: seed.WorkspaceID, Owner: seed.Owner, Repo: seed.Repo,
		PRNumber: seed.PRNumber, PRURL: seed.PRURL, PRTitle: "replaced", HeadBranch: seed.HeadBranch,
		BaseBranch: seed.BaseBranch, State: "open", HeadSHA: "head-c",
		CreatedAt: removedAt,
	}
	newRemovedAt := removedAt.Add(time.Minute)
	if _, err := store.ReplaceTaskPR(ctx, replaced, &PRStatus{
		PR:                      &PR{Number: seed.PRNumber, State: "open", RepoOwner: seed.Owner, RepoName: seed.Repo, HeadSHA: "head-c"},
		MergeQueueLastRemovalID: "removal-b", MergeQueueLastRemovedAt: &newRemovedAt,
		MergeQueueLastRemovalReason: "merge conflict", MergeQueueLastRemovalBeforeSHA: "before-b",
		mergeQueueRecoveryPopulated: true,
	}); err != nil {
		t.Fatalf("ReplaceTaskPR: %v", err)
	}
	wantReplacement := *replaced
	wantReplacement.MergeQueueLastRemovalID = "removal-b"
	wantReplacement.MergeQueueLastRemovedAt = &newRemovedAt
	wantReplacement.MergeQueueLastRemovalReason = "merge conflict"
	wantReplacement.MergeQueueLastRemovalBeforeSHA = "before-b"
	assertTaskPRMergeQueueRecoveryFields(t, "ReplaceTaskPR", mustGetTaskPR(t, store, ctx, replaced.ID), &wantReplacement)

	if _, changed, err := store.DetachTaskPR(ctx, replaced.ID); err != nil || !changed {
		t.Fatalf("DetachTaskPR: changed=%v err=%v", changed, err)
	}
	restored, err := store.RestoreTaskPR(ctx, replaced.TaskID, replaced.RepositoryID, &PRStatus{
		PR: &PR{Number: replaced.PRNumber, RepoOwner: replaced.Owner, RepoName: replaced.Repo, HTMLURL: replaced.PRURL, Title: replaced.PRTitle, State: "open"},
	})
	if err != nil {
		t.Fatalf("RestoreTaskPR: %v", err)
	}
	assertTaskPRMergeQueueRecoveryFields(t, "RestoreTaskPR", restored, &wantReplacement)
}

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001.3
func TestTaskPRColumnListsIncludeMergeQueueRecoveryColumns(t *testing.T) {
	for _, name := range taskPRMergeQueueRecoveryColumnNames {
		if !strings.Contains(taskPRColumns, name) {
			t.Errorf("taskPRColumns missing %q", name)
		}
		if !strings.Contains(taskPRColumnsQualified, "gtp."+name) {
			t.Errorf("taskPRColumnsQualified missing %q", name)
		}
	}
}

func TestTaskPRInsertArityMatchesColumnList(t *testing.T) {
	if got, want := taskPRColumnCount(), len(taskPRValues(&TaskPR{})); got != want {
		t.Fatalf("TaskPR insert arity = %d columns and %d values, want equal", got, want)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001.4
func TestUpdateTaskPRPreservesNewerQueueRemovalObservation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	base := &TaskPR{
		TaskID: "task-queue-recovery-order", Owner: "owner", Repo: "repo", PRNumber: 42,
		PRURL: "https://example.test/42", State: "open", HeadSHA: "head-a", CreatedAt: nowUTC(),
	}
	if err := store.CreateTaskPR(ctx, base); err != nil {
		t.Fatalf("seed TaskPR: %v", err)
	}

	newerAt := time.Date(2026, 8, 24, 18, 0, 0, 0, time.UTC)
	olderAt := time.Date(2026, 8, 24, 19, 0, 0, 0, time.FixedZone("WEST", 2*60*60))
	older := *base
	older.MergeQueueLastRemovalID = "removal-old"
	older.MergeQueueLastRemovedAt = &olderAt
	older.MergeQueueLastRemovalReason = "old reason"
	older.MergeQueueLastRemovalBeforeSHA = "before-old"
	newer := older
	newer.MergeQueueLastRemovalID = "removal-new"
	newer.MergeQueueLastRemovedAt = &newerAt
	newer.MergeQueueLastRemovalReason = "new reason"
	newer.MergeQueueLastRemovalBeforeSHA = "before-new"

	if err := store.UpdateTaskPR(ctx, &newer); err != nil {
		t.Fatalf("write newer removal observation: %v", err)
	}
	if err := store.UpdateTaskPR(ctx, &older); err != nil {
		t.Fatalf("write delayed older removal observation: %v", err)
	}

	got := mustGetTaskPR(t, store, ctx, base.ID)
	if got.MergeQueueLastRemovalID != "removal-new" || got.MergeQueueLastRemovalReason != "new reason" ||
		got.MergeQueueLastRemovalBeforeSHA != "before-new" || !timeEqual(got.MergeQueueLastRemovedAt, &newerAt) {
		t.Fatalf("delayed observation regressed removal evidence: %#v", got)
	}
}

func assertTaskPRMergeQueueRecoveryFields(t *testing.T, name string, got, want *TaskPR) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: got nil TaskPR", name)
	}
	if got.HeadSHA != want.HeadSHA || got.MergeQueueEntryID != want.MergeQueueEntryID || got.MergeQueueEntryHeadSHA != want.MergeQueueEntryHeadSHA {
		t.Errorf("%s: active snapshot = %#v, want head=%q entry=%q entry_head=%q", name, got, want.HeadSHA, want.MergeQueueEntryID, want.MergeQueueEntryHeadSHA)
	}
	if got.MergeQueueLastRemovalID != want.MergeQueueLastRemovalID || got.MergeQueueLastRemovalReason != want.MergeQueueLastRemovalReason || got.MergeQueueLastRemovalBeforeSHA != want.MergeQueueLastRemovalBeforeSHA {
		t.Errorf("%s: removal snapshot = %#v, want id=%q reason=%q before=%q", name, got, want.MergeQueueLastRemovalID, want.MergeQueueLastRemovalReason, want.MergeQueueLastRemovalBeforeSHA)
	}
	if !timeEqual(got.MergeQueueLastRemovedAt, want.MergeQueueLastRemovedAt) {
		t.Errorf("%s: removal time = %v, want %v", name, got.MergeQueueLastRemovedAt, want.MergeQueueLastRemovedAt)
	}
}
