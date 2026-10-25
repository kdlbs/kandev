package jira

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
		JQL: "project = X", AgentProfileID: "ap",
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
		JQL: "project = X", AgentProfileID: "ap", Enabled: true,
	}
	if err := f.store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("seed watch: %v", err)
	}
	f.svc.SetWorkspaceAuthorizer(denyOnly("ws-victim"))

	if _, err := f.svc.UpdateIssueWatch(ctx, w.ID, &UpdateIssueWatchRequest{}); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

func TestIssueWatchIDOperationsDenyForeignWorkspace(t *testing.T) {
	operations := []struct {
		name string
		run  func(*Service, string) error
	}{
		{name: "get", run: func(s *Service, id string) error { _, err := s.GetIssueWatch(context.Background(), id); return err }},
		{name: "delete", run: func(s *Service, id string) error { return s.DeleteIssueWatch(context.Background(), id) }},
		{name: "preview reset", run: func(s *Service, id string) error {
			_, err := s.PreviewResetIssueWatch(context.Background(), id)
			return err
		}},
		{name: "reset", run: func(s *Service, id string) error { _, err := s.ResetIssueWatch(context.Background(), id); return err }},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			f := newSvcFixture(t)
			w := &IssueWatch{
				WorkspaceID: "ws-victim", WorkflowID: "wf", WorkflowStepID: "step",
				JQL: "project = X", AgentProfileID: "ap", Enabled: true,
			}
			if err := f.store.CreateIssueWatch(context.Background(), w); err != nil {
				t.Fatalf("seed watch: %v", err)
			}
			f.svc.SetWorkspaceAuthorizer(denyOnly("ws-victim"))
			if err := operation.run(f.svc, w.ID); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
				t.Fatalf("expected ErrWorkspaceNotFound, got %v", err)
			}
			if got, err := f.store.GetIssueWatch(context.Background(), w.ID); err != nil || got == nil {
				t.Fatalf("denied operation removed watch: watch=%+v err=%v", got, err)
			}
		})
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
		JQL: "project = X", AgentProfileID: "ap", Enabled: true,
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
// P1 credential-exfiltration path: a caller-supplied siteUrl must not cause the
// default/foreign workspace's stored token to be sent anywhere.
func TestTestConnection_DeniesForeignWorkspace(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	f.svc.SetWorkspaceAuthorizer(denyOnly("ws-foreign"))

	result, err := f.svc.TestConnectionForWorkspace(ctx, "ws-foreign", &SetConfigRequest{
		SiteURL: "https://attacker.example", Email: "a@example.com",
		AuthMethod: AuthMethodAPIToken, InstanceType: InstanceTypeCloud, Secret: "x",
	})
	if !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("expected ErrWorkspaceNotFound, got %v (result=%+v)", err, result)
	}
	// The client factory (which would perform the outbound auth call) must never
	// have been reached.
	if f.factoryHit.Load() != 0 {
		t.Fatalf("client was built for a denied workspace: %d hits", f.factoryHit.Load())
	}
}

// TestDataPlane_DeniesForeignWorkspace covers the clientFor chokepoint that all
// data-plane reads (search, projects, statuses) funnel through.
func TestDataPlane_DeniesForeignWorkspace(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	f.svc.SetWorkspaceAuthorizer(denyOnly("ws-foreign"))

	if _, err := f.svc.SearchTicketsForWorkspace(ctx, "ws-foreign", "project = X", "", 10); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("expected ErrWorkspaceNotFound, got %v", err)
	}
}
