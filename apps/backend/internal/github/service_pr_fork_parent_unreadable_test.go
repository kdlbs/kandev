package github

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// parentErrorClient answers FindPRByHead on one parent repository with a fixed
// error, the way GitHub answers a private parent the credential cannot read.
type parentErrorClient struct {
	*graphQLMockClient
	parentOwner, parentRepo string
	err                     atomic.Pointer[error]
	parentCalls             atomic.Int32
}

func (c *parentErrorClient) setParentErr(err error) { c.err.Store(&err) }

func (c *parentErrorClient) parentErr() error {
	if err := c.err.Load(); err != nil {
		return *err
	}
	return nil
}

func (c *parentErrorClient) FindPRByHead(
	ctx context.Context, owner, repo, headOwner, headRepo, branch string,
) (*PR, error) {
	if strings.EqualFold(owner, c.parentOwner) && strings.EqualFold(repo, c.parentRepo) {
		c.parentCalls.Add(1)
		if err := c.parentErr(); err != nil {
			return nil, err
		}
	}
	return c.graphQLMockClient.FindPRByHead(ctx, owner, repo, headOwner, headRepo, branch)
}

func parentNotFoundError() error {
	return &GitHubAPIError{StatusCode: 404, Endpoint: "/repos/" + parentOwner + "/" + parentRepo + "/pulls"}
}

func setupParentErrorWatch(t *testing.T, parentErr error) (*Service, *MockClient, *parentErrorClient, *PRWatch) {
	t.Helper()
	_, service, mock, store := setupPollerTest(t)
	seedForkNetwork(mock)
	client := &parentErrorClient{graphQLMockClient: &graphQLMockClient{MockClient: mock}, parentOwner: parentOwner, parentRepo: parentRepo}
	client.setParentErr(parentErr)
	service.client = client
	configureTestWorkspaceAuth(t, service, client, testWorkspaceID, "ws-1", "ws1")

	seedTask(t, store, "task-fork-parent", false)
	watch := withTestWorkspace(&PRWatch{
		SessionID: "session-fork-parent", TaskID: "task-fork-parent",
		Owner: forkOwner, Repo: forkRepo, Branch: forkPRBranch,
	})
	if err := store.CreatePRWatch(context.Background(), watch); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}
	return service, mock, client, watch
}

// AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.6
func TestFindPRForWatch_UnreadableForkParentIsHealthyEmpty(t *testing.T) {
	notFoundErrors := map[string]error{
		"rest 404":   parentNotFoundError(),
		"gh cli 404": errors.New(`find PR by branch "x": gh: Not Found (HTTP 404)`),
		"gh cli GraphQL missing repository": errors.New(
			`find PR by branch "x": gh pr: exit status 1: GraphQL: Could not resolve to a Repository with the name 'kdlbs/kandev'. (repository)`,
		),
	}
	for name, parentErr := range notFoundErrors {
		t.Run(name, func(t *testing.T) {
			service, mock, client, watch := setupParentErrorWatch(t, parentErr)
			ctx := context.Background()
			scope := testAutomationScope(t, service, testWorkspaceID)

			pr, err := service.findPRByBranchForWatch(ctx, watch)
			if err != nil || pr != nil {
				t.Fatalf("lookup = (%v, %v), want no PR and no error", pr, err)
			}
			if health := service.prDiscoveryHealth.snapshot(testWorkspaceID, scope, 0); health.State != PRDiscoveryHealthHealthy {
				t.Fatalf("health = %+v, want healthy", health)
			}
			if got := client.parentCalls.Load(); got != 1 {
				t.Fatalf("parent lookups = %d, want 1", got)
			}

			// The unreadable parent is remembered: the next lookup skips it.
			if _, err := service.findPRByBranchForWatch(ctx, watch); err != nil {
				t.Fatalf("second lookup: %v", err)
			}
			if got := client.parentCalls.Load(); got != 1 {
				t.Fatalf("parent lookups after second attempt = %d, want 1 (cached)", got)
			}

			// A PR later opened in the fork itself is still discovered.
			mock.AddPR(&PR{
				Number: 7, State: "open", HeadBranch: forkPRBranch,
				HeadRepoOwner: forkOwner, HeadRepoName: forkRepo,
				RepoOwner: forkOwner, RepoName: forkRepo,
			})
			pr, err = service.findPRByBranchForWatch(ctx, watch)
			if err != nil || pr == nil || pr.Number != 7 {
				t.Fatalf("lookup after fork PR = (%v, %v), want PR #7", pr, err)
			}
		})
	}
}

// AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.6: once the cached unreadable-parent
// entry expires, the next lookup searches a parent that became readable.
func TestFindPRForWatch_ForkParentReadableAgainAfterCacheExpiry(t *testing.T) {
	service, mock, client, watch := setupParentErrorWatch(t, parentNotFoundError())
	ctx := context.Background()
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	service.forkParentCache.now = func() time.Time { return now }

	if pr, err := service.findPRByBranchForWatch(ctx, watch); err != nil || pr != nil {
		t.Fatalf("lookup with unreadable parent = (%v, %v), want no PR and no error", pr, err)
	}

	// Access is granted and the parent already has the fork's PR open.
	client.setParentErr(nil)
	mock.AddPR(forkNetworkPR())
	if pr, err := service.findPRByBranchForWatch(ctx, watch); err != nil || pr != nil {
		t.Fatalf("lookup inside cache lifetime = (%v, %v), want no PR (parent still cached)", pr, err)
	}

	now = now.Add(newForkParentCache().ttl + time.Second)
	pr, err := service.findPRByBranchForWatch(ctx, watch)
	if err != nil || pr == nil || pr.Number != forkPRNumber {
		t.Fatalf("lookup after cache expiry = (%v, %v), want parent PR #%d", pr, err, forkPRNumber)
	}
	if got := client.parentCalls.Load(); got != 2 {
		t.Fatalf("parent lookups = %d, want 2 (initial 404 and post-expiry search)", got)
	}
}

// AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.6: only not-found is absorbed.
func TestFindPRForWatch_ForkParentOtherErrorsKeepFailure(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		category PRDiscoveryHealthCategory
	}{
		{"server error", &GitHubAPIError{StatusCode: 502, Endpoint: "/pulls"}, PRDiscoveryHealthUnavailable},
		{"invalid credentials", &GitHubAPIError{StatusCode: 401, Endpoint: "/pulls"}, PRDiscoveryHealthUnavailable},
		{
			"forbidden with missing-repository text",
			&GitHubAPIError{StatusCode: 403, Endpoint: "/pulls", Body: "Could not resolve to a Repository"},
			PRDiscoveryHealthUnavailable,
		},
		{
			"integration permission error",
			errors.New("GraphQL: Resource not accessible by integration (repository.pullRequests)"),
			PRDiscoveryHealthUnavailable,
		},
		{"rate limit", &GitHubAPIError{StatusCode: 429, Endpoint: "/pulls"}, PRDiscoveryHealthRateLimited},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, _, _, watch := setupParentErrorWatch(t, tc.err)
			scope := testAutomationScope(t, service, testWorkspaceID)

			if _, err := service.findPRByBranchForWatch(context.Background(), watch); !errors.Is(err, tc.err) {
				t.Fatalf("lookup error = %v, want %v", err, tc.err)
			}
			health := service.prDiscoveryHealth.snapshot(testWorkspaceID, scope, 0)
			if health.State != PRDiscoveryHealthDegraded || health.Category != tc.category {
				t.Fatalf("health = %+v, want degraded %s", health, tc.category)
			}
		})
	}
}

// AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.6 on the batched path, where a
// definitive-empty fork alias goes straight to the parent lookup.
func TestSyncWatchesBatched_UnreadableForkParentIsHealthyEmpty(t *testing.T) {
	_, service, mock, store := setupPollerTest(t)
	ctx := context.Background()
	graphQL := &graphQLMockClient{MockClient: mock}
	client := &parentErrorClient{graphQLMockClient: graphQL, parentOwner: "upstream", parentRepo: "r"}
	client.setParentErr(parentNotFoundError())
	service.client = client
	configureTestWorkspaceAuth(t, service, client, testWorkspaceID, "ws-1", "ws1")
	scope := testAutomationScope(t, service, testWorkspaceID)

	watches := seedSearchingWatches(t, store, "fork", "r", 1)
	mock.SetRepositoryDetails(GitHubRepository{
		FullName: "fork/r", Owner: "fork", Name: "r", Fork: true, ParentFullName: "upstream/r",
	})
	graphQL.branchResponses = []string{branchAliasResponse(t, 1, nil), branchAliasResponse(t, 1, nil)}

	if _, err := service.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, watches); err != nil {
		t.Fatalf("first batched sync: %v", err)
	}
	if updated := reloadWatches(t, store, watches); updated[0].PRNumber != 0 {
		t.Fatalf("watch = PR #%d, want to stay searching", updated[0].PRNumber)
	}
	if health := service.prDiscoveryHealth.snapshot(testWorkspaceID, scope, 0); health.State != PRDiscoveryHealthHealthy {
		t.Fatalf("health = %+v, want healthy", health)
	}

	stale := time.Now().UTC().Add(-2 * PRSyncFreshnessWindow)
	if err := store.UpdatePRWatchTimestamps(ctx, watches[0].ID, stale, nil, "", ""); err != nil {
		t.Fatalf("age watch: %v", err)
	}
	if _, err := service.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, reloadWatches(t, store, watches)); err != nil {
		t.Fatalf("second batched sync: %v", err)
	}
	if got := client.parentCalls.Load(); got != 1 {
		t.Fatalf("parent lookups = %d, want 1 across 2 cycles (cached)", got)
	}
}
