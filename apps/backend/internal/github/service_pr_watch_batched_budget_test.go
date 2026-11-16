package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// These tests pin the API budget of the batched watch sync, not just its
// output. The regression they guard: the batched branch query answers "no open
// PR on this branch" definitively, the apply path used to discard that answer,
// and every searching watch then re-asked it through a separate `gh pr list`
// (GraphQL) plus a `gh api /repos/...` (fork check) on EVERY sync cycle. With
// 42 searching watches that burned ~150 GraphQL points/minute against a
// 5000/hour budget and parked the poller on `github rate-limited`.
//
// Assertions here are therefore call counts. A test that only checks the
// resulting PRStatus passes with the bug fully restored.

// branchAliasResponse builds a `query Branches` response where every supplied
// alias index resolves with zero open PRs, except those named in overrides
// (raw JSON for that alias, or omitted entirely when the value is "").
func branchAliasResponse(t *testing.T, count int, overrides map[int]string) string {
	t.Helper()
	aliases := make([]string, 0, count)
	for i := range count {
		if raw, ok := overrides[i]; ok {
			if raw == "" {
				continue // alias absent from the response entirely
			}
			aliases = append(aliases, fmt.Sprintf("%q: %s", fmt.Sprintf("b%d", i), raw))
			continue
		}
		aliases = append(aliases, fmt.Sprintf("%q: {\"pullRequests\": {\"nodes\": []}}", fmt.Sprintf("b%d", i)))
	}
	resp := "{\"data\": {" + strings.Join(aliases, ", ") + "}}"
	if !json.Valid([]byte(resp)) {
		t.Fatalf("built invalid branch response: %s", resp)
	}
	return resp
}

func openPRNode(number int, branch, author string) string {
	return fmt.Sprintf(`{"pullRequests": {"nodes": [{
		"number": %d,
		"state": "OPEN", "title": "PR %d", "url": "https://x/%d",
		"isDraft": false, "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
		"headRefName": %q, "baseRefName": "main", "headRefOid": "abc",
		"author": {"login": %q},
		"createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-02T00:00:00Z",
		"reviews": {"nodes": []}, "reviewRequests": {"totalCount": 0},
		"commits": {"nodes": []}
	}]}}`, number, number, number, branch, author)
}

type forkParentProbeClient struct {
	Client
	graphQL            GraphQLExecutor
	mu                 sync.Mutex
	deferredBranch     string
	deferralsRemaining int
	deferredErr        error
	deferredBranches   map[string]*forkParentProbeDeferral
	probeCalls         int
	onProbe            func()
}

type forkParentProbeDeferral struct {
	remaining int
	err       error
}

func (c *forkParentProbeClient) ExecuteGraphQL(
	ctx context.Context, query string, vars map[string]any, out any,
) error {
	if c.graphQL == nil {
		return errGraphQLUnsupported
	}
	return c.graphQL.ExecuteGraphQL(ctx, query, vars, out)
}

func (c *forkParentProbeClient) GetRepository(
	ctx context.Context, owner, repo string,
) (*GitHubRepository, error) {
	reader, ok := c.Client.(repositoryDetailsReader)
	if !ok {
		return nil, nil
	}
	return reader.GetRepository(ctx, owner, repo)
}

func (c *forkParentProbeClient) FindPRByHead(
	ctx context.Context, owner, repo, headOwner, headRepo, branch string,
) (*PR, error) {
	if branch != c.deferredBranch {
		c.mu.Lock()
		deferred, matched := c.deferredBranches[branch]
		if !matched {
			c.mu.Unlock()
			return c.findPRByHead(ctx, owner, repo, headOwner, headRepo, branch)
		}
		c.probeCalls++
		deferProbe := deferred.remaining > 0
		if deferProbe {
			deferred.remaining--
		}
		probeErr := deferred.err
		onProbe := c.onProbe
		c.mu.Unlock()
		if onProbe != nil {
			onProbe()
		}
		if deferProbe {
			return nil, probeErr
		}
		return c.findPRByHead(ctx, owner, repo, headOwner, headRepo, branch)
	}
	c.mu.Lock()
	c.probeCalls++
	deferProbe := c.deferralsRemaining > 0
	if deferProbe {
		c.deferralsRemaining--
	}
	probeErr := c.deferredErr
	onProbe := c.onProbe
	c.mu.Unlock()
	if onProbe != nil {
		onProbe()
	}
	if deferProbe {
		return nil, probeErr
	}
	return c.findPRByHead(ctx, owner, repo, headOwner, headRepo, branch)
}

func (c *forkParentProbeClient) findPRByHead(
	ctx context.Context, owner, repo, headOwner, headRepo, branch string,
) (*PR, error) {
	finder, ok := c.Client.(exactHeadPRFinder)
	if !ok {
		return nil, nil
	}
	return finder.FindPRByHead(ctx, owner, repo, headOwner, headRepo, branch)
}

func (c *forkParentProbeClient) forkParentProbeCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.probeCalls
}

func equalOptionalTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Equal(*right)
}

func TestSyncWatchesBatched_ForkParentDeferralPreservesHealth(t *testing.T) {
	for _, healthState := range []string{"healthy", "degraded"} {
		for _, deferral := range []struct {
			name   string
			reason string
		}{
			{name: "background pacing", reason: rateLimitBlockBackgroundPacing},
			{name: "interactive priority", reason: rateLimitBlockInteractiveWaiting},
		} {
			t.Run(healthState+"/"+deferral.name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					poller, service, baseClient, store := setupBatchedPollerTest(t)
					_ = poller
					ctx := context.Background()
					fixedNow := time.Now().UTC()
					service.prDiscoveryHealth.setNow(func() time.Time { return fixedNow })
					scope := testAutomationScope(t, service, testWorkspaceID)
					branch := "feature/deferred-parent"
					target := prDiscoveryHealthTarget{Owner: "fork", Repo: "repo", Branch: branch}
					oldCheckedAt := time.Now().UTC().Add(-time.Hour)
					makeWatch := func(sessionID, taskID, watchBranch string) *PRWatch {
						seedTask(t, store, taskID, false)
						watch := withTestWorkspace(&PRWatch{
							SessionID: sessionID, TaskID: taskID, Owner: "fork", Repo: "repo",
							Branch: watchBranch, LastCheckedAt: &oldCheckedAt,
						})
						if err := store.CreatePRWatch(ctx, watch); err != nil {
							t.Fatalf("create watch %s: %v", sessionID, err)
						}
						if err := store.UpdatePRWatchTimestamps(ctx, watch.ID, oldCheckedAt, nil, "", ""); err != nil {
							t.Fatalf("stamp watch %s: %v", sessionID, err)
						}
						return watch
					}
					first := makeWatch("session-deferred-1", "task-deferred-1", branch)
					second := makeWatch("session-deferred-2", "task-deferred-2", branch)
					joined := makeWatch("session-deferred-joined", "task-deferred-joined", branch)
					sibling := makeWatch("session-completed-sibling", "task-completed-sibling", "feature/sibling")
					for _, watch := range []*PRWatch{first, second, joined, sibling} {
						service.trackPRDiscoveryWatchConsumer(testWorkspaceID, scope, 1, watch)
					}
					attempt, admitted := service.prDiscoveryHealth.begin(testWorkspaceID, scope, 1, target)
					if !admitted {
						t.Fatal("initial health attempt was not admitted")
					}
					if healthState == "healthy" {
						service.prDiscoveryHealth.finishSuccess(testWorkspaceID, scope, 1, target, attempt)
					} else {
						service.prDiscoveryHealth.finishFailure(
							testWorkspaceID, scope, 1, target, attempt, PRDiscoveryHealthUnavailable, nil,
						)
					}
					fixedNow = fixedNow.Add(2 * PRDiscoveryRetryBase)
					priorHealth := service.prDiscoveryHealth.snapshot(testWorkspaceID, scope, 1)

					started := make(chan struct{})
					release := make(chan struct{})
					var onProbeOnce sync.Once
					wrappedDeferral := fmt.Errorf("fork parent admission: %w", &AdmissionDeferredError{
						Delay: time.Second, Reason: deferral.reason,
					})
					client := &forkParentProbeClient{
						Client: baseClient, graphQL: baseClient, deferredBranch: branch,
						deferralsRemaining: 1, deferredErr: wrappedDeferral,
						onProbe: func() {
							onProbeOnce.Do(func() {
								close(started)
								<-release
							})
						},
					}
					configureTestWorkspaceAuth(t, service, client, testWorkspaceID)
					baseClient.SetRepositoryDetails(GitHubRepository{
						FullName: "fork/repo", Owner: "fork", Name: "repo", Fork: true,
						ParentFullName: "upstream/repo",
					})
					baseClient.branchResponses = []string{branchAliasResponse(t, 2, nil)}

					leaderDone := make(chan prWatchSyncOutcome, 1)
					go func() {
						leaderDone <- service.syncWorkspaceWatchesBatchedForPoller(
							pollerContext(ctx), testWorkspaceID, []*PRWatch{first, second, sibling},
						)
					}()
					<-started

					joinedDone := make(chan prWatchSyncOutcome, 1)
					go func() {
						joinedDone <- service.syncWorkspaceWatchesBatchedForPoller(
							pollerContext(ctx), testWorkspaceID, []*PRWatch{joined},
						)
					}()
					synctest.Wait()
					close(release)
					leader := <-leaderDone
					joinedOutcome := <-joinedDone

					if leader.Deferred == nil || !errors.Is(leader.Err, ErrBackgroundAdmissionDeferred) {
						t.Fatalf("leader outcome = %+v, want the wrapped local deferral", leader)
					}
					if joinedOutcome.Deferred == nil || !errors.Is(joinedOutcome.Err, ErrBackgroundAdmissionDeferred) {
						t.Fatalf("joined outcome = %+v, want the same local deferral", joinedOutcome)
					}
					if got := prWatchStatusPendingWatches(leader.Pending); len(got) != 2 ||
						got[0].SessionID != first.SessionID || got[1].SessionID != second.SessionID {
						t.Fatalf("leader pending watches = %+v, want both target consumers", got)
					}
					if got := prWatchStatusPendingWatches(joinedOutcome.Pending); len(got) != 1 || got[0].SessionID != joined.SessionID {
						t.Fatalf("joined pending watches = %+v, want the joining consumer", got)
					}
					if got := client.forkParentProbeCalls(); got != 1 {
						t.Fatalf("fork-parent probes = %d, want one shared probe", got)
					}
					completedSibling := false
					for _, result := range leader.Results {
						if result.Watch != nil && result.Watch.SessionID == sibling.SessionID {
							completedSibling = result.DiscoveryResolved && result.DiscoveryTrackable
						}
					}
					if !completedSibling {
						t.Fatal("completed sibling discovery was not retained in the leader results")
					}
					for _, watch := range []*PRWatch{first, second, joined} {
						updated, err := store.GetPRWatchBySession(ctx, watch.SessionID)
						if err != nil {
							t.Fatalf("reload deferred watch %s: %v", watch.SessionID, err)
						}
						if updated.LastCheckedAt == nil || !updated.LastCheckedAt.Equal(oldCheckedAt) {
							t.Errorf("watch %s last_checked_at = %v, want unchanged %v", watch.SessionID, updated.LastCheckedAt, oldCheckedAt)
						}
					}
					updatedSibling, err := store.GetPRWatchBySession(ctx, sibling.SessionID)
					if err != nil {
						t.Fatalf("reload completed sibling: %v", err)
					}
					if updatedSibling.LastCheckedAt == nil {
						t.Fatal("completed sibling was not stamped")
					}
					afterHealth := service.prDiscoveryHealth.snapshot(testWorkspaceID, scope, 1)
					if afterHealth.State != priorHealth.State || afterHealth.FailedTargetCount != priorHealth.FailedTargetCount ||
						afterHealth.Category != priorHealth.Category || afterHealth.Revision != priorHealth.Revision ||
						!equalOptionalTime(afterHealth.LastFailureAt, priorHealth.LastFailureAt) ||
						!equalOptionalTime(afterHealth.RetryAt, priorHealth.RetryAt) {
						t.Fatalf("health changed after local deferral: before=%+v after=%+v", priorHealth, afterHealth)
					}
				})
			})
		}
	}
}

func TestSyncWatchesBatched_ForkParentProviderErrorRemainsFailure(t *testing.T) {
	_, service, baseClient, store := setupBatchedPollerTest(t)
	ctx := context.Background()
	branch := "feature/provider-error"
	lastCheckedAt := time.Now().UTC().Add(-time.Hour)
	seedTask(t, store, "task-provider-error", false)
	watch := withTestWorkspace(&PRWatch{
		SessionID: "session-provider-error", TaskID: "task-provider-error",
		Owner: "fork", Repo: "repo", Branch: branch, LastCheckedAt: &lastCheckedAt,
	})
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create watch: %v", err)
	}
	if err := store.UpdatePRWatchTimestamps(ctx, watch.ID, lastCheckedAt, nil, "", ""); err != nil {
		t.Fatalf("stamp watch: %v", err)
	}
	fixedNow := time.Now().UTC()
	service.prDiscoveryHealth.setNow(func() time.Time { return fixedNow })
	scope := testAutomationScope(t, service, testWorkspaceID)
	service.trackPRDiscoveryWatchConsumer(testWorkspaceID, scope, 1, watch)
	attempt, admitted := service.prDiscoveryHealth.begin(
		testWorkspaceID, scope, 1, prDiscoveryTargetForWatch(watch),
	)
	if !admitted {
		t.Fatal("initial health attempt was not admitted")
	}
	service.prDiscoveryHealth.finishSuccess(testWorkspaceID, scope, 1, prDiscoveryTargetForWatch(watch), attempt)
	fixedNow = fixedNow.Add(2 * PRDiscoveryRetryBase)

	providerErr := errors.New("provider request failed")
	client := &forkParentProbeClient{
		Client: baseClient, graphQL: baseClient, deferredBranch: branch,
		deferralsRemaining: 1, deferredErr: providerErr,
	}
	configureTestWorkspaceAuth(t, service, client, testWorkspaceID)
	baseClient.SetRepositoryDetails(GitHubRepository{
		FullName: "fork/repo", Owner: "fork", Name: "repo", Fork: true,
		ParentFullName: "upstream/repo",
	})
	baseClient.branchResponses = []string{branchAliasResponse(t, 1, nil)}

	outcome := service.syncWorkspaceWatchesBatchedForPoller(
		pollerContext(ctx), testWorkspaceID, []*PRWatch{watch},
	)
	if outcome.Deferred != nil || outcome.Err != nil || len(outcome.Pending) != 0 {
		t.Fatalf("provider error outcome = %+v, want completed failure handling without local deferral", outcome)
	}
	health := service.prDiscoveryHealth.snapshot(testWorkspaceID, scope, 1)
	if health.State != PRDiscoveryHealthDegraded || health.Category != PRDiscoveryHealthUnavailable || health.FailedTargetCount != 1 {
		t.Fatalf("health after provider error = %+v, want unavailable failure", health)
	}
	updated, err := store.GetPRWatchBySession(ctx, watch.SessionID)
	if err != nil {
		t.Fatalf("reload watch: %v", err)
	}
	if updated.LastCheckedAt == nil || updated.LastCheckedAt.Equal(lastCheckedAt) {
		t.Fatalf("last_checked_at = %v, want provider-error stamp after %v", updated.LastCheckedAt, lastCheckedAt)
	}
}

// seedSearchingWatches creates n searching (pr_number=0) watches on one repo,
// returning them in the same order the batched query aliases them.
func seedSearchingWatches(t *testing.T, store *Store, owner, repo string, n int) []*PRWatch {
	t.Helper()
	ctx := context.Background()
	watches := make([]*PRWatch, 0, n)
	for i := range n {
		taskID := fmt.Sprintf("task-%d", i)
		seedTask(t, store, taskID, false)
		w := withTestWorkspace(&PRWatch{
			SessionID: fmt.Sprintf("session-%d", i),
			TaskID:    taskID,
			Owner:     owner,
			Repo:      repo,
			PRNumber:  0,
			Branch:    fmt.Sprintf("feature/branch-%d", i),
		})
		if err := store.CreatePRWatch(ctx, w); err != nil {
			t.Fatalf("create PR watch %d: %v", i, err)
		}
		watches = append(watches, w)
	}
	return watches
}

// reloadWatches re-reads the watches from the store so a second sync cycle
// sees the last_checked_at the first one stamped, exactly as the poller does.
func reloadWatches(t *testing.T, store *Store, watches []*PRWatch) []*PRWatch {
	t.Helper()
	ctx := context.Background()
	out := make([]*PRWatch, 0, len(watches))
	for _, w := range watches {
		fresh, err := store.GetPRWatchBySession(ctx, w.SessionID)
		if err != nil {
			t.Fatalf("reload watch %s: %v", w.SessionID, err)
		}
		out = append(out, fresh)
	}
	return out
}

func TestSyncWatchesBatched_ResolvedEmptyBranchSkipsPerWatchLookup(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()

	const watchCount = 4
	watches := seedSearchingWatches(t, store, "o", "r", watchCount)
	gh.SetRepositoryDetails(GitHubRepository{FullName: "o/r", Owner: "o", Name: "r", Fork: false})
	gh.branchResponses = []string{
		branchAliasResponse(t, watchCount, nil),
		branchAliasResponse(t, watchCount, nil),
	}

	if _, err := svc.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, watches); err != nil {
		t.Fatalf("first batched sync: %v", err)
	}
	if _, err := svc.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, reloadWatches(t, store, watches)); err != nil {
		t.Fatalf("second batched sync: %v", err)
	}

	if got := len(gh.branchQueries); got != 2 {
		t.Errorf("batched branch queries = %d, want 2 (one per cycle)", got)
	}
	// The batched query already said "no open PR" for every branch. Re-asking
	// per watch is what blew the GraphQL budget.
	if got := gh.FindPRByBranchCallCount(); got != 0 {
		t.Errorf("per-watch branch lookups = %d, want 0 across %d watches x 2 cycles", got, watchCount)
	}
	// Fork status is constant, so it is resolved once and cached — not once
	// per watch per cycle.
	if got := gh.GetRepositoryCallCount(); got != 1 {
		t.Errorf("repository fork lookups = %d, want 1 across %d watches x 2 cycles", got, watchCount)
	}
	for _, w := range reloadWatches(t, store, watches) {
		if w.LastCheckedAt == nil {
			t.Errorf("watch %s was not stamped despite a definitive answer", w.SessionID)
		}
		if w.PRNumber != 0 {
			t.Errorf("watch %s = PR #%d, want to stay searching", w.SessionID, w.PRNumber)
		}
	}
}

func TestSyncWatchesBatched_UnresolvedAliasStillFallsBack(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()

	const watchCount = 3
	watches := seedSearchingWatches(t, store, "o", "r", watchCount)
	gh.SetRepositoryDetails(GitHubRepository{FullName: "o/r", Owner: "o", Name: "r", Fork: false})
	// Alias b1 is absent from the response: not a definitive negative.
	gh.branchResponses = []string{branchAliasResponse(t, watchCount, map[int]string{1: ""})}

	if _, err := svc.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, watches); err != nil {
		t.Fatalf("batched sync: %v", err)
	}

	if got := gh.FindPRByBranchCallCount(); got != 1 {
		t.Errorf("per-watch branch lookups = %d, want 1 (only the unresolved alias)", got)
	}
}

func TestSyncWatchesBatched_AmbiguousBranchStillFallsBack(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()

	watches := seedSearchingWatches(t, store, "o", "r", 2)
	gh.SetRepositoryDetails(GitHubRepository{FullName: "o/r", Owner: "o", Name: "r", Fork: false})
	// b0 carries two open PRs — the decoder refuses to pick one, so the
	// caller must not read that as "there is no PR".
	twoOpen := `{"pullRequests": {"nodes": [
		{"number": 7, "state": "OPEN", "title": "A", "url": "https://x/7",
		 "isDraft": false, "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
		 "headRefName": "feature/branch-0", "baseRefName": "main", "headRefOid": "a",
		 "author": {"login": "alice"},
		 "createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-01T00:00:00Z",
		 "reviews": {"nodes": []}, "reviewRequests": {"totalCount": 0}, "commits": {"nodes": []}},
		{"number": 8, "state": "OPEN", "title": "B", "url": "https://x/8",
		 "isDraft": false, "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
		 "headRefName": "feature/branch-0", "baseRefName": "main", "headRefOid": "b",
		 "author": {"login": "bob"},
		 "createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-01T00:00:00Z",
		 "reviews": {"nodes": []}, "reviewRequests": {"totalCount": 0}, "commits": {"nodes": []}}
	]}}`
	gh.branchResponses = []string{branchAliasResponse(t, 2, map[int]string{0: twoOpen})}

	if _, err := svc.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, watches); err != nil {
		t.Fatalf("batched sync: %v", err)
	}

	if got := gh.FindPRByBranchCallCount(); got != 1 {
		t.Errorf("per-watch branch lookups = %d, want 1 (only the ambiguous branch)", got)
	}
}

func TestSyncWatchesBatched_FallbackThrottledInsideFreshnessWindow(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()

	watches := seedSearchingWatches(t, store, "o", "r", 1)
	gh.SetRepositoryDetails(GitHubRepository{FullName: "o/r", Owner: "o", Name: "r", Fork: false})
	// The alias never resolves, so every cycle wants the fallback.
	gh.branchResponses = []string{
		branchAliasResponse(t, 1, map[int]string{0: ""}),
		branchAliasResponse(t, 1, map[int]string{0: ""}),
	}

	if _, err := svc.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, watches); err != nil {
		t.Fatalf("first batched sync: %v", err)
	}
	if _, err := svc.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, reloadWatches(t, store, watches)); err != nil {
		t.Fatalf("second batched sync: %v", err)
	}
	if got := gh.FindPRByBranchCallCount(); got != 1 {
		t.Fatalf("per-watch branch lookups = %d, want 1 (second cycle throttled)", got)
	}

	// Age the watch past the freshness window and the fallback runs again.
	stale := time.Now().UTC().Add(-2 * PRSyncFreshnessWindow)
	if err := store.UpdatePRWatchTimestamps(ctx, watches[0].ID, stale, nil, "", ""); err != nil {
		t.Fatalf("age watch: %v", err)
	}
	gh.branchResponses = []string{branchAliasResponse(t, 1, map[int]string{0: ""})}
	if _, err := svc.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, reloadWatches(t, store, watches)); err != nil {
		t.Fatalf("third batched sync: %v", err)
	}
	if got := gh.FindPRByBranchCallCount(); got != 2 {
		t.Errorf("per-watch branch lookups = %d, want 2 after the window elapsed", got)
	}
}

func TestSyncWatchesBatched_DetectsPRWithoutPerWatchLookup(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()

	watches := seedSearchingWatches(t, store, "o", "r", 2)
	gh.SetRepositoryDetails(GitHubRepository{FullName: "o/r", Owner: "o", Name: "r", Fork: false})
	gh.branchResponses = []string{
		branchAliasResponse(t, 2, map[int]string{1: openPRNode(42, "feature/branch-1", "alice")}),
	}

	if _, err := svc.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, watches); err != nil {
		t.Fatalf("batched sync: %v", err)
	}

	updated := reloadWatches(t, store, watches)
	if updated[1].PRNumber != 42 {
		t.Errorf("detected PR number = %d, want 42", updated[1].PRNumber)
	}
	if updated[0].PRNumber != 0 {
		t.Errorf("watch without a PR = #%d, want to stay searching", updated[0].PRNumber)
	}
	if got := gh.FindPRByBranchCallCount(); got != 0 {
		t.Errorf("per-watch branch lookups = %d, want 0", got)
	}
}

func TestSyncWatchesBatched_ForkParentPRStillDetected(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()

	watches := seedSearchingWatches(t, store, "fork", "r", 1)
	gh.SetRepositoryDetails(GitHubRepository{
		FullName: "fork/r", Owner: "fork", Name: "r", Fork: true, ParentFullName: "upstream/r",
	})
	// The PR lives in the parent with the fork as its head repository. The
	// batched query only asked the fork, so a definitive negative there must
	// still reach the parent.
	gh.AddPR(&PR{
		Number: 99, RepoOwner: "upstream", RepoName: "r",
		HeadBranch: "feature/branch-0", HeadRepoOwner: "fork", HeadRepoName: "r",
		State: "open", Title: "fork PR", URL: "https://x/99", BaseBranch: "main",
	})
	gh.branchResponses = []string{branchAliasResponse(t, 1, nil)}

	if _, err := svc.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, watches); err != nil {
		t.Fatalf("batched sync: %v", err)
	}
	if updated := reloadWatches(t, store, watches); updated[0].PRNumber != 99 {
		t.Fatalf("fork-parent PR number = %d, want 99", updated[0].PRNumber)
	}
}

func TestSyncWatchesBatched_ForkParentPRPromotesSharedConsumers(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()
	const branch = "feature/shared-fork-parent"

	watches := make([]*PRWatch, 0, 2)
	for i, taskID := range []string{"task-shared-fork-a", "task-shared-fork-b"} {
		seedTask(t, store, taskID, false)
		owner := "fork"
		if i == 1 {
			owner = "Fork"
		}
		watch := withTestWorkspace(&PRWatch{
			SessionID: fmt.Sprintf("session-shared-fork-%d", i),
			TaskID:    taskID, Owner: owner, Repo: "r", Branch: branch,
		})
		if err := store.CreatePRWatch(ctx, watch); err != nil {
			t.Fatalf("create shared fork watch %d: %v", i, err)
		}
		watches = append(watches, watch)
	}
	gh.SetRepositoryDetails(GitHubRepository{
		FullName: "fork/r", Owner: "fork", Name: "r", Fork: true, ParentFullName: "upstream/r",
	})
	gh.AddPR(&PR{
		Number: 99, RepoOwner: "upstream", RepoName: "r",
		HeadBranch: branch, HeadRepoOwner: "fork", HeadRepoName: "r",
		State: "open", Title: "shared fork PR", URL: "https://x/99", BaseBranch: "main",
	})
	gh.branchResponses = []string{branchAliasResponse(t, 1, nil)}

	if _, err := svc.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, watches); err != nil {
		t.Fatalf("batched sync: %v", err)
	}
	for _, updated := range reloadWatches(t, store, watches) {
		if updated.PRNumber != 99 || updated.Owner != "upstream" || updated.Repo != "r" {
			t.Errorf("watch for task %s = %s/%s#%d, want upstream/r#99", updated.TaskID, updated.Owner, updated.Repo, updated.PRNumber)
		}
		taskPR, err := store.GetTaskPR(ctx, updated.TaskID)
		if err != nil || taskPR == nil || taskPR.PRNumber != 99 {
			t.Errorf("task %s association = %#v, err=%v; shared fork-parent PR was not attached", updated.TaskID, taskPR, err)
		}
	}
	if got := gh.FindPRByBranchCallCount(); got != 1 {
		t.Errorf("fork-parent lookups = %d, want one shared lookup", got)
	}
}

func TestSyncWatchesBatched_ForkParentPRWithForeignHeadIsRejectedAndCached(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()

	watches := seedSearchingWatches(t, store, "fork", "r", 1)
	gh.SetRepositoryDetails(GitHubRepository{
		FullName: "fork/r", Owner: "fork", Name: "r", Fork: true, ParentFullName: "upstream/r",
	})
	// Same branch name, but opened from a different fork — must not associate,
	// so the watch stays searching and both cycles take the fork-parent path.
	gh.AddPR(&PR{
		Number: 99, RepoOwner: "upstream", RepoName: "r",
		HeadBranch: "feature/branch-0", HeadRepoOwner: "someone-else", HeadRepoName: "r",
		State: "open", Title: "other fork PR", URL: "https://x/99", BaseBranch: "main",
	})
	gh.branchResponses = []string{
		branchAliasResponse(t, 1, nil),
		branchAliasResponse(t, 1, nil),
	}

	if _, err := svc.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, watches); err != nil {
		t.Fatalf("first batched sync: %v", err)
	}
	if updated := reloadWatches(t, store, watches); updated[0].PRNumber != 0 {
		t.Fatalf("watch = PR #%d, want to stay searching (foreign head)", updated[0].PRNumber)
	}
	// Age past the freshness window so the second cycle genuinely probes the
	// parent again — otherwise the throttle, not the cache, would explain a
	// single fork lookup.
	stale := time.Now().UTC().Add(-2 * PRSyncFreshnessWindow)
	if err := store.UpdatePRWatchTimestamps(ctx, watches[0].ID, stale, nil, "", ""); err != nil {
		t.Fatalf("age watch: %v", err)
	}
	if _, err := svc.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, reloadWatches(t, store, watches)); err != nil {
		t.Fatalf("second batched sync: %v", err)
	}

	// The parent is a different repository the batch never asked about, so it
	// is probed on both cycles — but fork identity itself is fetched once.
	// The probe here is FindPRByHead, which the mock implements by delegating
	// to FindPRByBranch, so this counter sees it.
	if got := gh.FindPRByBranchCallCount(); got != 2 {
		t.Errorf("fork-parent lookups = %d, want 2 (one per cycle)", got)
	}
	if got := gh.GetRepositoryCallCount(); got != 1 {
		t.Errorf("repository fork lookups = %d, want 1 across 2 cycles", got)
	}
}
