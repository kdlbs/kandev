package github

import (
	"context"
	"strings"
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

// taskPROutcomeColumnNames is the AC-07 checklist: every one of the eight
// outcome-attribution columns must appear in taskPRColumns,
// taskPRColumnsQualified, and the CreateTaskPR/ReplaceTaskPR INSERT column
// lists, or a read/write path silently regresses to losing the column.
var taskPROutcomeColumnNames = []string{
	"is_draft", "changed_files", "merged_by_login", "closed_by_login",
	"auto_merge_observed_at", "disposition", "disposition_superseded_by_url",
	"disposition_recorded_at",
}

// TestTaskPRColumnLists_IncludeAllOutcomeColumns covers AC-07: every outcome
// column is present in both the qualified and unqualified read projections.
func TestTaskPRColumnLists_IncludeAllOutcomeColumns(t *testing.T) {
	for _, name := range taskPROutcomeColumnNames {
		if !strings.Contains(taskPRColumns, name) {
			t.Errorf("taskPRColumns missing %q", name)
		}
		if !strings.Contains(taskPRColumnsQualified, "gtp."+name) {
			t.Errorf("taskPRColumnsQualified missing qualified %q", name)
		}
	}
}

// TestCreateAndReplaceTaskPR_RoundTripOutcomeColumns covers the write half
// of AC-07: CreateTaskPR and ReplaceTaskPR must persist all eight outcome
// columns, not just the pre-existing ones, or a linked PR loses any
// upstream-observed or human-recorded outcome data the moment it is
// (re)written.
func TestCreateAndReplaceTaskPR_RoundTripOutcomeColumns(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.db.Exec(`INSERT INTO tasks (id, workspace_id) VALUES ('task-roundtrip', 'ws-roundtrip')`); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	isDraft := true
	changedFiles := 7
	mergedBy := "carlosflorencio"
	closedBy := "nova28"
	autoMergeAt := time.Now().UTC().Truncate(time.Second)
	disposition := TaskPRDispositionExploratory
	supersededURL := "https://github.com/kdlbs/kandev/pull/4242"
	recordedAt := autoMergeAt

	created := &TaskPR{
		WorkspaceID:                "ws-roundtrip",
		TaskID:                     "task-roundtrip",
		Owner:                      "kdlbs",
		Repo:                       "kandev",
		PRNumber:                   5001,
		PRURL:                      "https://github.com/kdlbs/kandev/pull/5001",
		PRTitle:                    "outcome column round trip",
		State:                      "closed",
		CreatedAt:                  time.Now().UTC(),
		IsDraft:                    &isDraft,
		ChangedFiles:               &changedFiles,
		MergedByLogin:              &mergedBy,
		ClosedByLogin:              &closedBy,
		AutoMergeObservedAt:        &autoMergeAt,
		Disposition:                &disposition,
		DispositionSupersededByURL: &supersededURL,
		DispositionRecordedAt:      &recordedAt,
	}
	if err := store.CreateTaskPR(ctx, created); err != nil {
		t.Fatalf("CreateTaskPR: %v", err)
	}
	gotCreated, err := store.GetTaskPRByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID after CreateTaskPR: %v", err)
	}
	assertOutcomeColumnsRoundTrip(t, "CreateTaskPR", gotCreated, isDraft, changedFiles, mergedBy, closedBy, disposition, supersededURL)

	replaced := &TaskPR{
		WorkspaceID:                "ws-roundtrip",
		TaskID:                     "task-roundtrip",
		Owner:                      "kdlbs",
		Repo:                       "kandev",
		PRNumber:                   5001,
		PRURL:                      "https://github.com/kdlbs/kandev/pull/5001",
		PRTitle:                    "outcome column round trip (replaced)",
		State:                      "closed",
		CreatedAt:                  time.Now().UTC(),
		IsDraft:                    &isDraft,
		ChangedFiles:               &changedFiles,
		MergedByLogin:              &mergedBy,
		ClosedByLogin:              &closedBy,
		AutoMergeObservedAt:        &autoMergeAt,
		Disposition:                &disposition,
		DispositionSupersededByURL: &supersededURL,
		DispositionRecordedAt:      &recordedAt,
	}
	if err := store.ReplaceTaskPR(ctx, replaced); err != nil {
		t.Fatalf("ReplaceTaskPR: %v", err)
	}
	gotReplaced, err := store.GetTaskPRByRepoAndNumber(ctx, "task-roundtrip", "", 5001)
	if err != nil {
		t.Fatalf("GetTaskPRByRepoAndNumber after ReplaceTaskPR: %v", err)
	}
	assertOutcomeColumnsRoundTrip(t, "ReplaceTaskPR", gotReplaced, isDraft, changedFiles, mergedBy, closedBy, disposition, supersededURL)
}

func assertOutcomeColumnsRoundTrip(
	t *testing.T, writer string, got *TaskPR,
	wantIsDraft bool, wantChangedFiles int, wantMergedBy, wantClosedBy, wantDisposition, wantSupersededURL string,
) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: row not found", writer)
	}
	if got.IsDraft == nil || *got.IsDraft != wantIsDraft {
		t.Errorf("%s: IsDraft = %v, want %v", writer, got.IsDraft, wantIsDraft)
	}
	if got.ChangedFiles == nil || *got.ChangedFiles != wantChangedFiles {
		t.Errorf("%s: ChangedFiles = %v, want %v", writer, got.ChangedFiles, wantChangedFiles)
	}
	if got.MergedByLogin == nil || *got.MergedByLogin != wantMergedBy {
		t.Errorf("%s: MergedByLogin = %v, want %v", writer, got.MergedByLogin, wantMergedBy)
	}
	if got.ClosedByLogin == nil || *got.ClosedByLogin != wantClosedBy {
		t.Errorf("%s: ClosedByLogin = %v, want %v", writer, got.ClosedByLogin, wantClosedBy)
	}
	if got.AutoMergeObservedAt == nil {
		t.Errorf("%s: AutoMergeObservedAt = nil, want set", writer)
	}
	if got.Disposition == nil || *got.Disposition != wantDisposition {
		t.Errorf("%s: Disposition = %v, want %v", writer, got.Disposition, wantDisposition)
	}
	if got.DispositionSupersededByURL == nil || *got.DispositionSupersededByURL != wantSupersededURL {
		t.Errorf("%s: DispositionSupersededByURL = %v, want %v", writer, got.DispositionSupersededByURL, wantSupersededURL)
	}
	if got.DispositionRecordedAt == nil {
		t.Errorf("%s: DispositionRecordedAt = nil, want set", writer)
	}
}
