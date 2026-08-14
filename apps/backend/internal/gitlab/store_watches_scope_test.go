package gitlab

import (
	"context"
	"errors"
	"testing"
)

// newMRWatchScopeStore seeds two workspaces, one task in each, and one MR
// watch per task, returning the store plus the two watch IDs. The
// *ForWorkspace lookups reach the watch through its owning task, so this is
// the minimum fixture that can tell a scoped lookup from an unscoped one.
func newMRWatchScopeStore(t *testing.T) (*Store, *MRWatch, *MRWatch) {
	t.Helper()
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	seedWorkspace(t, store, "ws-2")
	seedTask(t, store, "task-1", "ws-1")
	seedTask(t, store, "task-2", "ws-2")
	mine := &MRWatch{SessionID: "sess-1", TaskID: "task-1", RepositoryID: "repo-1", ProjectPath: "group/a", MRIID: 1}
	foreign := &MRWatch{SessionID: "sess-2", TaskID: "task-2", RepositoryID: "repo-2", ProjectPath: "group/b", MRIID: 2}
	for _, watch := range []*MRWatch{mine, foreign} {
		if err := store.CreateMRWatch(ctx, watch); err != nil {
			t.Fatalf("seed MR watch: %v", err)
		}
	}
	return store, mine, foreign
}

func TestStoreListMRWatchesBySessionForWorkspaceExcludesForeignSessions(t *testing.T) {
	store, mine, foreign := newMRWatchScopeStore(t)
	ctx := context.Background()

	own, err := store.ListMRWatchesBySessionForWorkspace(ctx, "ws-1", mine.SessionID)
	if err != nil {
		t.Fatalf("list own session watches: %v", err)
	}
	if len(own) != 1 || own[0].ID != mine.ID {
		t.Fatalf("watches = %+v, want only the ws-1 watch", own)
	}

	// The same session id queried from the other workspace must return
	// nothing: the scope comes from the owning task's workspace, not from
	// the caller-supplied session id.
	crossed, err := store.ListMRWatchesBySessionForWorkspace(ctx, "ws-2", mine.SessionID)
	if err != nil {
		t.Fatalf("list cross-workspace session watches: %v", err)
	}
	if len(crossed) != 0 {
		t.Errorf("cross-workspace lookup returned %+v, want nothing", crossed)
	}

	foreignScoped, err := store.ListMRWatchesBySessionForWorkspace(ctx, "ws-2", foreign.SessionID)
	if err != nil {
		t.Fatalf("list ws-2 session watches: %v", err)
	}
	if len(foreignScoped) != 1 || foreignScoped[0].ID != foreign.ID {
		t.Errorf("ws-2 watches = %+v, want its own watch", foreignScoped)
	}
}

func TestStoreListMRWatchesByTaskForWorkspaceExcludesForeignTasks(t *testing.T) {
	store, mine, _ := newMRWatchScopeStore(t)
	ctx := context.Background()

	own, err := store.ListMRWatchesByTaskForWorkspace(ctx, "ws-1", "task-1")
	if err != nil {
		t.Fatalf("list own task watches: %v", err)
	}
	if len(own) != 1 || own[0].ID != mine.ID {
		t.Fatalf("watches = %+v, want only the ws-1 watch", own)
	}

	crossed, err := store.ListMRWatchesByTaskForWorkspace(ctx, "ws-1", "task-2")
	if err != nil {
		t.Fatalf("list cross-workspace task watches: %v", err)
	}
	if len(crossed) != 0 {
		t.Errorf("cross-workspace lookup returned %+v, want nothing", crossed)
	}
}

// TestStoreListMRWatchesForWorkspaceHidesOrphanedRows pins the documented
// behavior: a watch whose task row is gone has no resolvable workspace, so the
// workspace-facing lookups must not surface it even though the unscoped
// listing still does.
func TestStoreListMRWatchesForWorkspaceHidesOrphanedRows(t *testing.T) {
	store, mine, _ := newMRWatchScopeStore(t)
	ctx := context.Background()
	if _, err := store.db.Exec(`DELETE FROM tasks WHERE id = ?`, "task-1"); err != nil {
		t.Fatalf("delete owning task: %v", err)
	}

	scoped, err := store.ListMRWatchesByTaskForWorkspace(ctx, "ws-1", "task-1")
	if err != nil {
		t.Fatalf("list orphaned task watches: %v", err)
	}
	if len(scoped) != 0 {
		t.Errorf("workspace lookup surfaced the orphaned watch %+v", scoped)
	}

	unscoped, err := store.ListMRWatchesByTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("list unscoped task watches: %v", err)
	}
	if len(unscoped) != 1 || unscoped[0].ID != mine.ID {
		t.Errorf("unscoped listing = %+v, want the orphaned row still present", unscoped)
	}
}

func TestStoreDeleteMRWatchesByTaskIDRemovesOnlyThatTasksWatches(t *testing.T) {
	store, _, foreign := newMRWatchScopeStore(t)
	ctx := context.Background()

	deleted, err := store.DeleteMRWatchesByTaskID(ctx, "task-1")
	if err != nil {
		t.Fatalf("delete MR watches by task: %v", err)
	}
	if deleted != 1 {
		t.Errorf("rows affected = %d, want 1", deleted)
	}
	remaining, err := store.ListActiveMRWatches(ctx)
	if err != nil {
		t.Fatalf("list active watches: %v", err)
	}
	if len(remaining) != 1 || remaining[0].ID != foreign.ID {
		t.Fatalf("remaining watches = %+v, want only task-2's watch", remaining)
	}
}

func TestStoreDeleteMRWatchesByTaskIDIsZeroForUnknownTask(t *testing.T) {
	store, _, _ := newMRWatchScopeStore(t)

	deleted, err := store.DeleteMRWatchesByTaskID(context.Background(), "no-such-task")
	if err != nil {
		t.Fatalf("delete MR watches by task: %v", err)
	}
	if deleted != 0 {
		t.Errorf("rows affected = %d, want 0", deleted)
	}
}

// --- Service-level session lookups ---

func TestServiceGetMRWatchBySessionAndRepoDiscriminatesRepositories(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")
	svc := NewService(DefaultHost, NewMockClient(DefaultHost), AuthMethodPAT, nil, newTestLogger(t))
	svc.SetStore(store)
	for _, repo := range []struct {
		id      string
		project string
		iid     int
	}{{"repo-a", "group/a", 1}, {"repo-b", "group/b", 2}} {
		if _, err := svc.CreateMRWatch(ctx, "sess-1", "task-1", repo.id, repo.project, repo.iid, "feat/x"); err != nil {
			t.Fatalf("seed watch for %s: %v", repo.id, err)
		}
	}

	watch, err := svc.GetMRWatchBySessionAndRepo(ctx, "sess-1", "repo-b")
	if err != nil {
		t.Fatalf("get watch by session and repo: %v", err)
	}
	if watch == nil || watch.ProjectPath != "group/b" {
		t.Fatalf("watch = %+v, want repo-b's group/b watch", watch)
	}

	missing, err := svc.GetMRWatchBySessionAndRepo(ctx, "sess-1", "repo-missing")
	if err != nil {
		t.Fatalf("get watch for unknown repo: %v", err)
	}
	if missing != nil {
		t.Errorf("watch = %+v, want nil for an unknown repository", missing)
	}
}

func TestServiceMRWatchLookupsRequireAStore(t *testing.T) {
	svc := NewService(DefaultHost, NewMockClient(DefaultHost), AuthMethodPAT, nil, newTestLogger(t))
	ctx := context.Background()

	if _, err := svc.GetMRWatchBySession(ctx, "sess-1"); !errors.Is(err, errStoreUnavailable) {
		t.Errorf("GetMRWatchBySession error = %v, want errStoreUnavailable", err)
	}
	if _, err := svc.GetMRWatchBySessionAndRepo(ctx, "sess-1", "repo-a"); !errors.Is(err, errStoreUnavailable) {
		t.Errorf("GetMRWatchBySessionAndRepo error = %v, want errStoreUnavailable", err)
	}
}

// TestServiceGetMRWatchBySessionOnlyMatchesTheLegacyRepositorylessRow pins the
// documented meaning of GetMRWatchBySession: it is the legacy single-repo
// accessor, keyed on an empty repository_id. A repo-keyed watch on the same
// session must not answer it, otherwise a multi-repo session would silently
// resolve to an arbitrary one of its repositories.
func TestServiceGetMRWatchBySessionOnlyMatchesTheLegacyRepositorylessRow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")
	svc := NewService(DefaultHost, NewMockClient(DefaultHost), AuthMethodPAT, nil, newTestLogger(t))
	svc.SetStore(store)
	if _, err := svc.CreateMRWatch(ctx, "sess-1", "task-1", "repo-a", "group/a", 1, "feat/x"); err != nil {
		t.Fatalf("seed repo-keyed watch: %v", err)
	}

	watch, err := svc.GetMRWatchBySession(ctx, "sess-1")
	if err != nil {
		t.Fatalf("get watch by session: %v", err)
	}
	if watch != nil {
		t.Fatalf("watch = %+v, want nil — a repo-keyed watch is not the legacy row", watch)
	}

	legacy, err := svc.CreateMRWatch(ctx, "sess-1", "task-1", "", "group/legacy", 2, "feat/y")
	if err != nil {
		t.Fatalf("seed legacy watch: %v", err)
	}
	watch, err = svc.GetMRWatchBySession(ctx, "sess-1")
	if err != nil {
		t.Fatalf("get watch by session: %v", err)
	}
	if watch == nil || watch.ID != legacy.ID {
		t.Fatalf("watch = %+v, want the legacy repository-less watch %s", watch, legacy.ID)
	}
}

// --- Reservation release ---

func TestStoreReleaseReviewMRTaskAllowsARetryOnTheSameMR(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	watch := validReviewWatchRow()
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("create review watch: %v", err)
	}

	reserved, err := store.ReserveReviewMRTask(ctx, watch.ID, "group/p", 1, "url")
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	if !reserved {
		t.Fatal("first reservation was refused")
	}
	again, err := store.ReserveReviewMRTask(ctx, watch.ID, "group/p", 1, "url")
	if err != nil {
		t.Fatalf("second reserve: %v", err)
	}
	if again {
		t.Fatal("the same MR was reserved twice without a release")
	}

	if err := store.ReleaseReviewMRTask(ctx, watch.ID, "group/p", 1); err != nil {
		t.Fatalf("release reservation: %v", err)
	}

	retried, err := store.ReserveReviewMRTask(ctx, watch.ID, "group/p", 1, "url")
	if err != nil {
		t.Fatalf("reserve after release: %v", err)
	}
	if !retried {
		t.Error("the MR could not be reserved again after its reservation was released")
	}
}

// TestStoreReleaseReviewMRTaskForGenerationIgnoresStaleGenerations guards the
// ownership check: a release carrying a generation the reservation no longer
// belongs to must leave the row alone, so a slow poller from before a watch
// reset cannot free a reservation the current generation owns.
func TestStoreReleaseReviewMRTaskForGenerationIgnoresStaleGenerations(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	watch := validReviewWatchRow()
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("create review watch: %v", err)
	}
	if _, err := store.ReserveReviewMRTask(ctx, watch.ID, "group/p", 1, "url"); err != nil {
		t.Fatalf("reserve: %v", err)
	}

	if err := store.ReleaseReviewMRTaskForGeneration(ctx, watch.ID, 999, "group/p", 1); err != nil {
		t.Fatalf("stale-generation release: %v", err)
	}
	stillHeld, err := store.ReserveReviewMRTask(ctx, watch.ID, "group/p", 1, "url")
	if err != nil {
		t.Fatalf("reserve after stale release: %v", err)
	}
	if stillHeld {
		t.Fatal("a stale-generation release freed the reservation")
	}

	generation, err := store.reviewWatchGeneration(ctx, watch.ID)
	if err != nil {
		t.Fatalf("load watch generation: %v", err)
	}
	if err := store.ReleaseReviewMRTaskForGeneration(ctx, watch.ID, generation, "group/p", 1); err != nil {
		t.Fatalf("current-generation release: %v", err)
	}
	freed, err := store.ReserveReviewMRTask(ctx, watch.ID, "group/p", 1, "url")
	if err != nil {
		t.Fatalf("reserve after current release: %v", err)
	}
	if !freed {
		t.Error("a current-generation release did not free the reservation")
	}
}

func TestStoreReleaseIssueWatchTaskAllowsARetryOnTheSameIssue(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	watch := validIssueWatchRow()
	if err := store.CreateIssueWatch(ctx, watch); err != nil {
		t.Fatalf("create issue watch: %v", err)
	}
	if _, err := store.ReserveIssueWatchTask(ctx, watch.ID, "group/p", 1, "url"); err != nil {
		t.Fatalf("reserve: %v", err)
	}

	if err := store.ReleaseIssueWatchTask(ctx, watch.ID, "group/p", 1); err != nil {
		t.Fatalf("release reservation: %v", err)
	}

	retried, err := store.ReserveIssueWatchTask(ctx, watch.ID, "group/p", 1, "url")
	if err != nil {
		t.Fatalf("reserve after release: %v", err)
	}
	if !retried {
		t.Error("the issue could not be reserved again after its reservation was released")
	}
}

func TestStoreReleaseIssueWatchTaskForGenerationIgnoresStaleGenerations(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	watch := validIssueWatchRow()
	if err := store.CreateIssueWatch(ctx, watch); err != nil {
		t.Fatalf("create issue watch: %v", err)
	}
	if _, err := store.ReserveIssueWatchTask(ctx, watch.ID, "group/p", 1, "url"); err != nil {
		t.Fatalf("reserve: %v", err)
	}

	if err := store.ReleaseIssueWatchTaskForGeneration(ctx, watch.ID, 999, "group/p", 1); err != nil {
		t.Fatalf("stale-generation release: %v", err)
	}
	stillHeld, err := store.ReserveIssueWatchTask(ctx, watch.ID, "group/p", 1, "url")
	if err != nil {
		t.Fatalf("reserve after stale release: %v", err)
	}
	if stillHeld {
		t.Fatal("a stale-generation release freed the reservation")
	}

	generation, err := store.issueWatchGeneration(ctx, watch.ID)
	if err != nil {
		t.Fatalf("load watch generation: %v", err)
	}
	if err := store.ReleaseIssueWatchTaskForGeneration(ctx, watch.ID, generation, "group/p", 1); err != nil {
		t.Fatalf("current-generation release: %v", err)
	}
	freed, err := store.ReserveIssueWatchTask(ctx, watch.ID, "group/p", 1, "url")
	if err != nil {
		t.Fatalf("reserve after current release: %v", err)
	}
	if !freed {
		t.Error("a current-generation release did not free the reservation")
	}
}

// TestStoreIssueWatchGenerationDefaultsForUnknownWatch covers the sql.ErrNoRows
// branch: a missing watch reports generation 1 rather than an error, so a
// release racing a watch deletion degrades instead of failing the poll.
func TestStoreIssueWatchGenerationDefaultsForUnknownWatch(t *testing.T) {
	store := newTestStore(t)

	generation, err := store.issueWatchGeneration(context.Background(), "no-such-watch")
	if err != nil {
		t.Fatalf("issue watch generation: %v", err)
	}
	if generation != 1 {
		t.Errorf("generation = %d, want 1 for an unknown watch", generation)
	}
}

// --- Repository provider identity ---

// TestValidateTaskMRRepositoryIdentityFailsClosedOnEmptyProviderHost encodes
// the invariant from apps/backend/CLAUDE.md: a provider-backed repository is
// keyed by workspace, provider, normalized provider_host origin, owner and
// name. A legacy row that carries a gitlab provider and a matching
// owner/name but no provider_host has unknown identity, so it must fail
// closed rather than being inferred from owner/name alone.
func TestValidateTaskMRRepositoryIdentityFailsClosedOnEmptyProviderHost(t *testing.T) {
	const host = "https://gitlab.self-managed.test"
	_, store, _ := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	// Provider and owner/name match the MR exactly; only the host is blank.
	if _, err := store.db.Exec(`UPDATE repositories
		SET provider = 'gitlab', provider_host = '', provider_owner = 'acme', provider_name = 'api'
		WHERE id = ?`, "repo-1"); err != nil {
		t.Fatalf("seed hostless repository identity: %v", err)
	}

	err := store.ValidateTaskMRRepositoryIdentity(
		context.Background(), "ws-1", "task-1", "repo-1", host, "acme/api")

	if !errors.Is(err, ErrTaskMRRepositoryMismatch) {
		t.Fatalf("error = %v, want ErrTaskMRRepositoryMismatch — an owner/name match "+
			"without a provider_host must not establish identity", err)
	}
}

// TestValidateTaskMRRepositoryIdentityRejectsSameProjectOnAnotherHost is the
// other half of the same invariant: identical owner/name on a different
// GitLab installation is a different repository.
func TestValidateTaskMRRepositoryIdentityRejectsSameProjectOnAnotherHost(t *testing.T) {
	const host = "https://gitlab.self-managed.test"
	_, store, _ := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", "https://gitlab.other.test", "acme/api")

	err := store.ValidateTaskMRRepositoryIdentity(
		context.Background(), "ws-1", "task-1", "repo-1", host, "acme/api")

	if !errors.Is(err, ErrTaskMRRepositoryMismatch) {
		t.Fatalf("error = %v, want ErrTaskMRRepositoryMismatch for the same project "+
			"on a different host", err)
	}
}

// TestValidateTaskMRRepositoryIdentityAcceptsMatchingHostAndProject is the
// positive control for the two rejections above: with the host present and
// matching, the same owner/name is accepted.
func TestValidateTaskMRRepositoryIdentityAcceptsMatchingHostAndProject(t *testing.T) {
	const host = "https://gitlab.self-managed.test"
	_, store, _ := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", host, "acme/api")

	if err := store.ValidateTaskMRRepositoryIdentity(
		context.Background(), "ws-1", "task-1", "repo-1", host, "acme/api"); err != nil {
		t.Fatalf("identity check rejected an exact host+project match: %v", err)
	}
}
