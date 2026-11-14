package github

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"testing/synctest"
	"time"
)

// @covers AC-INTEGRATIONS-GITHUB-RATE-002.7
func TestWatchProgressInvalidatesChangedConfigurationAndCredentials(t *testing.T) {
	for _, change := range []string{"query", "credential", "scope"} {
		t.Run(change, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				svc, store := setupWatchServiceTest(t)
				t.Cleanup(svc.Stop)
				var calls []string
				configureTestWorkspaceAuth(t, svc, watchProgressClient(&calls), "ws-1")
				watch := &IssueWatch{ID: "watch", WorkspaceID: "ws-1", Repos: []RepoFilter{{Owner: "acme", Name: "first"}, {Owner: "acme", Name: "second"}}, Enabled: true}
				if err := store.CreateIssueWatch(context.Background(), watch); err != nil {
					t.Fatal(err)
				}
				ctx := pollerContext(context.Background())
				if _, err := svc.CheckIssueWatch(ctx, watch); err != nil {
					t.Fatal(err)
				}
				switch change {
				case "query":
					watch.CustomQuery = "is:issue label:changed"
				case "credential":
					svc.resolver.connections.(testConnectionReader).workspaces["ws-1"].CredentialGeneration++
				case "scope":
					settings := defaultWorkspaceSettings("ws-1")
					settings.RepoScopeMode = "repos"
					settings.RepoScopeRepos = watch.Repos
					if err := svc.UpsertWorkspaceSettings(ctx, settings); err != nil {
						t.Fatal(err)
					}
				}
				time.Sleep(5 * time.Minute)
				if _, err := svc.CheckIssueWatch(ctx, watch); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(calls, []string{"first", "first"}) {
					t.Fatalf("stale cursor reused after %s change: %v", change, calls)
				}
			})
		})
	}
}

// @covers AC-INTEGRATIONS-GITHUB-RATE-002.7
func TestWatchProgressManualChecksAndAuthorizationStayIndependent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc, store := setupWatchServiceTest(t)
		t.Cleanup(svc.Stop)
		var calls []string
		configureTestWorkspaceAuth(t, svc, watchProgressClient(&calls), "ws-1")
		watch := &IssueWatch{ID: "watch", WorkspaceID: "ws-1", Repos: []RepoFilter{{Owner: "acme", Name: "first"}, {Owner: "acme", Name: "second"}}, Enabled: true}
		if err := store.CreateIssueWatch(context.Background(), watch); err != nil {
			t.Fatal(err)
		}
		ctx := pollerContext(context.Background())
		if _, err := svc.CheckIssueWatch(ctx, watch); err != nil {
			t.Fatal(err)
		}
		denied := errors.New("workspace access revoked")
		svc.workspaceAuthorizer = func(context.Context, string) error { return denied }
		if _, err := svc.CheckIssueWatch(ctx, watch); !errors.Is(err, denied) {
			t.Fatalf("authorization was bypassed: %v", err)
		}
		if len(calls) != 1 {
			t.Fatal("revoked workspace made a provider request")
		}
		svc.workspaceAuthorizer = nil
		if _, err := svc.CheckIssueWatch(context.Background(), watch); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Minute)
		if _, err := svc.CheckIssueWatch(ctx, watch); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(calls, []string{"first", "first", "second", "second"}) {
			t.Fatalf("manual check reused or changed background progress: %v", calls)
		}
	})
}

// @covers AC-INTEGRATIONS-GITHUB-RATE-002.2
// @covers AC-INTEGRATIONS-GITHUB-RATE-002.3
func TestWatchProgressPreservesInteractiveReserve(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc, store := setupWatchServiceTest(t)
		t.Cleanup(svc.Stop)
		var calls []string
		client := watchProgressClient(&calls)
		configureTestWorkspaceAuth(t, svc, client, "ws-1")
		watch := &IssueWatch{ID: "watch", WorkspaceID: "ws-1", Repos: []RepoFilter{{Owner: "acme", Name: "first"}}, Enabled: true}
		if err := store.CreateIssueWatch(context.Background(), watch); err != nil {
			t.Fatal(err)
		}
		resolved, err := svc.resolveAutomationClient(context.Background(), "ws-1", "", "")
		if err != nil {
			t.Fatal(err)
		}
		resolved.RateTracker.Record(RateSnapshot{Resource: ResourceSearch, Limit: 30, Remaining: 3, RemainingObserved: true, ResetAt: time.Now().Add(time.Hour)})
		if _, err := svc.CheckIssueWatch(pollerContext(context.Background()), watch); err != nil {
			t.Fatal(err)
		}
		if len(calls) != 0 {
			t.Fatal("background polling spent interactive reserve")
		}
		if _, err := svc.CheckIssueWatch(context.Background(), watch); err != nil {
			t.Fatal(err)
		}
		if len(calls) != 1 {
			t.Fatal("interactive check could not spend its reserve")
		}
		resolved.RateTracker.Record(RateSnapshot{Resource: ResourceSearch, Limit: 30, Remaining: 30, RemainingObserved: true, ResetAt: time.Now().Add(time.Hour)})
		if _, err := svc.CheckIssueWatch(pollerContext(context.Background()), watch); err != nil {
			t.Fatal(err)
		}
		if len(calls) != 2 {
			t.Fatal("background polling did not resume after quota recovered")
		}
	})
}

// @covers AC-INTEGRATIONS-GITHUB-RATE-002.7
func TestWatchProgressPrunesDisabledWatches(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		poller, svc, _, store := setupPollerTest(t)
		t.Cleanup(svc.Stop)
		var calls []string
		configureTestWorkspaceAuth(t, svc, watchProgressClient(&calls), "ws1")
		watch := &IssueWatch{ID: "watch", WorkspaceID: "ws1", Repos: []RepoFilter{{Owner: "acme", Name: "first"}, {Owner: "acme", Name: "second"}}, Enabled: true}
		if err := store.CreateIssueWatch(context.Background(), watch); err != nil {
			t.Fatal(err)
		}
		ctx := pollerContext(context.Background())
		poller.checkIssueWatches(ctx)
		watch.Enabled = false
		if err := store.UpdateIssueWatch(ctx, watch); err != nil {
			t.Fatal(err)
		}
		poller.checkIssueWatches(ctx)
		watch.Enabled = true
		if err := store.UpdateIssueWatch(ctx, watch); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Minute)
		poller.checkIssueWatches(ctx)
		if !reflect.DeepEqual(calls, []string{"first", "first"}) {
			t.Fatalf("disabled watch retained stale progress: %v", calls)
		}
	})
}
