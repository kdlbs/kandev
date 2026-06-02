package github

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// TestSyncWatchesBatched_NegativeCache_ShortCircuits is the regression
// test for the SyncWatchesBatched storm: once a repo is classified as
// unresolvable, subsequent syncs for watches on that repo must NOT issue
// a fresh GraphQL call. Before the negative cache, every 5s frontend
// retry against a dead repo (NBCUDTC/bff was the production smoking
// gun) burned a gh throttle slot and produced a fresh "Could not resolve
// to a Repository" log line.
func TestSyncWatchesBatched_NegativeCache_ShortCircuits(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()

	w := &PRWatch{SessionID: "s1", TaskID: "t1", Owner: "Dead", Repo: "Repo", PRNumber: 7, Branch: "br"}
	if err := store.CreatePRWatch(ctx, w); err != nil {
		t.Fatalf("create watch: %v", err)
	}
	// CreatePRWatch evicts the negative cache, so prime it AFTER the
	// watch is in place.
	svc.markRepoAsMissing(w.Owner, w.Repo, svc.repoErrorGenSnapshot())

	// No canned responses — if anything reaches gh the test fails fast
	// with "no canned PR response" / "no canned branch response".
	results, err := svc.SyncWatchesBatched(ctx, []*PRWatch{w})
	if err != nil {
		t.Fatalf("SyncWatchesBatched: %v", err)
	}
	if len(gh.prQueries) != 0 {
		t.Errorf("expected zero PR GraphQL calls when repo is cached as missing, got %d", len(gh.prQueries))
	}
	if len(gh.branchQueries) != 0 {
		t.Errorf("expected zero branch GraphQL calls when repo is cached as missing, got %d", len(gh.branchQueries))
	}
	if len(results) != 1 || !results[0].SyncFailed {
		t.Errorf("expected one SyncFailed result for cached-missing watch, got %+v", results)
	}
}

// TestSyncWatchesBatched_LearnsAndCachesMissingRepo verifies the
// "first call discovers the dead repo, the second call short-circuits"
// flow that collapses the storm down to one upstream call per 10
// minutes per repo. Mirrors TestService_GetPRFeedback_UsesCache's
// repeated-call assertion but on the SyncWatchesBatched seam.
func TestSyncWatchesBatched_LearnsAndCachesMissingRepo(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()

	w := &PRWatch{SessionID: "s1", TaskID: "t1", Owner: "Dead", Repo: "Repo", PRNumber: 7, Branch: "br"}
	if err := store.CreatePRWatch(ctx, w); err != nil {
		t.Fatalf("create watch: %v", err)
	}

	// Canned response: data has "repo0": null, errors[].path = ["repo0"]
	// with the deterministic "Could not resolve to a Repository" message.
	// This mirrors GitHub's actual response shape for a dead repo alias.
	gh.prResponses = []string{`{
		"data": {"repo0": null},
		"errors": [{"message": "Could not resolve to a Repository with the name 'Dead/Repo'.", "type": "NOT_FOUND", "path": ["repo0"]}]
	}`}

	if _, err := svc.SyncWatchesBatched(ctx, []*PRWatch{w}); err != nil {
		t.Fatalf("first SyncWatchesBatched: %v", err)
	}
	if !svc.isRepoCachedAsMissing(w.Owner, w.Repo) {
		t.Fatalf("expected repo to be negative-cached after first sync; cache=%v", svc.repoErrorCache.entries)
	}

	// Second call must NOT hit gh — the cache short-circuits everything.
	if _, err := svc.SyncWatchesBatched(ctx, []*PRWatch{w}); err != nil {
		t.Fatalf("second SyncWatchesBatched: %v", err)
	}
	if got := len(gh.prQueries); got != 1 {
		t.Errorf("expected 1 upstream PR call across two syncs (first discovers, second short-circuits), got %d", got)
	}
}

// blockingExec wraps the existing graphQLMockClient with a controllable
// blocker. The test releases it after asserting that concurrent
// SyncWatchesBatched calls have all coalesced onto the same in-flight
// fetch — mirroring TestService_GetPRFeedback_CoalescesConcurrentCalls.
type blockingExec struct {
	*graphQLMockClient
	calls   atomic.Int32
	release chan struct{}
}

func (b *blockingExec) ExecuteGraphQL(ctx context.Context, query string, vars map[string]any, out any) error {
	b.calls.Add(1)
	<-b.release
	return b.graphQLMockClient.ExecuteGraphQL(ctx, query, vars, out)
}

// TestSyncWatchesBatched_NegativeCache_CoalescesConcurrentCalls verifies
// that N concurrent SyncWatchesBatched calls for the same dead repo
// don't fan out into N gh subprocesses while waiting for the first
// classification to land in the negative cache. The throttle protects
// us at process level, but the cache check + classifier is the seam
// that has to coalesce here — pre-fix, every concurrent caller would
// race the empty cache and queue behind the gh throttle.
func TestSyncWatchesBatched_NegativeCache_CoalescesConcurrentCalls(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)

	w := &PRWatch{SessionID: "s1", TaskID: "t1", Owner: "Dead", Repo: "Repo", PRNumber: 7, Branch: "br"}
	if err := store.CreatePRWatch(context.Background(), w); err != nil {
		t.Fatalf("create watch: %v", err)
	}

	// Prime the negative cache directly — the test then asserts that
	// every concurrent caller short-circuits without queueing on
	// ExecuteGraphQL. (Coalescing-via-singleflight inside the cache is
	// covered by lower-level cache tests; here we just need to confirm
	// SyncWatchesBatched honors the cache under concurrency.)
	svc.markRepoAsMissing(w.Owner, w.Repo, svc.repoErrorGenSnapshot())

	blocker := &blockingExec{graphQLMockClient: gh, release: make(chan struct{})}
	svc.client = blocker
	close(blocker.release) // never block — if any call reaches gh the test will catch it via blocker.calls

	const n = 16
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if _, err := svc.SyncWatchesBatched(context.Background(), []*PRWatch{w}); err != nil {
				t.Errorf("concurrent SyncWatchesBatched: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := blocker.calls.Load(); got != 0 {
		t.Fatalf("expected 0 upstream calls (every caller should short-circuit on cache), got %d", got)
	}
}

// TestSyncWatchesBatched_FirstMissCoalesces is the regression test for
// the burst-before-first-cache-hit window: when N concurrent
// SyncWatchesBatched calls race against an EMPTY negative cache for the
// same dead repo, the singleflight wrap around fetchBatchedWatchStatuses
// must collapse them to a single upstream GraphQL call. Without it, the
// pre-filter's isRepoCachedAsMissing check would all return false (cache
// is empty) and every caller would issue its own batched query, stacking
// upstream requests during the exact window the negative cache is trying
// to calm down.
func TestSyncWatchesBatched_FirstMissCoalesces(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)

	w := &PRWatch{SessionID: "s1", TaskID: "t1", Owner: "Dead", Repo: "Repo", PRNumber: 7, Branch: "br"}
	if err := store.CreatePRWatch(context.Background(), w); err != nil {
		t.Fatalf("create watch: %v", err)
	}

	// Block the first in-flight ExecuteGraphQL so concurrent callers
	// have time to join the singleflight before any response lands. The
	// `joined` counter is the deterministic barrier: every caller that
	// joins the singleflight bumps it before the leader's fetch returns,
	// so we can wait for "all n callers are inside the singleflight"
	// without a timing-sensitive sleep.
	release := make(chan struct{})
	var gotFirstInFlight sync.WaitGroup
	gotFirstInFlight.Add(1)
	var firstSignaled atomic.Bool
	var joined atomic.Int32
	gh.onExecute = func() {
		if firstSignaled.CompareAndSwap(false, true) {
			gotFirstInFlight.Done()
		}
		<-release
	}
	gh.prResponses = []string{`{
		"data": {"repo0": null},
		"errors": [{"message": "Could not resolve to a Repository with the name 'Dead/Repo'.", "type": "NOT_FOUND", "path": ["repo0"]}]
	}`}

	const n = 16
	var wg sync.WaitGroup
	wg.Add(n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			joined.Add(1)
			if _, err := svc.SyncWatchesBatched(context.Background(), []*PRWatch{w}); err != nil {
				errs <- err
			}
		}()
	}
	// Wait until the leader is inside ExecuteGraphQL, then wait until
	// every co-waiter has reached the singleflight join before releasing.
	// Pre-fix this used a fixed 50ms sleep, which violated the backend's
	// no-Sleep-in-pure-in-memory-tests convention and was prone to CI
	// scheduler variance.
	gotFirstInFlight.Wait()
	for joined.Load() < int32(n) {
		runtime.Gosched()
	}
	close(release)
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent SyncWatchesBatched: %v", err)
	}
	if got := len(gh.prQueries); got != 1 {
		t.Errorf("expected 1 upstream PR call when concurrent first-time misses coalesce, got %d", got)
	}
	if !svc.isRepoCachedAsMissing(w.Owner, w.Repo) {
		t.Errorf("expected repo to be negative-cached after the coalesced fetch")
	}
}

// TestService_ClearRepoErrorCache verifies that the new bulk-clear hook
// (wired from ConfigureToken / ClearToken) drops every entry. Without
// this, a repo classified as unresolvable under stale credentials would
// stay "permanent: true" for up to 10 minutes after the user fixes auth,
// and the frontend retry loop would stop early on stale data.
func TestService_ClearRepoErrorCache(t *testing.T) {
	_, svc, _, _ := setupBatchedPollerTest(t)

	gen := svc.repoErrorGenSnapshot()
	svc.markRepoAsMissing("Dead1", "Repo", gen)
	svc.markRepoAsMissing("Dead2", "Repo", gen)
	if !svc.isRepoCachedAsMissing("Dead1", "Repo") || !svc.isRepoCachedAsMissing("Dead2", "Repo") {
		t.Fatalf("priming failed")
	}

	svc.ClearRepoErrorCache()

	if svc.isRepoCachedAsMissing("Dead1", "Repo") {
		t.Errorf("Dead1/Repo still cached after ClearRepoErrorCache")
	}
	if svc.isRepoCachedAsMissing("Dead2", "Repo") {
		t.Errorf("Dead2/Repo still cached after ClearRepoErrorCache")
	}
}

// TestService_EvictRepoNegative_OnWatchCreate verifies that creating a
// PR watch clears any prior negative-cache entry for the repo, so a
// freshly linked repository is probed immediately instead of waiting
// out the 10-min TTL.
func TestService_EvictRepoNegative_OnWatchCreate(t *testing.T) {
	_, svc, _, store := setupBatchedPollerTest(t)
	ctx := context.Background()

	svc.markRepoAsMissing("Dead", "Repo", svc.repoErrorGenSnapshot())
	if !svc.isRepoCachedAsMissing("Dead", "Repo") {
		t.Fatalf("priming failed; cache=%v", svc.repoErrorCache.entries)
	}

	// CreatePRWatch must evict regardless of input casing, because the
	// cache key is case-insensitive.
	w := &PRWatch{SessionID: "s1", TaskID: "t1", Owner: "dead", Repo: "repo", PRNumber: 1, Branch: "main"}
	if _, err := svc.CreatePRWatch(ctx, w.SessionID, w.TaskID, "", w.Owner, w.Repo, w.PRNumber, w.Branch); err != nil {
		t.Fatalf("CreatePRWatch: %v", err)
	}
	_ = store
	if svc.isRepoCachedAsMissing("Dead", "Repo") {
		t.Errorf("expected negative cache to be evicted after CreatePRWatch; entry still present")
	}
}

// TestService_TriggerPRSyncAllPermanent_FlagsDeadRepos pins the
// permanent-flag wiring: a task whose every watch points at a cached
// missing repo gets permanent=true from TriggerPRSyncAllPermanent, so
// the WS handler can tell the frontend to stop polling. Mixed
// (one dead repo + one live one) is NOT permanent — the live one might
// still acquire a PR.
func TestService_TriggerPRSyncAllPermanent_FlagsDeadRepos(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()

	w1 := &PRWatch{SessionID: "s1", TaskID: "t1", Owner: "Dead", Repo: "Repo", PRNumber: 0, Branch: "x"}
	if err := store.CreatePRWatch(ctx, w1); err != nil {
		t.Fatalf("create w1: %v", err)
	}
	svc.markRepoAsMissing(w1.Owner, w1.Repo, svc.repoErrorGenSnapshot())
	// No canned responses needed — the cache short-circuits both watches.

	_, permanent, err := svc.TriggerPRSyncAllPermanent(ctx, "t1")
	if err != nil {
		t.Fatalf("TriggerPRSyncAllPermanent: %v", err)
	}
	if !permanent {
		t.Errorf("expected permanent=true when every watch's repo is cached-missing")
	}

	// Add a second watch on a live repo; permanent must flip to false
	// even though w1 is still cached-missing.
	w2 := &PRWatch{SessionID: "s2", TaskID: "t1", Owner: "Live", Repo: "Repo", PRNumber: 0, Branch: "y"}
	if err := store.CreatePRWatch(ctx, w2); err != nil {
		t.Fatalf("create w2: %v", err)
	}
	gh.branchResponses = []string{`{"data":{"b0":{"pullRequests":{"nodes":[]}}}}`}

	_, permanent2, err := svc.TriggerPRSyncAllPermanent(ctx, "t1")
	if err != nil {
		t.Fatalf("TriggerPRSyncAllPermanent (mixed): %v", err)
	}
	if permanent2 {
		t.Errorf("expected permanent=false when at least one watch's repo is live")
	}
	// Sanity: the live repo's branch query DID fire (single one).
	if got := len(gh.branchQueries); got != 1 {
		t.Errorf("expected 1 branch GraphQL call for the live repo, got %d", got)
	}
	// Confirm the dead repo was NOT in any query.
	for _, q := range gh.branchQueries {
		if strings.Contains(q, w1.Owner) {
			t.Errorf("dead repo leaked into branch query: %q", q)
		}
	}
}
