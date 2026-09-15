package sentry

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

// TestListAllIssueWatches_FiltersToAccessibleWorkspaces is the regression for
// the gap noted in AGENTS.md's "Per-user scoping" section: ListAllIssueWatches
// was a bare passthrough to the store with no workspace filter, so
// GET /api/v1/sentry/watches/issue with workspace_id omitted returned every
// workspace's watch config to any authenticated caller.
func TestListAllIssueWatches_FiltersToAccessibleWorkspaces(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	if err := f.store.CreateIssueWatch(ctx, &IssueWatch{
		WorkspaceID: "ws-allowed", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: SearchFilter{OrgSlug: "org", ProjectSlugs: []string{"proj"}}, AgentProfileID: "ap", Enabled: true,
	}); err != nil {
		t.Fatalf("seed allowed watch: %v", err)
	}
	if err := f.store.CreateIssueWatch(ctx, &IssueWatch{
		WorkspaceID: "ws-denied", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: SearchFilter{OrgSlug: "org", ProjectSlugs: []string{"proj"}}, AgentProfileID: "ap", Enabled: true,
	}); err != nil {
		t.Fatalf("seed denied watch: %v", err)
	}
	f.svc.SetWorkspaceAuthorizer(denyOnly("ws-denied"))

	watches, err := f.svc.ListAllIssueWatches(ctx)
	if err != nil {
		t.Fatalf("ListAllIssueWatches: %v", err)
	}
	if len(watches) != 1 || watches[0].WorkspaceID != "ws-allowed" {
		t.Fatalf("watches = %+v, want only ws-allowed's watch", watches)
	}
}

// TestListAllIssueWatches_NilAuthorizerReturnsEveryWorkspace preserves the
// identity-less internal caller behaviour: poller.go's ListAllIssueWatches
// call must keep seeing every watch when no authorizer is wired. Mirrors
// TestIssueAndPRAllWorkspaceListsRetainIdentitylessInternalUse
// (internal/github/service_workspace_authorization_test.go).
func TestListAllIssueWatches_NilAuthorizerReturnsEveryWorkspace(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	for _, ws := range []string{"ws-1", "ws-2"} {
		if err := f.store.CreateIssueWatch(ctx, &IssueWatch{
			WorkspaceID: ws, WorkflowID: "wf", WorkflowStepID: "step",
			Filter: SearchFilter{OrgSlug: "org", ProjectSlugs: []string{"proj"}}, AgentProfileID: "ap", Enabled: true,
		}); err != nil {
			t.Fatalf("seed watch: %v", err)
		}
	}

	watches, err := f.svc.ListAllIssueWatches(ctx)
	if err != nil {
		t.Fatalf("ListAllIssueWatches: %v", err)
	}
	if len(watches) != 2 {
		t.Fatalf("watches = %+v, want 2 (nil authorizer must stay unscoped for internal callers like the poller)", watches)
	}
}

// TestListAllIssueWatches_PropagatesAuthorizerError is the regression for a
// non-denial authorizer error (e.g. a transient DB failure): it must surface,
// not be treated as a silent per-watch denial that returns a truncated 200.
// Mirrors internal/jira/scope_watch_test.go.
func TestListAllIssueWatches_PropagatesAuthorizerError(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	if err := f.store.CreateIssueWatch(ctx, &IssueWatch{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: SearchFilter{OrgSlug: "org", ProjectSlugs: []string{"proj"}}, AgentProfileID: "ap", Enabled: true,
	}); err != nil {
		t.Fatalf("seed watch: %v", err)
	}
	boom := errors.New("authorizer backend down")
	f.svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return boom })

	if _, err := f.svc.ListAllIssueWatches(ctx); !errors.Is(err, boom) {
		t.Fatalf("expected the authorizer error to propagate, got %v", err)
	}
}
