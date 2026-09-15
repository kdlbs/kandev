package gitlab

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// denyOnly returns a workspace authorizer that allows every workspace except
// the named ones, which it denies with the same ErrWorkspaceNotFound the real
// task-service authorizer returns. Mirrors internal/jira/scope_test.go.
func denyOnly(denied ...string) func(context.Context, string) error {
	blocked := make(map[string]bool, len(denied))
	for _, ws := range denied {
		blocked[ws] = true
	}
	return func(_ context.Context, ws string) error {
		if blocked[ws] {
			return repoerrors.ErrWorkspaceNotFound
		}
		return nil
	}
}

func newScopeWatchFixture(t *testing.T) (*Service, *Store) {
	t.Helper()
	store := newTestStore(t)
	svc := NewService("", nil, AuthMethodNone, nil, newTestLogger(t))
	svc.SetStore(store)
	return svc, store
}

func seedIssueWatch(t *testing.T, store *Store, workspaceID string) *IssueWatch {
	t.Helper()
	w := &IssueWatch{
		WorkspaceID: workspaceID, WorkflowID: "wf", WorkflowStepID: "step",
		AgentProfileID: "agent", ExecutorProfileID: "exec", Enabled: true,
	}
	if err := store.CreateIssueWatch(context.Background(), w); err != nil {
		t.Fatalf("seed issue watch: %v", err)
	}
	return w
}

// TestListAllIssueWatches_FiltersToAccessibleWorkspaces is the regression for
// the gap noted in AGENTS.md's "Per-user scoping" section: ListAllIssueWatches
// was a bare passthrough to the store with no workspace filter.
func TestListAllIssueWatches_FiltersToAccessibleWorkspaces(t *testing.T) {
	svc, store := newScopeWatchFixture(t)
	ctx := context.Background()
	seedIssueWatch(t, store, "ws-allowed")
	seedIssueWatch(t, store, "ws-denied")
	svc.SetWorkspaceAuthorizer(denyOnly("ws-denied"))

	watches, err := svc.ListAllIssueWatches(ctx)
	if err != nil {
		t.Fatalf("ListAllIssueWatches: %v", err)
	}
	if len(watches) != 1 || watches[0].WorkspaceID != "ws-allowed" {
		t.Fatalf("watches = %+v, want only ws-allowed's watch", watches)
	}
}

// TestListAllIssueWatches_NilAuthorizerReturnsEveryWorkspace preserves the
// identity-less internal caller behaviour: poller.go's ListAllIssueWatches
// call must keep seeing every watch when no authorizer is wired.
func TestListAllIssueWatches_NilAuthorizerReturnsEveryWorkspace(t *testing.T) {
	svc, store := newScopeWatchFixture(t)
	ctx := context.Background()
	seedIssueWatch(t, store, "ws-1")
	seedIssueWatch(t, store, "ws-2")

	watches, err := svc.ListAllIssueWatches(ctx)
	if err != nil {
		t.Fatalf("ListAllIssueWatches: %v", err)
	}
	if len(watches) != 2 {
		t.Fatalf("watches = %+v, want 2 (nil authorizer must stay unscoped for internal callers like the poller)", watches)
	}
}

// TestListAllIssueWatches_PropagatesAuthorizerError is the regression for a
// non-denial authorizer error (e.g. a transient DB failure): it must surface,
// not be treated as a silent per-watch denial that returns a truncated list.
func TestListAllIssueWatches_PropagatesAuthorizerError(t *testing.T) {
	svc, store := newScopeWatchFixture(t)
	ctx := context.Background()
	seedIssueWatch(t, store, "ws-1")
	boom := errors.New("authorizer backend down")
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return boom })

	if _, err := svc.ListAllIssueWatches(ctx); !errors.Is(err, boom) {
		t.Fatalf("expected the authorizer error to propagate, got %v", err)
	}
}
