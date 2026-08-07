package gitlab

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestAutoLinkMRForBranch_CreatesAssociationAndWatch(t *testing.T) {
	const host = "https://gitlab.acme.test"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", host, "group/proj")
	client.SeedMR("group/proj", &MR{
		IID: 7, Title: "Feat A", HeadBranch: "feat/a", State: mrStateOpen,
		WebURL: host + "/group/proj/-/merge_requests/7", CreatedAt: time.Now().UTC(),
	})

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

	assoc, err := svc.AutoLinkMRForBranch(context.Background(), "ws-1", "sess-1", "task-1", "repo-1", "group/proj", "feat/a")
	if err != nil {
		t.Fatalf("AutoLinkMRForBranch: %v", err)
	}
	if assoc == nil || assoc.MRIID != 7 || assoc.RepositoryID != "repo-1" {
		t.Fatalf("unexpected association: %+v", assoc)
	}

	watch, err := store.GetMRWatchBySessionRepoAndBranch(context.Background(), "sess-1", "repo-1", "feat/a")
	if err != nil || watch == nil || watch.MRIID != 7 {
		t.Fatalf("expected watch created with iid 7, got %+v err=%v", watch, err)
	}

	select {
	case <-received:
	default:
		t.Fatal("expected gitlab.task_mr.updated to be published")
	}
}

func TestAutoLinkMRForBranch_NoOpenMR(t *testing.T) {
	const host = "https://gitlab.acme.test"
	svc, store, _ := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", host, "group/proj")

	assoc, err := svc.AutoLinkMRForBranch(context.Background(), "ws-1", "sess-1", "task-1", "repo-1", "group/proj", "feat/none")
	if err != nil {
		t.Fatalf("AutoLinkMRForBranch: %v", err)
	}
	if assoc != nil {
		t.Fatalf("expected nil association when no MR is open, got %+v", assoc)
	}

	rows, err := store.ListTaskMRsByTask(context.Background(), "task-1")
	if err != nil || len(rows) != 0 {
		t.Fatalf("expected no rows created, got %d err=%v", len(rows), err)
	}
}

// TestAutoLinkMRForBranch_NoOpenMRLeavesPlaceholderWatch covers late MR
// discovery: when push detection's own [0, 30s, 60s] retry window closes
// with no MR open yet, a placeholder (iid=0) watch must still exist
// afterward so an MR opened later — e.g. from the GitLab web UI, well after
// the retry window — is discovered by the poller's own iid<=0 resolution
// instead of being lost forever.
func TestAutoLinkMRForBranch_NoOpenMRLeavesPlaceholderWatch(t *testing.T) {
	const host = "https://gitlab.acme.test"
	svc, store, _ := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", host, "group/proj")

	assoc, err := svc.AutoLinkMRForBranch(context.Background(), "ws-1", "sess-1", "task-1", "repo-1", "group/proj", "feat/none")
	if err != nil {
		t.Fatalf("AutoLinkMRForBranch: %v", err)
	}
	if assoc != nil {
		t.Fatalf("expected nil association when no MR is open, got %+v", assoc)
	}

	watch, err := store.GetMRWatchBySessionRepoAndBranch(context.Background(), "sess-1", "repo-1", "feat/none")
	if err != nil || watch == nil {
		t.Fatalf("expected a placeholder watch to be created, got %+v err=%v", watch, err)
	}
	if watch.MRIID != 0 {
		t.Fatalf("expected placeholder watch iid=0, got %d", watch.MRIID)
	}
}

func TestAutoLinkMRForBranch_Idempotent(t *testing.T) {
	const host = "https://gitlab.acme.test"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", host, "group/proj")
	client.SeedMR("group/proj", &MR{
		IID: 7, Title: "Feat A", HeadBranch: "feat/a", State: mrStateOpen,
		WebURL: host + "/group/proj/-/merge_requests/7", CreatedAt: time.Now().UTC(),
	})

	for i := 0; i < 2; i++ {
		if _, err := svc.AutoLinkMRForBranch(context.Background(), "ws-1", "sess-1", "task-1", "repo-1", "group/proj", "feat/a"); err != nil {
			t.Fatalf("AutoLinkMRForBranch run %d: %v", i, err)
		}
	}

	rows, err := store.ListTaskMRsByTask(context.Background(), "task-1")
	if err != nil || len(rows) != 1 {
		t.Fatalf("expected exactly 1 task MR row, got %d err=%v", len(rows), err)
	}
	watches, err := store.ListMRWatchesByTask(context.Background(), "task-1")
	if err != nil || len(watches) != 1 {
		t.Fatalf("expected exactly 1 watch row, got %d err=%v", len(watches), err)
	}
}

func TestAutoLinkMRForBranch_RejectsRepositoryIdentityMismatch(t *testing.T) {
	const host = "https://gitlab.acme.test"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", host, "group/other")
	client.SeedMR("group/proj", &MR{
		IID: 7, Title: "Feat A", HeadBranch: "feat/a", State: mrStateOpen,
		WebURL: host + "/group/proj/-/merge_requests/7", CreatedAt: time.Now().UTC(),
	})

	_, err := svc.AutoLinkMRForBranch(context.Background(), "ws-1", "sess-1", "task-1", "repo-1", "group/proj", "feat/a")
	if !errors.Is(err, ErrTaskMRRepositoryMismatch) {
		t.Fatalf("error = %v, want ErrTaskMRRepositoryMismatch", err)
	}

	rows, _ := store.ListTaskMRsByTask(context.Background(), "task-1")
	if len(rows) != 0 {
		t.Fatalf("expected no association on identity mismatch, got %d rows", len(rows))
	}
}

func TestAutoLinkMRForBranch_MultiBranchCoexist(t *testing.T) {
	const host = "https://gitlab.acme.test"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", host, "group/proj")
	client.SeedMR("group/proj", &MR{
		IID: 7, Title: "Feat A", HeadBranch: "feat/a", State: mrStateOpen,
		WebURL: host + "/group/proj/-/merge_requests/7", CreatedAt: time.Now().UTC(),
	})
	client.SeedMR("group/proj", &MR{
		IID: 8, Title: "Feat B", HeadBranch: "feat/b", State: mrStateOpen,
		WebURL: host + "/group/proj/-/merge_requests/8", CreatedAt: time.Now().UTC(),
	})

	if _, err := svc.AutoLinkMRForBranch(context.Background(), "ws-1", "sess-1", "task-1", "repo-1", "group/proj", "feat/a"); err != nil {
		t.Fatalf("link branch a: %v", err)
	}
	if _, err := svc.AutoLinkMRForBranch(context.Background(), "ws-1", "sess-1", "task-1", "repo-1", "group/proj", "feat/b"); err != nil {
		t.Fatalf("link branch b: %v", err)
	}

	rows, err := store.ListTaskMRsByTask(context.Background(), "task-1")
	if err != nil || len(rows) != 2 {
		t.Fatalf("expected 2 task MR rows for 2 branches, got %d err=%v", len(rows), err)
	}
	watches, err := store.ListMRWatchesByTask(context.Background(), "task-1")
	if err != nil || len(watches) != 2 {
		t.Fatalf("expected 2 watch rows for 2 branches, got %d err=%v", len(watches), err)
	}
}

func TestAutoLinkMRForBranch_MultiRepoScoping(t *testing.T) {
	const host = "https://gitlab.acme.test"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", host, "group/front")
	if _, err := store.db.Exec(`INSERT INTO repositories (id, workspace_id) VALUES ('repo-2', 'ws-1');
		INSERT INTO task_repositories (id, task_id, repository_id) VALUES ('task-repo-2', 'task-1', 'repo-2')`); err != nil {
		t.Fatalf("seed second repo: %v", err)
	}
	setTaskMRRepositoryIdentity(t, store, "repo-2", host, "group/back")
	client.SeedMR("group/front", &MR{
		IID: 1, Title: "Front", HeadBranch: "feat/x", State: mrStateOpen,
		WebURL: host + "/group/front/-/merge_requests/1", CreatedAt: time.Now().UTC(),
	})
	client.SeedMR("group/back", &MR{
		IID: 2, Title: "Back", HeadBranch: "feat/x", State: mrStateOpen,
		WebURL: host + "/group/back/-/merge_requests/2", CreatedAt: time.Now().UTC(),
	})

	if _, err := svc.AutoLinkMRForBranch(context.Background(), "ws-1", "sess-1", "task-1", "repo-1", "group/front", "feat/x"); err != nil {
		t.Fatalf("link repo-1: %v", err)
	}
	if _, err := svc.AutoLinkMRForBranch(context.Background(), "ws-1", "sess-1", "task-1", "repo-2", "group/back", "feat/x"); err != nil {
		t.Fatalf("link repo-2: %v", err)
	}

	front, err := store.ListTaskMRsByTask(context.Background(), "task-1")
	if err != nil || len(front) != 2 {
		t.Fatalf("expected 2 rows scoped by repository, got %d err=%v", len(front), err)
	}
	byRepo := map[string]int{}
	for _, r := range front {
		byRepo[r.RepositoryID] = r.MRIID
	}
	if byRepo["repo-1"] != 1 || byRepo["repo-2"] != 2 {
		t.Fatalf("cross-repo overwrite: %+v", byRepo)
	}
}

// TestAutoLinkMRForBranch_RejectsBlankBranch guards this at the service
// layer directly (not just at the orchestrator's push-detection call site):
// a blank branch must never reach FindMRByBranch, which would otherwise
// interpolate it into a query with no effective source-branch filter and
// link an arbitrary open MR of the project to the task.
func TestAutoLinkMRForBranch_RejectsBlankBranch(t *testing.T) {
	const host = "https://gitlab.acme.test"
	for _, branch := range []string{"", "   ", "\t\n"} {
		t.Run("branch="+branch, func(t *testing.T) {
			svc, store, client := newTaskMRLinkService(t, host)
			seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
			setTaskMRRepositoryIdentity(t, store, "repo-1", host, "group/proj")
			client.SeedMR("group/proj", &MR{
				IID: 7, Title: "Feat A", HeadBranch: "feat/a", State: mrStateOpen,
				WebURL: host + "/group/proj/-/merge_requests/7", CreatedAt: time.Now().UTC(),
			})

			assoc, err := svc.AutoLinkMRForBranch(context.Background(), "ws-1", "sess-1", "task-1", "repo-1", "group/proj", branch)
			if err != nil {
				t.Fatalf("AutoLinkMRForBranch: %v", err)
			}
			if assoc != nil {
				t.Fatalf("expected nil association for a blank branch, got %+v", assoc)
			}

			rows, _ := store.ListTaskMRsByTask(context.Background(), "task-1")
			if len(rows) != 0 {
				t.Fatalf("expected no association created for a blank branch, got %d rows", len(rows))
			}
		})
	}
}
