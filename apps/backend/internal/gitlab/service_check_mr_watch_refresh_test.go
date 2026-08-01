package gitlab

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestCheckMRWatch_RefreshesTaskMRAndPublishes(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", "https://gitlab.com", "group/proj")

	client := NewMockClient("https://gitlab.com")
	client.SeedMR("group/proj", &MR{
		IID: 5, State: mrStateOpen, HeadBranch: "feat/a", Title: "Feat A",
		WebURL: "https://gitlab.com/group/proj/-/merge_requests/5",
	})

	svc := NewService("https://gitlab.com", client, "none", nil, newTestLogger(t))
	svc.SetStore(store)
	log := newTestLogger(t)
	eventBus := bus.NewMemoryEventBus(log)
	svc.SetEventBus(eventBus)

	received := make(chan *bus.Event, 1)
	if _, err := eventBus.Subscribe(events.GitLabTaskMRUpdated, func(_ context.Context, evt *bus.Event) error {
		received <- evt
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	watch := &MRWatch{
		SessionID: "sess-1", TaskID: "task-1", RepositoryID: "repo-1",
		ProjectPath: "group/proj", MRIID: 5, Branch: "feat/a",
	}
	if err := store.CreateMRWatch(ctx, watch); err != nil {
		t.Fatalf("create watch: %v", err)
	}

	status, notable, err := svc.CheckMRWatch(ctx, watch)
	if err != nil {
		t.Fatalf("check watch: %v", err)
	}
	if status == nil || status.MR == nil || status.MR.IID != 5 {
		t.Fatalf("unexpected status: %+v", status)
	}
	_ = notable

	mrs, err := store.ListTaskMRsByTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("list task MRs: %v", err)
	}
	if len(mrs) != 1 {
		t.Fatalf("expected 1 task MR row after refresh, got %d", len(mrs))
	}
	if mrs[0].MRIID != 5 || mrs[0].RepositoryID != "repo-1" || mrs[0].ProjectPath != "group/proj" {
		t.Fatalf("unexpected task MR row: %+v", mrs[0])
	}

	select {
	case evt := <-received:
		if evt == nil {
			t.Fatal("expected non-nil published event")
		}
	default:
		t.Fatal("expected gitlab.task_mr.updated to be published")
	}
}

// TestCheckMRWatch_RejectsMismatchedIdentityOnRefresh guards the refresh path
// the same way AutoLinkMRForBranch and AssociateExistingMRByURL already are:
// a status response whose MR doesn't actually match the watch's (host,
// project, iid) — a client bug, or a race where the watch's mr_iid was
// updated between fetch and refresh — must not create or overwrite the
// task-MR association.
func TestCheckMRWatch_RejectsMismatchedIdentityOnRefresh(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", "https://gitlab.com", "group/proj")

	client := NewMockClient("https://gitlab.com")
	svc := NewService("https://gitlab.com", client, "none", nil, newTestLogger(t))
	svc.SetStore(store)

	watch := &MRWatch{
		SessionID: "sess-1", TaskID: "task-1", RepositoryID: "repo-1",
		ProjectPath: "group/proj", MRIID: 5, Branch: "feat/a",
	}
	if err := store.CreateMRWatch(ctx, watch); err != nil {
		t.Fatalf("create watch: %v", err)
	}

	// GetMRStatus is keyed by (projectPath, iid) — the watch's own
	// (group/proj, 5) — but the seeded MR's WebURL points at a different
	// project, simulating a misbehaving/mismatched client response.
	client.SeedMR("group/proj", &MR{
		IID: 5, State: mrStateOpen, HeadBranch: "feat/a", Title: "Wrong URL",
		WebURL: "https://gitlab.com/group/other/-/merge_requests/5",
	})

	if _, _, err := svc.CheckMRWatch(ctx, watch); err != nil {
		t.Fatalf("check watch: %v", err)
	}

	mrs, err := store.ListTaskMRsByTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("list task MRs: %v", err)
	}
	if len(mrs) != 0 {
		t.Fatalf("expected no task MR row for a mismatched identity response, got %+v", mrs)
	}
}

// TestCheckMRWatch_MismatchedIdentityDoesNotOverwriteExistingRow covers the
// other half of the same guarantee: a mismatched refresh must not just skip
// creating a row, it must leave an already-correct row untouched rather than
// clobbering it with the mismatched response's fields.
func TestCheckMRWatch_MismatchedIdentityDoesNotOverwriteExistingRow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", "https://gitlab.com", "group/proj")

	client := NewMockClient("https://gitlab.com")
	svc := NewService("https://gitlab.com", client, "none", nil, newTestLogger(t))
	svc.SetStore(store)

	watch := &MRWatch{
		SessionID: "sess-1", TaskID: "task-1", RepositoryID: "repo-1",
		ProjectPath: "group/proj", MRIID: 5, Branch: "feat/a",
	}
	if err := store.CreateMRWatch(ctx, watch); err != nil {
		t.Fatalf("create watch: %v", err)
	}

	existing := &TaskMR{
		TaskID: "task-1", RepositoryID: "repo-1", Host: "https://gitlab.com",
		ProjectPath: "group/proj", MRIID: 5, MRTitle: "Correct Title",
		HeadBranch: "feat/a", State: mrStateOpen,
	}
	if err := store.UpsertTaskMR(ctx, existing); err != nil {
		t.Fatalf("seed existing task MR: %v", err)
	}

	// Same mismatched-identity response as the sibling test above: seeded
	// MR's WebURL points at a different project than the watch's own.
	client.SeedMR("group/proj", &MR{
		IID: 5, State: mrStateOpen, HeadBranch: "feat/a", Title: "Wrong URL",
		WebURL: "https://gitlab.com/group/other/-/merge_requests/5",
	})

	if _, _, err := svc.CheckMRWatch(ctx, watch); err != nil {
		t.Fatalf("check watch: %v", err)
	}

	mrs, err := store.ListTaskMRsByTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("list task MRs: %v", err)
	}
	if len(mrs) != 1 || mrs[0].MRTitle != "Correct Title" {
		t.Fatalf("expected existing row to remain unchanged, got %+v", mrs)
	}
}

// TestCheckMRWatch_DoesNotRepublishWhenNothingVisibleChanged covers the
// smaller follow-up cubic-dev-ai raised alongside the identity-revalidation
// finding: refreshTaskMRFromWatch runs on every poll, not just "notable"
// pipeline/approval transitions, so publishing gitlab.task_mr.updated
// unconditionally on every poll would broadcast far more WS events than
// anything actually changed. A second poll with byte-identical MR data must
// not publish again.
func TestCheckMRWatch_DoesNotRepublishWhenNothingVisibleChanged(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", "https://gitlab.com", "group/proj")

	client := NewMockClient("https://gitlab.com")
	client.SeedMR("group/proj", &MR{
		IID: 5, State: mrStateOpen, HeadBranch: "feat/a", Title: "Feat A",
		WebURL: "https://gitlab.com/group/proj/-/merge_requests/5",
	})

	svc := NewService("https://gitlab.com", client, "none", nil, newTestLogger(t))
	svc.SetStore(store)
	eventBus := bus.NewMemoryEventBus(newTestLogger(t))
	svc.SetEventBus(eventBus)

	received := make(chan *bus.Event, 4)
	if _, err := eventBus.Subscribe(events.GitLabTaskMRUpdated, func(_ context.Context, evt *bus.Event) error {
		received <- evt
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	watch := &MRWatch{
		SessionID: "sess-1", TaskID: "task-1", RepositoryID: "repo-1",
		ProjectPath: "group/proj", MRIID: 5, Branch: "feat/a",
	}
	if err := store.CreateMRWatch(ctx, watch); err != nil {
		t.Fatalf("create watch: %v", err)
	}

	if _, _, err := svc.CheckMRWatch(ctx, watch); err != nil {
		t.Fatalf("check watch 1: %v", err)
	}
	if _, _, err := svc.CheckMRWatch(ctx, watch); err != nil {
		t.Fatalf("check watch 2: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("expected exactly 1 published event across 2 identical polls, got %d", len(received))
	}
}

// TestCheckMRWatch_DoesNotDeleteWatchOnNotFoundError guards the coderabbitai
// finding: ValidateTaskMRRepositoryIdentity returns ErrTaskMRNotFound (and
// wraps generic store errors) as well as the direct ErrTaskMRRepositoryMismatch
// sentinel. Only the mismatch sentinel proves the watch is durably wrong;
// ErrTaskMRNotFound can be a transient race (e.g. the task_repositories link
// row not yet visible) or a real backend problem, either of which should be
// retried rather than destroying the watch. Here the watch's repository_id
// points at a repository that was never linked to the task at all — a
// not-found, not a mismatch — so the watch must survive and the error must
// propagate instead of being swallowed as a clean "not valid" result.
func TestCheckMRWatch_DoesNotDeleteWatchOnNotFoundError(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	// No repository row at all — repo-1 is never linked via task_repositories.
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "")

	client := NewMockClient("https://gitlab.com")
	svc := NewService("https://gitlab.com", client, "none", nil, newTestLogger(t))
	svc.SetStore(store)

	watch := &MRWatch{
		SessionID: "sess-1", TaskID: "task-1", RepositoryID: "repo-1",
		ProjectPath: "group/proj", MRIID: 5, Branch: "feat/a",
	}
	if err := store.CreateMRWatch(ctx, watch); err != nil {
		t.Fatalf("create watch: %v", err)
	}

	_, _, err := svc.CheckMRWatch(ctx, watch)
	if !errors.Is(err, ErrTaskMRNotFound) {
		t.Fatalf("expected ErrTaskMRNotFound to propagate, got %v", err)
	}

	remaining, err := store.GetMRWatchBySessionRepoAndBranch(ctx, "sess-1", "repo-1", "feat/a")
	if err != nil {
		t.Fatalf("list watches: %v", err)
	}
	if remaining == nil {
		t.Fatal("expected the watch to survive a not-found (non-mismatch) error, but it was deleted")
	}
}

// TestCheckMRWatch_DeletesWatchWhoseRepositoryNoLongerMatchesTask guards the
// gap cubic-dev-ai flagged: refreshTaskMRFromWatch only validates the MR
// *returned by GitLab* against the watch's own (host, project, iid) — that
// alone still lets a stale or mis-scoped watch (whose repository_id was
// never actually tied to this GitLab project) poll and self-consistently
// upsert data anyway, since GetMRStatus itself only cares about project path
// and iid, not repository_id. The watch's own repository identity must be
// validated against the task before any GitLab request or upsert, and a
// mismatched watch is deleted rather than left to retry forever.
func TestCheckMRWatch_DeletesWatchWhoseRepositoryNoLongerMatchesTask(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	// repo-1 belongs to a different GitLab project than the watch below.
	setTaskMRRepositoryIdentity(t, store, "repo-1", "https://gitlab.com", "group/other")

	client := NewMockClient("https://gitlab.com")
	client.SeedMR("group/proj", &MR{
		IID: 5, State: mrStateOpen, HeadBranch: "feat/a", Title: "Feat A",
		WebURL: "https://gitlab.com/group/proj/-/merge_requests/5",
	})
	svc := NewService("https://gitlab.com", client, "none", nil, newTestLogger(t))
	svc.SetStore(store)

	watch := &MRWatch{
		SessionID: "sess-1", TaskID: "task-1", RepositoryID: "repo-1",
		ProjectPath: "group/proj", MRIID: 5, Branch: "feat/a",
	}
	if err := store.CreateMRWatch(ctx, watch); err != nil {
		t.Fatalf("create watch: %v", err)
	}

	status, notable, err := svc.CheckMRWatch(ctx, watch)
	if err != nil {
		t.Fatalf("check watch: %v", err)
	}
	if status != nil || notable {
		t.Fatalf("expected no status for a watch whose repository doesn't match the task, got status=%+v notable=%v", status, notable)
	}

	mrs, err := store.ListTaskMRsByTask(ctx, "task-1")
	if err != nil || len(mrs) != 0 {
		t.Fatalf("expected no task MR row created, got %d rows err=%v", len(mrs), err)
	}
	remaining, err := store.GetMRWatchBySessionRepoAndBranch(ctx, "sess-1", "repo-1", "feat/a")
	if err != nil {
		t.Fatalf("list watches: %v", err)
	}
	if remaining != nil {
		t.Fatalf("expected the mismatched watch to be deleted, got %+v", remaining)
	}
}
