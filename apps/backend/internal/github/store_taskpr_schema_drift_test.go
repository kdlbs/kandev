package github

import (
	"context"
	"strings"
	"sync"
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
	if _, err := store.ReplaceTaskPR(ctx, tp, &PRStatus{}); err != nil {
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

// taskPROutcomeColumnNames is the AC-07 checklist: every one of the five
// outcome-attribution columns must appear in taskPRColumns,
// taskPRColumnsQualified, and the CreateTaskPR/ReplaceTaskPR INSERT column
// lists, or a read/write path silently regresses to losing the column.
var taskPROutcomeColumnNames = []string{
	"is_draft", "changed_files", "merged_by_login", "closed_by_login",
	"auto_merge_observed_at",
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
// of AC-07: CreateTaskPR and ReplaceTaskPR must persist all five outcome
// columns, not just the pre-existing ones, or a linked PR loses any
// upstream-observed outcome data the moment it is (re)written.
//
// ReplaceTaskPR resolves the five columns itself from a *PRStatus
// observation (AC-43), rather than trusting a caller-supplied TaskPR's own
// field values, so the "replaced" case below drives it with an explicit
// populating status instead of presetting replaced's outcome fields.
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

	created := &TaskPR{
		WorkspaceID:         "ws-roundtrip",
		TaskID:              "task-roundtrip",
		Owner:               "kdlbs",
		Repo:                "kandev",
		PRNumber:            5001,
		PRURL:               "https://github.com/kdlbs/kandev/pull/5001",
		PRTitle:             "outcome column round trip",
		State:               "closed",
		CreatedAt:           time.Now().UTC(),
		IsDraft:             &isDraft,
		ChangedFiles:        &changedFiles,
		MergedByLogin:       &mergedBy,
		ClosedByLogin:       &closedBy,
		AutoMergeObservedAt: &autoMergeAt,
	}
	if err := store.CreateTaskPR(ctx, created); err != nil {
		t.Fatalf("CreateTaskPR: %v", err)
	}
	gotCreated, err := store.GetTaskPRByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID after CreateTaskPR: %v", err)
	}
	assertOutcomeColumnsRoundTrip(t, "CreateTaskPR", gotCreated, isDraft, changedFiles, mergedBy, closedBy)

	replaced := &TaskPR{
		WorkspaceID: "ws-roundtrip",
		TaskID:      "task-roundtrip",
		Owner:       "kdlbs",
		Repo:        "kandev",
		PRNumber:    5001,
		PRURL:       "https://github.com/kdlbs/kandev/pull/5001",
		PRTitle:     "outcome column round trip (replaced)",
		State:       "closed",
		CreatedAt:   time.Now().UTC(),
	}
	status := &PRStatus{
		PR: &PR{
			Draft:                isDraft,
			IsDraftObserved:      true,
			ChangedFiles:         changedFiles,
			ChangedFilesObserved: true,
			MergedByLogin:        mergedBy,
			AutoMergeEnabled:     true,
		},
		OutcomeFieldsPopulated:      true,
		ClosedByLogin:               closedBy,
		ClosureAttributionPopulated: true,
	}
	gotReplaceResult, err := store.ReplaceTaskPR(ctx, replaced, status)
	if err != nil {
		t.Fatalf("ReplaceTaskPR: %v", err)
	}
	assertOutcomeColumnsRoundTrip(t, "ReplaceTaskPR (return value)", gotReplaceResult, isDraft, changedFiles, mergedBy, closedBy)

	gotReplaced, err := store.GetTaskPRByRepoAndNumber(ctx, "task-roundtrip", "", 5001)
	if err != nil {
		t.Fatalf("GetTaskPRByRepoAndNumber after ReplaceTaskPR: %v", err)
	}
	assertOutcomeColumnsRoundTrip(t, "ReplaceTaskPR", gotReplaced, isDraft, changedFiles, mergedBy, closedBy)
}

func assertOutcomeColumnsRoundTrip(
	t *testing.T, writer string, got *TaskPR,
	wantIsDraft bool, wantChangedFiles int, wantMergedBy, wantClosedBy string,
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
}

// TestReplaceTaskPR_PreservesAutoMergeLatch covers AC-43: a non-populating
// observation must not clobber any of the five outcome columns just because
// ReplaceTaskPR's DELETE+INSERT upsert touches the row. Seeding and
// asserting all five, not just the latch, matters here: the spec's
// verification-surfaces section calls out that a latch-only implementation
// can pass a latch-only test while silently zeroing is_draft, changed_files,
// merged_by_login and closed_by_login.
func TestReplaceTaskPR_PreservesAutoMergeLatch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.db.Exec(`INSERT INTO tasks (id, workspace_id) VALUES ('task-latch-replace', 'ws-latch')`); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	latchedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	seedIsDraft := true
	seedChangedFiles := 9
	seedMergedBy := "carlosflorencio"
	seedClosedBy := "nova28"
	seed := &TaskPR{
		WorkspaceID:         "ws-latch",
		TaskID:              "task-latch-replace",
		Owner:               "kdlbs",
		Repo:                "kandev",
		PRNumber:            6001,
		PRURL:               "https://github.com/kdlbs/kandev/pull/6001",
		PRTitle:             "latch seed",
		State:               "open",
		CreatedAt:           time.Now().UTC(),
		IsDraft:             &seedIsDraft,
		ChangedFiles:        &seedChangedFiles,
		MergedByLogin:       &seedMergedBy,
		ClosedByLogin:       &seedClosedBy,
		AutoMergeObservedAt: &latchedAt,
	}
	if err := store.CreateTaskPR(ctx, seed); err != nil {
		t.Fatalf("seed CreateTaskPR: %v", err)
	}

	replace := &TaskPR{
		WorkspaceID: "ws-latch",
		TaskID:      "task-latch-replace",
		Owner:       "kdlbs",
		Repo:        "kandev",
		PRNumber:    6001,
		PRURL:       "https://github.com/kdlbs/kandev/pull/6001",
		PRTitle:     "latch seed (replaced)",
		State:       "open",
		CreatedAt:   time.Now().UTC(),
	}
	replaced, err := store.ReplaceTaskPR(ctx, replace, &PRStatus{})
	if err != nil {
		t.Fatalf("ReplaceTaskPR: %v", err)
	}
	assertOutcomeColumnsRoundTrip(t, "ReplaceTaskPR return value (preserved)",
		replaced, seedIsDraft, seedChangedFiles, seedMergedBy, seedClosedBy)
	if !replaced.AutoMergeObservedAt.Equal(latchedAt) {
		t.Fatalf("ReplaceTaskPR return value: AutoMergeObservedAt = %v, want preserved %v", replaced.AutoMergeObservedAt, latchedAt)
	}

	got, err := store.GetTaskPRByRepoAndNumber(ctx, "task-latch-replace", "", 6001)
	if err != nil {
		t.Fatalf("GetTaskPRByRepoAndNumber: %v", err)
	}
	assertOutcomeColumnsRoundTrip(t, "ReplaceTaskPR stored row (preserved)",
		got, seedIsDraft, seedChangedFiles, seedMergedBy, seedClosedBy)
	if got == nil || got.AutoMergeObservedAt == nil || !got.AutoMergeObservedAt.Equal(latchedAt) {
		t.Fatalf("stored row: AutoMergeObservedAt = %v, want preserved %v", got, latchedAt)
	}
}

// TestRestoreTaskPR_PreservesAutoMergeLatch mirrors
// TestReplaceTaskPR_PreservesAutoMergeLatch for the relink-after-detach path
// (AC-43): RestoreTaskPR must carry the deleted row's outcome columns
// forward — all five, not just auto_merge_observed_at — when the relink's
// observation doesn't populate outcome fields.
func TestRestoreTaskPR_PreservesAutoMergeLatch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.db.Exec(`INSERT INTO tasks (id, workspace_id) VALUES ('task-latch-restore', 'ws-latch')`); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	latchedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	seedIsDraft := true
	seedChangedFiles := 3
	seedMergedBy := "carlosflorencio"
	seedClosedBy := "nova28"
	seed := &TaskPR{
		WorkspaceID:         "ws-latch",
		TaskID:              "task-latch-restore",
		Owner:               "kdlbs",
		Repo:                "kandev",
		PRNumber:            6002,
		PRURL:               "https://github.com/kdlbs/kandev/pull/6002",
		PRTitle:             "latch seed",
		State:               "open",
		CreatedAt:           time.Now().UTC(),
		IsDraft:             &seedIsDraft,
		ChangedFiles:        &seedChangedFiles,
		MergedByLogin:       &seedMergedBy,
		ClosedByLogin:       &seedClosedBy,
		AutoMergeObservedAt: &latchedAt,
	}
	if err := store.CreateTaskPR(ctx, seed); err != nil {
		t.Fatalf("seed CreateTaskPR: %v", err)
	}

	pr := &PR{
		Number:    6002,
		RepoOwner: "kdlbs",
		RepoName:  "kandev",
		HTMLURL:   "https://github.com/kdlbs/kandev/pull/6002",
		Title:     "latch seed (restored)",
		State:     "open",
	}
	restored, err := store.RestoreTaskPR(ctx, "task-latch-restore", "", &PRStatus{PR: pr})
	if err != nil {
		t.Fatalf("RestoreTaskPR: %v", err)
	}
	if restored == nil {
		t.Fatalf("RestoreTaskPR: row not found")
	}
	assertOutcomeColumnsRoundTrip(t, "RestoreTaskPR (preserved)",
		restored, seedIsDraft, seedChangedFiles, seedMergedBy, seedClosedBy)
	if restored.AutoMergeObservedAt == nil || !restored.AutoMergeObservedAt.Equal(latchedAt) {
		t.Fatalf("RestoreTaskPR: AutoMergeObservedAt = %v, want preserved %v", restored, latchedAt)
	}
}

// TestReplaceTaskPR_ConcurrentWritersBothLand is the AC-43a regression:
// unlike TestUpdateTaskPR_AutoMergeLatchIsAtomicUnderConcurrentStaleWrites
// (which must manufacture a stale-read scenario because UpdateTaskPR has no
// injectable pause point), ReplaceTaskPR's read-resolve-write lives entirely
// inside one BeginTxx/Commit and the store's writer connection pool is
// SetMaxOpenConns(1) (see newTestStore), so two real concurrent goroutines
// calling ReplaceTaskPR are genuinely, deterministically serialized by the
// pool rather than interleaving — which is exactly the guarantee AC-43a
// requires. This launches two goroutines that each populate a DIFFERENT
// outcome field (one resolves merged_by_login, the other resolves
// closed_by_login, each preserving the field it doesn't touch) and asserts
// the final row carries BOTH contributions. If the outgoing read were taken
// outside the transaction (or against a stale snapshot) — the defect class
// the spec calls "invisible in a serial test" — whichever goroutine
// committed last would silently revert the other's write, and this would be
// flaky/order-dependent instead of reliably showing both fields set.
func TestReplaceTaskPR_ConcurrentWritersBothLand(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.db.Exec(`INSERT INTO tasks (id, workspace_id) VALUES ('task-race-replace', 'ws-race')`); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	seed := &TaskPR{
		WorkspaceID: "ws-race", TaskID: "task-race-replace", Owner: "kdlbs", Repo: "kandev",
		PRNumber: 6003, PRURL: "https://github.com/kdlbs/kandev/pull/6003", PRTitle: "race seed",
		State: "open", CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateTaskPR(ctx, seed); err != nil {
		t.Fatalf("seed CreateTaskPR: %v", err)
	}

	mergerStatus := &PRStatus{
		PR:                     &PR{MergedByLogin: "carlosflorencio"},
		OutcomeFieldsPopulated: true,
	}
	closerStatus := &PRStatus{
		ClosedByLogin:               "nova28",
		ClosureAttributionPopulated: true,
	}
	replaceWith := func(title string) *TaskPR {
		return &TaskPR{
			WorkspaceID: "ws-race", TaskID: "task-race-replace", Owner: "kdlbs", Repo: "kandev",
			PRNumber: 6003, PRURL: "https://github.com/kdlbs/kandev/pull/6003", PRTitle: title,
			State: "closed", CreatedAt: time.Now().UTC(),
		}
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := store.ReplaceTaskPR(ctx, replaceWith("replaced by merger"), mergerStatus); err != nil {
			errs <- err
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := store.ReplaceTaskPR(ctx, replaceWith("replaced by closer"), closerStatus); err != nil {
			errs <- err
		}
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("ReplaceTaskPR: %v", err)
	}

	got, err := store.GetTaskPRByRepoAndNumber(ctx, "task-race-replace", "", 6003)
	if err != nil || got == nil {
		t.Fatalf("GetTaskPRByRepoAndNumber: err=%v row=%v", err, got)
	}
	if got.MergedByLogin == nil || *got.MergedByLogin != "carlosflorencio" {
		t.Errorf("MergedByLogin = %v, want %q — the merger goroutine's write was lost to a non-serialized read",
			got.MergedByLogin, "carlosflorencio")
	}
	if got.ClosedByLogin == nil || *got.ClosedByLogin != "nova28" {
		t.Errorf("ClosedByLogin = %v, want %q — the closer goroutine's write was lost to a non-serialized read",
			got.ClosedByLogin, "nova28")
	}
}

// TestRestoreTaskPR_ConcurrentWritersBothLand mirrors
// TestReplaceTaskPR_ConcurrentWritersBothLand for RestoreTaskPR's own
// in-transaction read-resolve-write (AC-43a).
func TestRestoreTaskPR_ConcurrentWritersBothLand(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.db.Exec(`INSERT INTO tasks (id, workspace_id) VALUES ('task-race-restore', 'ws-race')`); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	seed := &TaskPR{
		WorkspaceID: "ws-race", TaskID: "task-race-restore", Owner: "kdlbs", Repo: "kandev",
		PRNumber: 6004, PRURL: "https://github.com/kdlbs/kandev/pull/6004", PRTitle: "race seed",
		State: "open", CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateTaskPR(ctx, seed); err != nil {
		t.Fatalf("seed CreateTaskPR: %v", err)
	}

	basePR := PR{
		Number: 6004, RepoOwner: "kdlbs", RepoName: "kandev",
		HTMLURL: "https://github.com/kdlbs/kandev/pull/6004", Title: "restored", State: "open",
	}
	mergerPR := basePR
	mergerPR.MergedByLogin = "carlosflorencio"
	mergerStatus := &PRStatus{PR: &mergerPR, OutcomeFieldsPopulated: true}
	closerPR := basePR
	closerStatus := &PRStatus{PR: &closerPR, ClosedByLogin: "nova28", ClosureAttributionPopulated: true}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := store.RestoreTaskPR(ctx, "task-race-restore", "", mergerStatus); err != nil {
			errs <- err
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := store.RestoreTaskPR(ctx, "task-race-restore", "", closerStatus); err != nil {
			errs <- err
		}
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("RestoreTaskPR: %v", err)
	}

	got, err := store.GetTaskPRByRepoAndNumber(ctx, "task-race-restore", "", 6004)
	if err != nil || got == nil {
		t.Fatalf("GetTaskPRByRepoAndNumber: err=%v row=%v", err, got)
	}
	if got.MergedByLogin == nil || *got.MergedByLogin != "carlosflorencio" {
		t.Errorf("MergedByLogin = %v, want %q — the merger goroutine's write was lost to a non-serialized read",
			got.MergedByLogin, "carlosflorencio")
	}
	if got.ClosedByLogin == nil || *got.ClosedByLogin != "nova28" {
		t.Errorf("ClosedByLogin = %v, want %q — the closer goroutine's write was lost to a non-serialized read",
			got.ClosedByLogin, "nova28")
	}
}
