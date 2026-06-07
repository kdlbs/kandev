package github

import (
	"context"
	"testing"
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

	w1, err := svc.CreatePRWatch(ctx, "session-1", "task-1", "repo-1", "owner", "repo", 0, "feature/primary")
	if err != nil {
		t.Fatalf("CreatePRWatch primary: %v", err)
	}
	if w1 == nil {
		t.Fatal("primary watch must not be nil")
	}

	w2, err := svc.CreatePRWatch(ctx, "session-1", "task-1", "repo-1", "owner", "repo", 0, "feature/branch-2")
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

	first, err := svc.CreatePRWatch(ctx, "session-1", "task-1", "repo-1", "owner", "repo", 0, "feature/x")
	if err != nil {
		t.Fatalf("first CreatePRWatch: %v", err)
	}
	second, err := svc.CreatePRWatch(ctx, "session-1", "task-1", "repo-1", "owner", "repo", 0, "feature/x")
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

	primary, err := svc.CreatePRWatch(ctx, "s1", "task-1", "repo-1", "owner", "repo", 1217, "feature/primary")
	if err != nil {
		t.Fatalf("primary: %v", err)
	}
	secondary, err := svc.CreatePRWatch(ctx, "s1", "task-1", "repo-1", "owner", "repo", 0, "feature/secondary")
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

	w, err := svc.CreatePRWatch(ctx, "session-1", "task-1", "repo-1", "owner", "repo", 0, "feature/old")
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

	source, err := svc.CreatePRWatch(ctx, "session-1", "task-1", "repo-1", "owner", "repo", 0, "feature/A")
	if err != nil {
		t.Fatalf("create source watch: %v", err)
	}
	sibling, err := svc.CreatePRWatch(ctx, "session-1", "task-1", "repo-1", "owner", "repo", 0, "feature/B")
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

// TestUpdatePRWatchBranchIfSearching_PRAlreadyFound_NoOp preserves the
// existing "searching" guard: a watch that already found its PR
// (pr_number != 0) must not have its branch overwritten.
func TestUpdatePRWatchBranchIfSearching_PRAlreadyFound_NoOp(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", false)

	w, err := svc.CreatePRWatch(ctx, "session-1", "task-1", "repo-1", "owner", "repo", 42, "feature/found")
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
