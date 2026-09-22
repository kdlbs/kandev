package github

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type passiveFallbackGraphQLClient struct {
	*graphQLMockClient
	mu        sync.Mutex
	findErr   error
	findCalls int
}

func (c *passiveFallbackGraphQLClient) FindPRByBranch(
	context.Context, string, string, string,
) (*PR, error) {
	c.mu.Lock()
	c.findCalls++
	err := c.findErr
	c.mu.Unlock()
	return nil, err
}

func (c *passiveFallbackGraphQLClient) findPRByBranchCallCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.findCalls
}

func TestPassiveFallbackAdmissionBoundsWorkspaceAndGlobalWindow(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	clock := time.Date(2026, time.September, 22, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return clock })
	watches := seedSearchingFallbackWatches(t, store, 7)

	first := svc.selectPassiveFallbackTargets(testWorkspaceID, watches)
	if len(first) != prWatchFallbackWorkspaceBudget {
		t.Fatalf("first workspace fallback targets = %d, want %d", len(first), prWatchFallbackWorkspaceBudget)
	}

	otherWorkspace := make([]*PRWatch, 7)
	for index := range otherWorkspace {
		otherWorkspace[index] = &PRWatch{
			ID:          "other-" + watches[index].ID,
			WorkspaceID: "workspace-other",
			Owner:       "o",
			Repo:        "r",
			Branch:      watches[index].Branch,
		}
	}
	second := svc.selectPassiveFallbackTargets("workspace-other", otherWorkspace)
	if len(second) != prWatchFallbackWorkspaceBudget {
		t.Fatalf("second workspace fallback targets = %d, want %d", len(second), prWatchFallbackWorkspaceBudget)
	}
	if third := svc.selectPassiveFallbackTargets(testWorkspaceID, watches); len(third) != 0 {
		t.Fatalf("same-window workspace fallback targets = %d, want 0", len(third))
	}

	clock = clock.Add(time.Minute)
	rotated := svc.selectPassiveFallbackTargets(testWorkspaceID, watches)
	if len(rotated) != prWatchFallbackWorkspaceBudget {
		t.Fatalf("next-window fallback targets = %d, want %d", len(rotated), prWatchFallbackWorkspaceBudget)
	}
	if rotated[0].ID == first[0].ID {
		t.Fatalf("fallback target rotation restarted at %q", rotated[0].ID)
	}
}

func TestPassiveFallbackStopsAfterFirstRateLimit(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	rateClient := &fallbackProbeClient{
		Client:  NewMockClient(),
		findErr: rateLimitAPIError(429, "secondary limit"),
	}
	configureTestWorkspaceAuth(t, svc, rateClient, testWorkspaceID)
	watches := seedSearchingFallbackWatches(t, store, 7)

	err := svc.refreshWatchesPerWatch(context.Background(), testWorkspaceID, watches)
	if err == nil || !prWatchFallbackShouldStopWorkspace(err) {
		t.Fatalf("fallback error = %v, want a workspace stop", err)
	}
	if got := len(rateClient.fallbackBranches()); got != 1 {
		t.Fatalf("rate-limited passive fallback calls = %d, want one", got)
	}
}

func TestPassiveFallbackSkipsRateLimitedBatch(t *testing.T) {
	_, svc, client, store := setupBatchedPollerTest(t)
	rateErr := rateLimitAPIError(429, "secondary limit")
	client.branchErr = rateErr
	watches := seedSearchingFallbackWatches(t, store, 7)
	staleTasks := make(map[string]struct{}, len(watches))
	for _, watch := range watches {
		staleTasks[watch.TaskID] = struct{}{}
	}

	svc.refreshStaleWorkspaceWatches(testWorkspaceID, staleTasks)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := svc.inflightWorkspaceRefreshes.Load(testWorkspaceID); !ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, ok := svc.inflightWorkspaceRefreshes.Load(testWorkspaceID); ok {
		t.Fatal("workspace refresh did not finish")
	}
	if got := len(client.branchQueries); got != 1 {
		t.Fatalf("batch branch queries = %d, want one", got)
	}
	// The GraphQL error is rate limited, so the passive legacy fallback must
	// not issue a second provider request for any of the seven targets.
	if got := client.FindPRByBranchCallCount(); got != 0 {
		t.Fatalf("per-watch fallback calls = %d, want 0", got)
	}
}

func TestPassiveFallbackOrdinaryErrorContinuesWithinBudget(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ordinary := &fallbackProbeClient{Client: NewMockClient(), findErr: errors.New("temporary transport failure")}
	configureTestWorkspaceAuth(t, svc, ordinary, testWorkspaceID)
	watches := seedSearchingFallbackWatches(t, store, 7)

	if err := svc.refreshWatchesPerWatch(context.Background(), testWorkspaceID, watches); err != nil {
		t.Fatalf("ordinary fallback error = %v, want best-effort completion", err)
	}
	if got := len(ordinary.fallbackBranches()); got != prWatchFallbackWorkspaceBudget {
		t.Fatalf("ordinary passive fallback calls = %d, want %d", got, prWatchFallbackWorkspaceBudget)
	}
}

func TestPassiveWorkspaceFallbackBoundsTargetsAndStopsAfterFirstRateLimit(t *testing.T) {
	_, svc, _, store := setupBatchedPollerTest(t)
	client := &passiveFallbackGraphQLClient{
		graphQLMockClient: &graphQLMockClient{
			MockClient: NewMockClient(),
			branchErr:  errors.New("temporary graphql transport failure"),
		},
		findErr: rateLimitAPIError(429, "secondary limit"),
	}
	configureTestWorkspaceAuth(t, svc, client, testWorkspaceID)
	watches := seedSearchingFallbackWatches(t, store, 7)
	staleTasks := make(map[string]struct{}, len(watches))
	for _, watch := range watches {
		staleTasks[watch.TaskID] = struct{}{}
	}

	svc.refreshStaleWorkspaceWatches(testWorkspaceID, staleTasks)
	waitForPassiveWorkspaceRefresh(t, svc, testWorkspaceID)

	if got := client.findPRByBranchCallCount(); got != 1 {
		t.Fatalf("workspace fallback calls after first rate limit = %d, want 1", got)
	}
	if got := len(client.branchQueries); got != 1 {
		t.Fatalf("workspace batch queries = %d, want 1", got)
	}
	svc.Stop()
}

func TestPassiveWorkspaceFallbackUsesFiveTargetBudget(t *testing.T) {
	_, svc, _, store := setupBatchedPollerTest(t)
	client := &passiveFallbackGraphQLClient{
		graphQLMockClient: &graphQLMockClient{
			MockClient: NewMockClient(),
			branchErr:  errors.New("temporary graphql transport failure"),
		},
		findErr: errors.New("temporary REST transport failure"),
	}
	configureTestWorkspaceAuth(t, svc, client, testWorkspaceID)
	watches := seedSearchingFallbackWatches(t, store, 7)
	staleTasks := make(map[string]struct{}, len(watches))
	for _, watch := range watches {
		staleTasks[watch.TaskID] = struct{}{}
	}

	svc.refreshStaleWorkspaceWatches(testWorkspaceID, staleTasks)
	waitForPassiveWorkspaceRefresh(t, svc, testWorkspaceID)

	if got := client.findPRByBranchCallCount(); got != prWatchFallbackWorkspaceBudget {
		t.Fatalf("workspace fallback calls = %d, want %d", got, prWatchFallbackWorkspaceBudget)
	}
	svc.Stop()
}

func waitForPassiveWorkspaceRefresh(t *testing.T, svc *Service, workspaceID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := svc.inflightWorkspaceRefreshes.Load(workspaceID); !ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("workspace refresh did not finish")
}
