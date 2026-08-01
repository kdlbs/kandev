package gitlab

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestCheckMRWatch_RefreshesTaskMRAndPublishes(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")

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
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")

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
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")

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
