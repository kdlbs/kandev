package github

import (
	"context"
	"testing"
	"time"
)

// TestCreatePRWatch_AllowsMultipleBranchesPerRepo locks the multi-branch
// invariant: the same (session, repository) pair can hold N watches, one
// per branch. Previously the unique constraint was UNIQUE(session_id,
// repository_id) and the dedup in CreatePRWatch returned the existing row
// — secondary branches' pushes silently re-attached to the primary's watch
// and the secondary PR never landed in github_task_prs.
func TestCreatePRWatch_AllowsMultipleBranchesPerRepo(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	seedTask(t, store, "task-1", false)

	w1, err := svc.CreatePRWatchForWorkspace(ctx, testWorkspaceID, "session-1", "task-1", "repo-1", "owner", "repo", 0, "feature/primary")
	if err != nil {
		t.Fatalf("CreatePRWatch primary: %v", err)
	}
	if w1 == nil {
		t.Fatal("primary watch must not be nil")
	}

	w2, err := svc.CreatePRWatchForWorkspace(ctx, testWorkspaceID, "session-1", "task-1", "repo-1", "owner", "repo", 0, "feature/branch-2")
	if err != nil {
		t.Fatalf("CreatePRWatch branch-2: %v", err)
	}
	if w2 == nil {
		t.Fatal("branch-2 watch must not be nil")
	}
	if w1.ID == w2.ID {
		t.Errorf("expected distinct watches for two branches, got same id %q", w1.ID)
	}

	all, err := store.ListPRWatchesBySession(ctx, "session-1")
	if err != nil {
		t.Fatalf("list watches: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 watches for (session, repo) across branches, got %d", len(all))
	}
}

// TestCreatePRWatch_IdempotentPerBranch verifies that re-creating a watch
// for the same (session, repo, branch) triple is a no-op returning the
// existing row — push detection retries that race the original create
// must not duplicate watches.
func TestCreatePRWatch_IdempotentPerBranch(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	seedTask(t, store, "task-1", false)

	first, err := svc.CreatePRWatchForWorkspace(ctx, testWorkspaceID, "session-1", "task-1", "repo-1", "owner", "repo", 0, "feature/x")
	if err != nil {
		t.Fatalf("first CreatePRWatch: %v", err)
	}
	second, err := svc.CreatePRWatchForWorkspace(ctx, testWorkspaceID, "session-1", "task-1", "repo-1", "owner", "repo", 0, "feature/x")
	if err != nil {
		t.Fatalf("second CreatePRWatch: %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("expected idempotent return of same watch id, got %q vs %q", first.ID, second.ID)
	}

	all, err := store.ListPRWatchesBySession(ctx, "session-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("expected 1 watch after idempotent retry, got %d", len(all))
	}
}

// TestGetPRWatchBySessionRepoAndBranch_FindsRightRow verifies the
// branch-aware lookup used by push detection. Two watches on the same
// repo, different branches: the lookup must return the matching branch's
// watch and not the other.
func TestGetPRWatchBySessionRepoAndBranch_FindsRightRow(t *testing.T) {
	_, svc, _, _ := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, svc.store, "task-1", false)

	primary, err := svc.CreatePRWatchForWorkspace(ctx, testWorkspaceID, "s1", "task-1", "repo-1", "owner", "repo", 1217, "feature/primary")
	if err != nil {
		t.Fatalf("primary: %v", err)
	}
	secondary, err := svc.CreatePRWatchForWorkspace(ctx, testWorkspaceID, "s1", "task-1", "repo-1", "owner", "repo", 0, "feature/secondary")
	if err != nil {
		t.Fatalf("secondary: %v", err)
	}

	got, err := svc.GetPRWatchBySessionRepoAndBranch(ctx, "s1", "repo-1", "feature/secondary")
	if err != nil {
		t.Fatalf("GetPRWatchBySessionRepoAndBranch: %v", err)
	}
	if got == nil || got.ID != secondary.ID {
		t.Fatalf("expected secondary watch id=%q, got %+v", secondary.ID, got)
	}

	gotPrimary, err := svc.GetPRWatchBySessionRepoAndBranch(ctx, "s1", "repo-1", "feature/primary")
	if err != nil {
		t.Fatalf("GetPRWatchBySessionRepoAndBranch primary: %v", err)
	}
	if gotPrimary == nil || gotPrimary.ID != primary.ID {
		t.Fatalf("expected primary watch id=%q, got %+v", primary.ID, gotPrimary)
	}
}

// TestUpdatePRWatchBranchIfSearching_NoCollision_UpdatesBranch covers the
// happy path: a single still-searching watch (pr_number=0) whose live
// branch changed gets its branch column rewritten.
func TestUpdatePRWatchBranchIfSearching_NoCollision_UpdatesBranch(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", false)

	w, err := svc.CreatePRWatchForWorkspace(ctx, testWorkspaceID, "session-1", "task-1", "repo-1", "owner", "repo", 0, "feature/old")
	if err != nil {
		t.Fatalf("create watch: %v", err)
	}

	if err := svc.UpdatePRWatchBranchIfSearching(ctx, w.ID, "feature/new"); err != nil {
		t.Fatalf("UpdatePRWatchBranchIfSearching: %v", err)
	}

	got, err := svc.GetPRWatchBySessionRepoAndBranch(ctx, "session-1", "repo-1", "feature/new")
	if err != nil {
		t.Fatalf("lookup updated branch: %v", err)
	}
	if got == nil || got.ID != w.ID {
		t.Fatalf("expected watch %q on new branch, got %+v", w.ID, got)
	}
}

// TestUpdatePRWatchBranchIfSearching_CollidesWithSibling_DropsSource locks
// the fix for the UNIQUE-constraint collision: when a sibling watch already
// owns the destination (session, repo, branch) triple, the source row is
// deleted instead of triggering a UNIQUE constraint error.
func TestUpdatePRWatchBranchIfSearching_CollidesWithSibling_DropsSource(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", false)

	source, err := svc.CreatePRWatchForWorkspace(ctx, testWorkspaceID, "session-1", "task-1", "repo-1", "owner", "repo", 0, "feature/A")
	if err != nil {
		t.Fatalf("create source watch: %v", err)
	}
	sibling, err := svc.CreatePRWatchForWorkspace(ctx, testWorkspaceID, "session-1", "task-1", "repo-1", "owner", "repo", 0, "feature/B")
	if err != nil {
		t.Fatalf("create sibling watch: %v", err)
	}

	if err := svc.UpdatePRWatchBranchIfSearching(ctx, source.ID, "feature/B"); err != nil {
		t.Fatalf("UpdatePRWatchBranchIfSearching must not error on sibling collision: %v", err)
	}

	all, err := store.ListPRWatchesBySession(ctx, "session-1")
	if err != nil {
		t.Fatalf("list watches: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected source watch dropped, got %d remaining", len(all))
	}
	if all[0].ID != sibling.ID {
		t.Fatalf("expected sibling watch %q to survive, got %q", sibling.ID, all[0].ID)
	}
	if all[0].Branch != "feature/B" {
		t.Errorf("sibling branch must be untouched, got %q", all[0].Branch)
	}
}

// TestUpdatePRWatchBranchIfSearching_CollidesWithSiblingHasPR_DropsSource
// exercises the realistic race: the sibling has already discovered its PR
// (pr_number != 0) when the still-searching source tries to migrate onto
// the same branch. Source must be deleted, sibling row (including its PR
// number) must survive untouched.
func TestUpdatePRWatchBranchIfSearching_CollidesWithSiblingHasPR_DropsSource(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", false)

	source, err := svc.CreatePRWatchForWorkspace(ctx, testWorkspaceID, "session-1", "task-1", "repo-1", "owner", "repo", 0, "feature/A")
	if err != nil {
		t.Fatalf("create source watch: %v", err)
	}
	sibling, err := svc.CreatePRWatchForWorkspace(ctx, testWorkspaceID, "session-1", "task-1", "repo-1", "owner", "repo", 0, "feature/B")
	if err != nil {
		t.Fatalf("create sibling watch: %v", err)
	}
	if err := store.UpdatePRWatchPRNumber(ctx, sibling.ID, 99); err != nil {
		t.Fatalf("mark sibling as found: %v", err)
	}

	if err := svc.UpdatePRWatchBranchIfSearching(ctx, source.ID, "feature/B"); err != nil {
		t.Fatalf("UpdatePRWatchBranchIfSearching must not error when sibling owns branch with active PR: %v", err)
	}

	all, err := store.ListPRWatchesBySession(ctx, "session-1")
	if err != nil {
		t.Fatalf("list watches: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected source watch dropped, sibling preserved; got %d remaining", len(all))
	}
	if all[0].ID != sibling.ID {
		t.Fatalf("expected sibling %q to survive, got %q", sibling.ID, all[0].ID)
	}
	if all[0].Branch != "feature/B" {
		t.Errorf("sibling branch must be untouched, got %q", all[0].Branch)
	}
	if all[0].PRNumber != 99 {
		t.Errorf("sibling PR number must be preserved (99), got %d", all[0].PRNumber)
	}
}

// TestUpdatePRWatchBranchIfSearching_PRAlreadyFound_NoOp preserves the
// existing "searching" guard: a watch that already found its PR
// (pr_number != 0) must not have its branch overwritten.
func TestUpdatePRWatchBranchIfSearching_PRAlreadyFound_NoOp(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", false)

	w, err := svc.CreatePRWatchForWorkspace(ctx, testWorkspaceID, "session-1", "task-1", "repo-1", "owner", "repo", 42, "feature/found")
	if err != nil {
		t.Fatalf("create watch: %v", err)
	}

	if err := svc.UpdatePRWatchBranchIfSearching(ctx, w.ID, "feature/other"); err != nil {
		t.Fatalf("UpdatePRWatchBranchIfSearching: %v", err)
	}

	got, err := svc.GetPRWatchBySessionRepoAndBranch(ctx, "session-1", "repo-1", "feature/found")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got == nil || got.ID != w.ID {
		t.Fatalf("expected watch branch unchanged, got %+v", got)
	}
}

// TestUpdatePRWatchBranchIfSearching_MissingRow_NoOp locks the
// idempotency contract: calling with an ID that doesn't exist must be
// a silent no-op (matches the prior UPDATE-by-id semantics that
// affected 0 rows without error).
func TestUpdatePRWatchBranchIfSearching_MissingRow_NoOp(t *testing.T) {
	_, svc, _, _ := setupPollerTest(t)
	ctx := context.Background()

	if err := svc.UpdatePRWatchBranchIfSearching(ctx, "nonexistent-id", "feature/any"); err != nil {
		t.Fatalf("must be a no-op for missing row, got: %v", err)
	}
}

func TestTriggerPRStatusSync_AssociatesExactPRWhenSiblingIsFresh(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", false)

	now := time.Now().UTC()
	mergedAt := now.Add(-30 * time.Minute)
	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		Owner:        "owner",
		Repo:         "repo",
		PRNumber:     1293,
		PRURL:        "https://github.com/owner/repo/pull/1293",
		PRTitle:      "First",
		HeadBranch:   "feature/first",
		BaseBranch:   "main",
		State:        "merged",
		CreatedAt:    now.Add(-2 * time.Hour),
		MergedAt:     &mergedAt,
		ClosedAt:     &mergedAt,
		LastSyncedAt: &now,
	}); err != nil {
		t.Fatalf("seed merged sibling: %v", err)
	}

	watch, err := svc.CreatePRWatchForWorkspace(ctx, testWorkspaceID, "session-1", "task-1", "repo-1", "owner", "repo", 1299, "feature/second")
	if err != nil {
		t.Fatalf("CreatePRWatch: %v", err)
	}
	mockClient.AddPR(&PR{
		Number:     1299,
		Title:      "Second",
		HTMLURL:    "https://github.com/owner/repo/pull/1299",
		State:      "open",
		HeadBranch: "feature/second",
		BaseBranch: "main",
		RepoOwner:  "owner",
		RepoName:   "repo",
		CreatedAt:  now.Add(-1 * time.Hour),
		UpdatedAt:  now,
	})

	got, err := svc.triggerPRStatusSync(ctx, watch, "task-1")
	if err != nil {
		t.Fatalf("triggerPRStatusSync: %v", err)
	}
	if got == nil {
		t.Fatal("expected TaskPR result")
	}
	if got.PRNumber != 1299 || got.State != "open" {
		t.Fatalf("sync returned PR #%d state=%q, want PR #1299 open", got.PRNumber, got.State)
	}

	all, err := store.ListTaskPRsByTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("ListTaskPRsByTask: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected sync to create exact sibling row, got %d rows", len(all))
	}
}

func TestApplyBatchedNumberedWatch_AssociatesExactPRWhenSiblingExists(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", false)

	now := time.Now().UTC()
	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		Owner:        "owner",
		Repo:         "repo",
		PRNumber:     1293,
		PRURL:        "https://github.com/owner/repo/pull/1293",
		PRTitle:      "First",
		HeadBranch:   "feature/first",
		BaseBranch:   "main",
		State:        "merged",
		CreatedAt:    now.Add(-2 * time.Hour),
		LastSyncedAt: &now,
	}); err != nil {
		t.Fatalf("seed merged sibling: %v", err)
	}

	watch, err := svc.CreatePRWatchForWorkspace(ctx, testWorkspaceID, "session-1", "task-1", "repo-1", "owner", "repo", 1299, "feature/second")
	if err != nil {
		t.Fatalf("CreatePRWatch: %v", err)
	}
	status := &PRStatus{
		PR: &PR{
			Number:     1299,
			Title:      "Second",
			HTMLURL:    "https://github.com/owner/repo/pull/1299",
			State:      "open",
			HeadBranch: "feature/second",
			BaseBranch: "main",
			RepoOwner:  "owner",
			RepoName:   "repo",
			CreatedAt:  now.Add(-1 * time.Hour),
			UpdatedAt:  now,
		},
		ChecksState: "pending",
	}

	result := svc.applyBatchedNumberedWatch(ctx, "legacy", watch, map[string]*PRStatus{
		prStatusCacheKey("owner", "repo", 1299): status,
	}, now)
	if result.SyncFailed {
		t.Fatal("batched sync should not fail")
	}

	got, err := store.GetTaskPRByRepoAndNumber(ctx, "task-1", "repo-1", 1299)
	if err != nil {
		t.Fatalf("GetTaskPRByRepoAndNumber: %v", err)
	}
	if got == nil || got.State != "open" {
		t.Fatalf("expected exact PR #1299 open row, got %+v", got)
	}
}

func TestApplyBatchedNumberedWatch_MissingPRDataDoesNotPanic(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", false)

	now := time.Now().UTC()
	watch, err := svc.CreatePRWatchForWorkspace(ctx, testWorkspaceID, "session-1", "task-1", "repo-1", "owner", "repo", 1299, "feature/second")
	if err != nil {
		t.Fatalf("CreatePRWatch: %v", err)
	}

	result := svc.applyBatchedNumberedWatch(ctx, "legacy", watch, map[string]*PRStatus{
		prStatusCacheKey("owner", "repo", 1299): &PRStatus{ChecksState: "pending"},
	}, now)
	if !result.SyncFailed {
		t.Fatal("batched sync should fail without PR data")
	}

	got, err := store.GetTaskPRByRepoAndNumber(ctx, "task-1", "repo-1", 1299)
	if err != nil {
		t.Fatalf("GetTaskPRByRepoAndNumber: %v", err)
	}
	if got != nil {
		t.Fatalf("expected no task PR row without PR data, got %+v", got)
	}
}
