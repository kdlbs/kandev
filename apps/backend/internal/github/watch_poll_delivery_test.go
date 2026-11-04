package github

import (
	"context"
	"reflect"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// @covers AC-INTEGRATIONS-GITHUB-RATE-002.7
func TestWatchPollingPublishesPartialDiscoveriesWithoutDuplicateReservations(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		poller, svc, _, store := setupPollerTest(t)
		t.Cleanup(svc.Stop)
		var calls, created []string
		configureTestWorkspaceAuth(t, svc, watchProgressClient(&calls), "ws1")
		watch := &IssueWatch{ID: "watch", WorkspaceID: "ws1", Repos: []RepoFilter{{Owner: "acme", Name: "first"}, {Owner: "acme", Name: "second"}}, Enabled: true}
		if err := store.CreateIssueWatch(context.Background(), watch); err != nil {
			t.Fatal(err)
		}
		subscription, err := poller.eventBus.Subscribe(events.GitHubNewIssue, func(ctx context.Context, event *bus.Event) error {
			data := event.Data.(*NewIssueEvent)
			reserved, err := svc.ReserveIssueWatchTask(ctx, data.IssueWatchID, data.Issue.RepoOwner, data.Issue.RepoName, data.Issue.Number, data.Issue.HTMLURL)
			if err != nil {
				return err
			}
			if reserved {
				created = append(created, data.Issue.RepoName)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = subscription.Unsubscribe() })
		for cycle := 0; cycle < 4; cycle++ {
			poller.checkWatchQueues(pollerContext(context.Background()))
			if cycle == 0 && !reflect.DeepEqual(created, []string{"first"}) {
				t.Fatalf("partial discovery was lost: %v", created)
			}
			time.Sleep(5 * time.Minute)
		}
		if !reflect.DeepEqual(created, []string{"first", "second"}) {
			t.Fatalf("discovery reservations=%v", created)
		}
	})
}

// @covers AC-INTEGRATIONS-GITHUB-RATE-002.7
func TestWatchProgressStopStartDiscardsCursor(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		poller, svc, _, store := setupPollerTest(t)
		t.Cleanup(svc.Stop)
		var calls []string
		configureTestWorkspaceAuth(t, svc, watchProgressClient(&calls), "ws1")
		watch := &IssueWatch{ID: "watch", WorkspaceID: "ws1", Repos: []RepoFilter{{Owner: "acme", Name: "first"}, {Owner: "acme", Name: "second"}}, Enabled: true}
		if err := store.CreateIssueWatch(context.Background(), watch); err != nil {
			t.Fatal(err)
		}
		poller.Start(context.Background())
		t.Cleanup(poller.Stop)
		synctest.Wait()
		poller.Stop()
		time.Sleep(5 * time.Minute)
		poller.Start(context.Background())
		synctest.Wait()
		poller.Stop()
		if !reflect.DeepEqual(calls, []string{"first", "first"}) {
			t.Fatalf("restart reused stale cursor: %v", calls)
		}
	})
}

type watchPrincipalProvider map[string]Client

func (p watchPrincipalProvider) ResolveAutomation(_ context.Context, c *WorkspaceConnection, _ ResolveCredentialRequest) (*ResolvedCredential, error) {
	return &ResolvedCredential{Client: p[c.WorkspaceID], Principal: AuthPrincipal{Kind: AuthPrincipalHuman, Login: c.Login, Source: ConnectionSourcePAT}, Capabilities: allTokenCapabilities()}, nil
}

// @covers AC-INTEGRATIONS-GITHUB-RATE-002.6
// @covers AC-INTEGRATIONS-GITHUB-RATE-002.7
func TestWatchPollingDoesNotGateAnotherPrincipal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		poller, svc, _, store := setupPollerTest(t)
		t.Cleanup(svc.Stop)
		var aliceCalls, bobCalls []string
		clients := watchPrincipalProvider{"ws1": watchProgressClient(&aliceCalls), "ws-1": watchProgressClient(&bobCalls)}
		configureTestWorkspaceAuth(t, svc, clients["ws1"], "ws1", "ws-1")
		reader := svc.resolver.connections.(testConnectionReader)
		reader.workspaces["ws1"].Login = "alice"
		reader.workspaces["ws-1"].Login = "bob"
		svc.resolver.SetAutomationProvider(clients)
		alice, err := svc.resolveAutomationClient(context.Background(), "ws1", "", "")
		if err != nil {
			t.Fatal(err)
		}
		alice.RateTracker.ObserveSecondary(ResourceSearch, time.Now().Add(time.Hour), RetrySourceRetryAfter, "")
		for _, workspace := range []string{"ws1", "ws-1"} {
			if err := store.CreateIssueWatch(context.Background(), &IssueWatch{ID: workspace, WorkspaceID: workspace, Repos: []RepoFilter{{Owner: "acme", Name: workspace}}, Enabled: true}); err != nil {
				t.Fatal(err)
			}
		}
		poller.checkWatchQueues(pollerContext(context.Background()))
		if len(aliceCalls) != 0 || !reflect.DeepEqual(bobCalls, []string{"ws-1"}) {
			t.Fatalf("provider calls: alice=%v bob=%v", aliceCalls, bobCalls)
		}
	})
}
