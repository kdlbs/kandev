package github

import (
	"context"
	"testing"
	"time"
)

// TestTaskPRReads_ToleratesExtraColumn locks in resilience against a
// specific production incident: the "Link GitHub pull request" dialog
// failed with "associate PR with task: missing destination name workspace_id
// in *github.TaskPR" for a task in a workspace whose github_task_prs table
// had picked up a column (via a schema migration from a newer release, e.g.
// a self-update that was later rolled back) that the running binary's
// TaskPR struct doesn't declare. sqlx's StructScan errors out on any SELECT
// * column with no matching destination field, so every TaskPR read query
// must project an explicit column list rather than `SELECT *` /
// `SELECT gtp.*` — otherwise ANY future schema drift ahead of the binary
// breaks every read path, not just the one column that happened to trigger
// the original report.
func TestTaskPRReads_ToleratesExtraColumn(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Simulate schema drift: a column present in the table that the
	// current TaskPR struct has no field for.
	if _, err := store.db.Exec(
		`ALTER TABLE github_task_prs ADD COLUMN future_only_column TEXT NOT NULL DEFAULT ''`,
	); err != nil {
		t.Fatalf("simulate schema drift: %v", err)
	}
	if _, err := store.db.Exec(
		`INSERT INTO tasks (id, workspace_id) VALUES ('task-drift', 'ws-drift')`,
	); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	tp := &TaskPR{
		WorkspaceID: "ws-drift",
		TaskID:      "task-drift",
		Owner:       "kdlbs",
		Repo:        "kandev",
		PRNumber:    1978,
		PRURL:       "https://github.com/kdlbs/kandev/pull/1978",
		PRTitle:     "drifted schema",
		State:       "open",
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.ReplaceTaskPR(ctx, tp); err != nil {
		t.Fatalf("ReplaceTaskPR: %v", err)
	}

	fatalOnScanError := func(name string, err error) {
		t.Helper()
		if err == nil {
			return
		}
		t.Fatalf("%s: SELECT scan broke on schema drift: %v", name, err)
	}

	got, err := store.GetTaskPR(ctx, "task-drift")
	fatalOnScanError("GetTaskPR", err)
	if got == nil || got.PRNumber != 1978 || got.WorkspaceID != "ws-drift" {
		t.Fatalf("GetTaskPR: expected PR #1978 in ws-drift, got %+v", got)
	}

	gotByRepo, err := store.GetTaskPRByRepository(ctx, "task-drift", "")
	fatalOnScanError("GetTaskPRByRepository", err)
	if gotByRepo == nil || gotByRepo.PRNumber != 1978 {
		t.Fatalf("GetTaskPRByRepository: expected PR #1978, got %+v", gotByRepo)
	}

	gotByNumber, err := store.GetTaskPRByRepoAndNumber(ctx, "task-drift", "", 1978)
	fatalOnScanError("GetTaskPRByRepoAndNumber", err)
	if gotByNumber == nil || gotByNumber.PRTitle != "drifted schema" {
		t.Fatalf("GetTaskPRByRepoAndNumber: expected the drifted-schema PR, got %+v", gotByNumber)
	}

	list, err := store.ListTaskPRsByTask(ctx, "task-drift")
	fatalOnScanError("ListTaskPRsByTask", err)
	if len(list) != 1 || list[0].PRNumber != 1978 {
		t.Fatalf("ListTaskPRsByTask: expected 1 PR (#1978), got %+v", list)
	}

	byIDs, err := store.ListTaskPRsByTaskIDs(ctx, []string{"task-drift"})
	fatalOnScanError("ListTaskPRsByTaskIDs", err)
	if len(byIDs["task-drift"]) != 1 || byIDs["task-drift"][0].PRNumber != 1978 {
		t.Fatalf("ListTaskPRsByTaskIDs: expected 1 PR (#1978) for task-drift, got %+v", byIDs)
	}

	byWorkspace, err := store.ListTaskPRsByWorkspaceID(ctx, "ws-drift")
	fatalOnScanError("ListTaskPRsByWorkspaceID", err)
	if len(byWorkspace["task-drift"]) != 1 || byWorkspace["task-drift"][0].PRNumber != 1978 {
		t.Fatalf("ListTaskPRsByWorkspaceID: expected 1 PR (#1978) for task-drift, got %+v", byWorkspace)
	}
}
