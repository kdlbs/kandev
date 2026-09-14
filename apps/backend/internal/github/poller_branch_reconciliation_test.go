package github

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

type watchIdentityProvider struct{ targets []TaskBranchInfo }

func (p *watchIdentityProvider) ListTasksNeedingPRWatch(context.Context) ([]TaskBranchInfo, error) {
	return p.targets, nil
}
func (p *watchIdentityProvider) ResolveBranchForWatch(_ context.Context, w *PRWatch) string {
	for _, target := range p.targets {
		if target.SessionID == w.SessionID && target.RepositoryID == w.RepositoryID && target.Branch == w.Branch {
			return target.Branch
		}
	}
	return ""
}

// @covers AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.1
// @covers AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.4
func TestReconcileWatches_PreservesBranchTargetsAcrossCycles(t *testing.T) {
	poller, svc, client, store := setupBatchedPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "owner", false)
	provider := &watchIdentityProvider{targets: []TaskBranchInfo{
		{WorkspaceID: testWorkspaceID, TaskID: "owner", SessionID: "member-session", RepositoryID: "repo1", Owner: "o", Repo: "r", Branch: "primary"},
		{WorkspaceID: testWorkspaceID, TaskID: "owner", SessionID: "member-session", RepositoryID: "repo1", Owner: "o", Repo: "r", Branch: "secondary"},
		{WorkspaceID: testWorkspaceID, TaskID: "owner", SessionID: "member-session", RepositoryID: "repo2", Owner: "o", Repo: "other", Branch: "other-branch"},
	}}
	poller.SetTaskBranchProvider(provider)
	poller.reconcileWatches(ctx)
	initial, err := store.ListActivePRWatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(initial) != 3 {
		t.Fatalf("initial watches = %d, want 3", len(initial))
	}
	// A numbered sibling must survive while the other branches still search.
	for _, w := range initial {
		if w.Branch == "primary" {
			if err := store.UpdatePRWatchPRNumber(ctx, w.ID, 99); err != nil {
				t.Fatal(err)
			}
		}
	}
	initial, err = store.ListActivePRWatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assertWatchReconciliationStable(t, poller, store, initial)
	assertSecondaryWatchDiscovery(t, svc, client, store)
}

func assertWatchReconciliationStable(t *testing.T, poller *Poller, store *Store, initial []*PRWatch) {
	t.Helper()
	ctx := context.Background()
	for range 3 {
		poller.reconcileWatches(ctx)
		current, err := store.ListActivePRWatches(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(current) != len(initial) {
			t.Fatalf("watch count = %d, want %d", len(current), len(initial))
		}
		for _, before := range initial {
			after, err := store.GetPRWatchBySessionRepoAndBranch(ctx, before.SessionID, before.RepositoryID, before.Branch)
			if err != nil {
				t.Fatal(err)
			}
			if after == nil || after.ID != before.ID || !after.UpdatedAt.Equal(before.UpdatedAt) {
				t.Fatalf("watch %s recreated or rewritten: before=%+v after=%+v", before.Branch, before, after)
			}
			if before.Branch == "primary" && after.PRNumber != 99 {
				t.Fatal("numbered sibling changed")
			}
		}
	}
}

func assertSecondaryWatchDiscovery(t *testing.T, svc *Service, client *graphQLMockClient, store *Store) {
	t.Helper()
	ctx := context.Background()
	secondary, err := store.GetPRWatchBySessionRepoAndBranch(ctx, "member-session", "repo1", "secondary")
	if err != nil {
		t.Fatal(err)
	}
	updated := make(chan *TaskPR, 1)
	sub, err := svc.eventBus.Subscribe(events.GitHubTaskPRUpdated, func(_ context.Context, event *bus.Event) error {
		if pr, ok := event.Data.(*TaskPR); ok {
			updated <- pr
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	client.branchResponses = []string{branchAliasResponse(t, 1, map[int]string{0: openPRNode(7, "secondary", "alice")})}
	if _, err := svc.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, []*PRWatch{secondary}); err != nil {
		t.Fatal(err)
	}
	associated, err := store.GetTaskPR(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if associated == nil || associated.PRNumber != 7 || associated.RepositoryID != "repo1" {
		t.Fatalf("association = %+v", associated)
	}
	select {
	case event := <-updated:
		if event.TaskID != "owner" || event.PRNumber != 7 || event.RepositoryID != "repo1" {
			t.Fatalf("event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("missing task PR event")
	}
}
