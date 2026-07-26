package jira

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// denyOnly returns a workspace authorizer that allows every workspace except
// the named ones, which it denies with the same ErrWorkspaceNotFound the real
// task-service authorizer returns.
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

// TestCopyConfigToWorkspace_DeniesForeignTarget is the regression for the HIGH
// finding: the copy target arrives in the request body, which the query-only
// integration middleware never authorizes, so without a service-layer check a
// caller could copy their config + credential into another user's workspace.
func TestCopyConfigToWorkspace_DeniesForeignTarget(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	const src, victim = "ws-src", "ws-victim"

	// Seed the attacker's own source config while unscoped, then turn on scoping.
	if _, err := f.svc.SetConfigForWorkspace(ctx, src, &SetConfigRequest{
		SiteURL:      "https://attacker.atlassian.net",
		Email:        "a@example.com",
		AuthMethod:   AuthMethodAPIToken,
		InstanceType: InstanceTypeCloud,
		Secret:       "tok-attacker",
	}); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	drainProbe(t, f)

	// The victim already has a healthy Jira connection of their own. A denied
	// copy must leave it byte-for-byte untouched — including its health status,
	// which the up-front guard protects from the pre-write UpdateAuthHealth flip.
	if err := f.store.UpsertConfigForWorkspace(ctx, victim, &JiraConfig{
		SiteURL: "https://victim.atlassian.net", Email: "v@example.com",
		AuthMethod: AuthMethodAPIToken, InstanceType: InstanceTypeCloud,
	}); err != nil {
		t.Fatalf("seed victim config: %v", err)
	}
	if err := f.secrets.Set(ctx, SecretKeyForWorkspace(victim), "Jira token", "tok-victim"); err != nil {
		t.Fatalf("seed victim secret: %v", err)
	}
	if err := f.store.UpdateAuthHealthForWorkspace(ctx, victim, true, "", time.Now().UTC()); err != nil {
		t.Fatalf("seed victim health: %v", err)
	}

	f.svc.SetWorkspaceAuthorizer(denyOnly(victim))

	_, err := f.svc.CopyConfigToWorkspace(ctx, src, victim)
	if !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("expected ErrWorkspaceNotFound, got %v", err)
	}
	got, cfgErr := f.store.GetConfigForWorkspace(ctx, victim)
	if cfgErr != nil || got == nil {
		t.Fatalf("victim config vanished: cfg=%+v err=%v", got, cfgErr)
	}
	if got.SiteURL != "https://victim.atlassian.net" {
		t.Errorf("victim config overwritten by copy: %q", got.SiteURL)
	}
	if !got.LastOk {
		t.Errorf("victim health flipped to unhealthy by unauthorized copy")
	}
	if v, _ := f.secrets.Reveal(ctx, SecretKeyForWorkspace(victim)); v != "tok-victim" {
		t.Errorf("victim secret overwritten by copy: %q", v)
	}
}

// TestConfigForWorkspace_DeniesForeignWorkspace is the regression for the LOW
// finding: a scoped caller must not read, write, or delete a workspace's config
// it does not own (including via the resolved default when workspace_id is
// omitted — resolveWorkspaceID authorizes whatever normalize resolves).
func TestConfigForWorkspace_DeniesForeignWorkspace(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	const foreign = "ws-foreign"
	f.svc.SetWorkspaceAuthorizer(denyOnly(foreign))

	if _, err := f.svc.GetConfigForWorkspace(ctx, foreign); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Errorf("Get: expected ErrWorkspaceNotFound, got %v", err)
	}
	if _, err := f.svc.SetConfigForWorkspace(ctx, foreign, &SetConfigRequest{
		SiteURL: "https://x.atlassian.net", Email: "x@example.com",
		AuthMethod: AuthMethodAPIToken, InstanceType: InstanceTypeCloud, Secret: "t",
	}); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Errorf("Set: expected ErrWorkspaceNotFound, got %v", err)
	}
	if err := f.svc.DeleteConfigForWorkspace(ctx, foreign); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Errorf("Delete: expected ErrWorkspaceNotFound, got %v", err)
	}
}

// TestListAllIssueWatches_FiltersToCallerWorkspaces is the regression for the
// MEDIUM finding: the unscoped list-all form (reached by omitting workspace_id)
// leaked every workspace's watch config. A scoped caller now sees only their
// own; an identity-less internal caller (nil authorizer) still sees all.
func TestListAllIssueWatches_FiltersToCallerWorkspaces(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()

	seed := func(ws, jql string) {
		if err := f.store.CreateIssueWatch(ctx, &IssueWatch{
			WorkspaceID: ws, WorkflowID: "wf", WorkflowStepID: "step",
			JQL: jql, AgentProfileID: "ap", Enabled: true,
		}); err != nil {
			t.Fatalf("seed watch: %v", err)
		}
	}
	seed("ws-mine", "project = MINE")
	seed("ws-mine", "project = MINE2")
	seed("ws-other", "project = OTHER")

	// Unscoped (internal caller): every watch is visible, as before auth.
	if all, err := f.svc.ListAllIssueWatches(ctx); err != nil || len(all) != 3 {
		t.Fatalf("unscoped: want 3 watches, got %d (err=%v)", len(all), err)
	}

	// Scoped: only the caller's own workspace watches survive the filter.
	f.svc.SetWorkspaceAuthorizer(denyOnly("ws-other"))
	mine, err := f.svc.ListAllIssueWatches(ctx)
	if err != nil {
		t.Fatalf("scoped list: %v", err)
	}
	if len(mine) != 2 {
		t.Fatalf("scoped: want 2 watches, got %d", len(mine))
	}
	for _, w := range mine {
		if w.WorkspaceID != "ws-mine" {
			t.Errorf("leaked foreign watch from %q", w.WorkspaceID)
		}
	}
}
