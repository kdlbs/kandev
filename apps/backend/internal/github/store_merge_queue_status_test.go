package github

import (
	"context"
	"strings"
	"testing"
	"time"
)

var taskPRMergeQueueColumnNames = []string{
	"merge_queue_state",
	"merge_queue_position",
	"merge_queue_estimated_time_to_merge_seconds",
}

func TestTaskPRMergeQueueSchemaIsReplayable(t *testing.T) {
	store := newTestStore(t)

	columns, err := store.tableColumns("github_task_prs")
	if err != nil {
		t.Fatalf("read github_task_prs columns: %v", err)
	}
	for _, name := range taskPRMergeQueueColumnNames {
		if _, ok := columns[name]; !ok {
			t.Errorf("github_task_prs is missing %q", name)
		}
	}
	if err := store.initSchema(false); err != nil {
		t.Fatalf("replay github schema: %v", err)
	}
}

func TestTaskPRMergeQueueColumnsRoundTripEveryWritePath(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	position := 4
	estimate := 125
	now := time.Now().UTC().Truncate(time.Second)
	seed := &TaskPR{
		TaskID: "task-queue-columns", WorkspaceID: "workspace-queue-columns",
		Owner: "owner", Repo: "repo", PRNumber: 42, PRURL: "https://example.test/42",
		PRTitle: "queued", HeadBranch: "feature", BaseBranch: "main", State: "open",
		MergeQueueState: "queued", MergeQueuePosition: &position,
		MergeQueueEstimatedTimeToMergeSeconds: &estimate, CreatedAt: now,
	}
	if err := store.CreateTaskPR(ctx, seed); err != nil {
		t.Fatalf("CreateTaskPR: %v", err)
	}
	assertTaskPRMergeQueueFields(t, "CreateTaskPR", mustGetTaskPR(t, store, ctx, seed.ID), "queued", &position, &estimate)

	position = 5
	estimate = 185
	seed.MergeQueueState = "awaiting_checks"
	if err := store.UpdateTaskPR(ctx, seed); err != nil {
		t.Fatalf("UpdateTaskPR: %v", err)
	}
	assertTaskPRMergeQueueFields(t, "UpdateTaskPR", mustGetTaskPR(t, store, ctx, seed.ID), "awaiting_checks", &position, &estimate)

	replacedPosition := 2
	replacedEstimate := 60
	replaced := &TaskPR{
		TaskID: seed.TaskID, WorkspaceID: seed.WorkspaceID, Owner: seed.Owner, Repo: seed.Repo,
		PRNumber: seed.PRNumber, PRURL: seed.PRURL, PRTitle: "replaced", HeadBranch: seed.HeadBranch,
		BaseBranch: seed.BaseBranch, State: "open", CreatedAt: now,
		MergeQueueState: "mergeable", MergeQueuePosition: &replacedPosition,
		MergeQueueEstimatedTimeToMergeSeconds: &replacedEstimate,
	}
	if _, err := store.ReplaceTaskPR(ctx, replaced, &PRStatus{
		PR:              &PR{State: "open"},
		MergeQueueState: "mergeable", MergeQueuePosition: &replacedPosition,
		MergeQueueEstimatedTimeToMergeSeconds: &replacedEstimate, mergeQueuePopulated: true,
	}); err != nil {
		t.Fatalf("ReplaceTaskPR: %v", err)
	}
	assertTaskPRMergeQueueFields(t, "ReplaceTaskPR", mustGetTaskPR(t, store, ctx, replaced.ID), "mergeable", &replacedPosition, &replacedEstimate)
}

func TestTaskPRColumnListsIncludeMergeQueueColumns(t *testing.T) {
	for _, name := range taskPRMergeQueueColumnNames {
		if !strings.Contains(taskPRColumns, name) {
			t.Errorf("taskPRColumns missing %q", name)
		}
		if !strings.Contains(taskPRColumnsQualified, "gtp."+name) {
			t.Errorf("taskPRColumnsQualified missing %q", name)
		}
	}
}

func TestRestoreTaskPRClearsQueueFieldsFromQueueUnawareRead(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	position := 3
	estimate := 90
	seed := &TaskPR{
		TaskID: "task-queue-restore", WorkspaceID: "workspace-queue-restore",
		Owner: "owner", Repo: "repo", PRNumber: 43, PRURL: "https://example.test/43",
		PRTitle: "queued restore", State: "open", CreatedAt: time.Now().UTC(),
		MergeQueueState: "awaiting_checks", MergeQueuePosition: &position,
		MergeQueueEstimatedTimeToMergeSeconds: &estimate,
	}
	if err := store.CreateTaskPR(ctx, seed); err != nil {
		t.Fatalf("CreateTaskPR: %v", err)
	}
	if _, changed, err := store.DetachTaskPR(ctx, seed.ID); err != nil || !changed {
		t.Fatalf("DetachTaskPR: changed=%v err=%v", changed, err)
	}

	restored, err := store.RestoreTaskPR(ctx, seed.TaskID, seed.RepositoryID, &PRStatus{
		PR: &PR{Number: seed.PRNumber, RepoOwner: seed.Owner, RepoName: seed.Repo, HTMLURL: seed.PRURL, Title: seed.PRTitle, State: "open"},
	})
	if err != nil {
		t.Fatalf("RestoreTaskPR: %v", err)
	}
	assertTaskPRMergeQueueFields(t, "RestoreTaskPR", restored, "", nil, nil)
}

func mustGetTaskPR(t *testing.T, store *Store, ctx context.Context, id string) *TaskPR {
	t.Helper()
	got, err := store.GetTaskPRByID(ctx, id)
	if err != nil {
		t.Fatalf("GetTaskPRByID: %v", err)
	}
	if got == nil {
		t.Fatalf("TaskPR %q not found", id)
	}
	return got
}

func assertTaskPRMergeQueueFields(t *testing.T, name string, got *TaskPR, state string, position, estimate *int) {
	t.Helper()
	if got.MergeQueueState != state {
		t.Errorf("%s: MergeQueueState = %q, want %q", name, got.MergeQueueState, state)
	}
	if !intPtrEqual(got.MergeQueuePosition, position) {
		t.Errorf("%s: MergeQueuePosition = %v, want %v", name, got.MergeQueuePosition, position)
	}
	if !intPtrEqual(got.MergeQueueEstimatedTimeToMergeSeconds, estimate) {
		t.Errorf("%s: MergeQueueEstimatedTimeToMergeSeconds = %v, want %v", name, got.MergeQueueEstimatedTimeToMergeSeconds, estimate)
	}
}
