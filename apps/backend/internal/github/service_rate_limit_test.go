package github

import (
	"context"
	"testing"
	"time"
)

func TestServiceGetWorkspaceRateLimitSnapshotReportsPrimarySecondaryDisagreement(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	connection := &WorkspaceConnection{
		WorkspaceID: "workspace-1", Source: ConnectionSourcePAT,
		GitHubHost: defaultGitHubHost, Login: "yattdev", Status: ConnectionStatusActive,
	}
	if err := store.UpsertWorkspaceConnection(context.Background(), connection); err != nil {
		t.Fatal(err)
	}
	svc := NewService(nil, AuthMethodPAT, nil, store, nil, testLogger(t))
	t.Cleanup(svc.Stop)
	tracker, _ := svc.rateCoordinator.coordinate(defaultGitHubHost, AuthPrincipal{
		Kind: AuthPrincipalHuman, Source: ConnectionSourcePAT,
		Login: "yattdev", WorkspaceID: "workspace-1",
	}, nil)
	now := time.Now().UTC()
	tracker.Record(RateSnapshot{
		Resource: ResourceCore, Limit: 5000, Remaining: 5000,
		ResetAt: now.Add(time.Hour), UpdatedAt: now,
	})
	tracker.Record(RateSnapshot{
		Resource: ResourceGraphQL, Limit: 5000, Remaining: 5000,
		ResetAt: now.Add(time.Hour), UpdatedAt: now,
	})
	tracker.ObserveSecondary(
		ResourceCore,
		now.Add(20*time.Minute),
		RetrySourceRetryAfter,
		"API rate limit exceeded for user ID 79718216",
	)

	snapshot, err := svc.GetWorkspaceRateLimitSnapshot(context.Background(), "workspace-1")
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Core.Known || !snapshot.Core.Fresh || snapshot.Core.Remaining != 5000 || snapshot.Core.Limit != 5000 {
		t.Fatalf("core snapshot = %+v", snapshot.Core)
	}
	if !snapshot.GraphQL.Known || snapshot.GraphQL.Remaining != 5000 {
		t.Fatalf("graphql snapshot = %+v", snapshot.GraphQL)
	}
	if !snapshot.Secondary.Active || snapshot.Secondary.RetrySource != RetrySourceRetryAfter ||
		snapshot.Secondary.Resource != ResourceCore {
		t.Fatalf("secondary snapshot = %+v", snapshot.Secondary)
	}
	if snapshot.InteractiveAllowed || snapshot.BackgroundAllowed || snapshot.BlockingReason != "observed_secondary_rate_limit" {
		t.Fatalf("admission snapshot = %+v", snapshot)
	}

	// A successful accepted request clears Kandev's local estimate even when
	// an earlier refusal advertised a later reset. The live incident cleared
	// seventeen minutes before the 403's reset timestamp.
	tracker.ObserveSuccess(ResourceCore)
	snapshot, err = svc.GetWorkspaceRateLimitSnapshot(context.Background(), "workspace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Secondary.Active || !snapshot.InteractiveAllowed || !snapshot.BackgroundAllowed {
		t.Fatalf("snapshot after accepted response = %+v", snapshot)
	}
}

func TestServiceGetWorkspaceRateLimitSnapshotColdStateDoesNotRequireProvider(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	if err := store.UpsertWorkspaceConnection(context.Background(), &WorkspaceConnection{
		WorkspaceID: "workspace-1", Source: ConnectionSourceGHCLI,
		GitHubHost: defaultGitHubHost, Login: "yattdev", Status: ConnectionStatusActive,
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	t.Cleanup(svc.Stop)

	snapshot, err := svc.GetWorkspaceRateLimitSnapshot(context.Background(), "workspace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Core.Known || snapshot.GraphQL.Known || snapshot.Secondary.Active {
		t.Fatalf("cold snapshot = %+v", snapshot)
	}
	if !snapshot.InteractiveAllowed || !snapshot.BackgroundAllowed {
		t.Fatalf("cold local state should not invent a block: %+v", snapshot)
	}
}

func TestServiceGetWorkspaceRateLimitSnapshotLabelsConservativeFallback(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	if err := store.UpsertWorkspaceConnection(context.Background(), &WorkspaceConnection{
		WorkspaceID: "workspace-1", Source: ConnectionSourcePAT,
		GitHubHost: defaultGitHubHost, Login: "yattdev", Status: ConnectionStatusActive,
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(nil, AuthMethodPAT, nil, store, nil, testLogger(t))
	t.Cleanup(svc.Stop)
	tracker, _ := svc.rateCoordinator.coordinate(defaultGitHubHost, AuthPrincipal{
		Kind: AuthPrincipalHuman, Source: ConnectionSourcePAT,
		Login: "yattdev", WorkspaceID: "workspace-1",
	}, nil)
	tracker.ObserveSecondary(
		ResourceGraphQL,
		time.Now().Add(secondaryFallbackDelay),
		RetrySourceConservativeFallback,
		"secondary rate limit",
	)

	snapshot, err := svc.GetWorkspaceRateLimitSnapshot(context.Background(), "workspace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Secondary.RetrySource != RetrySourceConservativeFallback {
		t.Fatalf("secondary snapshot = %+v", snapshot.Secondary)
	}
}

func TestObservedSecondarySnapshotUsesOverallEnforcedRetryBoundary(t *testing.T) {
	tracker := NewRateTracker(nil, nil)
	now := time.Now().UTC()
	tracker.ObserveSecondary(
		ResourceGraphQL, now.Add(15*time.Minute), RetrySourceConservativeFallback, "graphql refusal",
	)
	tracker.ObserveSecondary(
		ResourceCore, now.Add(5*time.Minute), RetrySourceRetryAfter, "core refusal",
	)

	snapshot := observedSecondarySnapshot(tracker, now)
	if snapshot.Resource != ResourceGraphQL || snapshot.RetryAt == nil ||
		snapshot.RetryAt.Before(now.Add(14*time.Minute)) {
		t.Fatalf("secondary snapshot = %+v", snapshot)
	}
}
