package linear

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// TestCreateIssueWatch_DeniesForeignWorkspace is the regression for the review's
// P1: CreateIssueWatch takes WorkspaceID from the request body, which the
// query-only integration middleware never authorizes.
func TestCreateIssueWatch_DeniesForeignWorkspace(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	f.svc.SetWorkspaceAuthorizer(denyOnly("ws-victim"))

	_, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-victim", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: SearchFilter{Query: "bug"}, AgentProfileID: "ap",
	})
	if !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("expected ErrWorkspaceNotFound, got %v", err)
	}
	if all, _ := f.store.ListAllIssueWatches(ctx); len(all) != 0 {
		t.Fatalf("watch was created in a foreign workspace: %d rows", len(all))
	}
}

// TestUpdateIssueWatch_DeniesForeignWorkspace guards the by-ID mutation path:
// the watch ID alone reveals nothing about ownership.
func TestUpdateIssueWatch_DeniesForeignWorkspace(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	w := &IssueWatch{
		WorkspaceID: "ws-victim", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: SearchFilter{Query: "bug"}, AgentProfileID: "ap", Enabled: true,
	}
	if err := f.store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("seed watch: %v", err)
	}
	f.svc.SetWorkspaceAuthorizer(denyOnly("ws-victim"))

	if _, err := f.svc.UpdateIssueWatch(ctx, w.ID, &UpdateIssueWatchRequest{}); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

// TestListIssueWatches_DeniesForeignWorkspace covers the single-workspace list
// path (the scoped form the settings page uses with an explicit workspace_id).
func TestListIssueWatches_DeniesForeignWorkspace(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	f.svc.SetWorkspaceAuthorizer(denyOnly("ws-victim"))

	if _, err := f.svc.ListIssueWatches(ctx, "ws-victim"); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

// TestListAllIssueWatches_PropagatesAuthorizerError is the regression for the
// review's P2: a non-denial authorizer error (e.g. a transient DB failure) must
// surface, not be treated as a silent per-watch denial that returns a truncated
// 200.
func TestListAllIssueWatches_PropagatesAuthorizerError(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	if err := f.store.CreateIssueWatch(ctx, &IssueWatch{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: SearchFilter{Query: "bug"}, AgentProfileID: "ap", Enabled: true,
	}); err != nil {
		t.Fatalf("seed watch: %v", err)
	}
	boom := errors.New("authorizer backend down")
	f.svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return boom })

	if _, err := f.svc.ListAllIssueWatches(ctx); !errors.Is(err, boom) {
		t.Fatalf("expected the authorizer error to propagate, got %v", err)
	}
}

// TestTestConnection_DeniesForeignWorkspace is the regression for the review's
// P1 credential-exfiltration path.
func TestTestConnection_DeniesForeignWorkspace(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	f.svc.SetWorkspaceAuthorizer(denyOnly("ws-foreign"))

	result, err := f.svc.TestConnectionForWorkspace(ctx, "ws-foreign", &SetConfigRequest{
		AuthMethod: AuthMethodAPIKey, DefaultTeamKey: "ENG", Secret: "x",
	})
	if !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("expected ErrWorkspaceNotFound, got %v (result=%+v)", err, result)
	}
}

// TestDataPlane_DeniesForeignWorkspace covers the clientFor chokepoint that all
// data-plane reads funnel through.
func TestDataPlane_DeniesForeignWorkspace(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	f.svc.SetWorkspaceAuthorizer(denyOnly("ws-foreign"))

	if _, err := f.svc.SearchIssuesForWorkspace(ctx, "ws-foreign", SearchFilter{Query: "bug"}, "", 10); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("expected ErrWorkspaceNotFound, got %v", err)
	}
}
