package github

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestLoadValidation_TenSimulatedMinutes_NoWatchCreationLoopBoundedLookups is
// the Task 07 deterministic load-validation fixture (plan.md Task 07,
// "Document operations and validate sustained load"). It reproduces the
// original amplification scenario at scale — one task resumed across 50
// historical sessions (mirroring AC1's 50-session fixture) alongside 24
// unrelated concurrently-active tasks, standing in for many simultaneous
// task/coordinator boards — and drives ten simulated poll cycles at the
// poller's real one-minute cadence (defaultPRPollInterval) without sleeping
// wall-clock time between them.
//
// It asserts the three AC1/AC3-shaped invariants a production amplification
// regression would violate:
//  1. No watch-creation loop: the exact set of watch IDs after the first
//     converged cycle is byte-for-byte identical after the tenth cycle, and
//     the canonical watch count equals the number of distinct
//     task/repository/branch identities (25), never the 74 total
//     historical-session entries the provider reports.
//  2. Bounded lookups: each cycle performs exactly one batched GraphQL
//     branch-search call for all 25 still-searching canonical watches
//     (below graphQLBatchChunkSize), independent of how many sessions ever
//     existed for any one task.
//  3. Stable latency: no cycle's wall-clock duration exceeds a generous
//     absolute bound, ruling out the quadratic-cost-per-cycle shape a
//     duplicate-row leak would produce.
func TestLoadValidation_TenSimulatedMinutes_NoWatchCreationLoopBoundedLookups(t *testing.T) {
	poller, svc, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()

	const (
		historicalSessions = 50
		otherTasks         = 24
		canonicalWatches   = otherTasks + 1 // + the one resumed historical task
		simulatedCycles    = 10             // ten simulated minutes at defaultPRPollInterval
	)

	// Seed the task rows ListActivePRWatches joins against.
	seedTask(t, store, "task-hist", false)
	for i := 0; i < otherTasks; i++ {
		seedTask(t, store, fmt.Sprintf("task-%02d", i), false)
	}

	// The resumed task: 50 historical sessions, all resolving to the same
	// task/repository/branch — the exact amplification shape from AC1.
	var provided []TaskBranchInfo
	for i := 0; i < historicalSessions; i++ {
		provided = append(provided, TaskBranchInfo{
			WorkspaceID:  testWorkspaceID,
			TaskID:       "task-hist",
			SessionID:    fmt.Sprintf("session-hist-%03d", i),
			RepositoryID: "repo-hist",
			Owner:        "acme",
			Repo:         "hist-repo",
			Branch:       "feature-hist",
		})
	}
	// 24 unrelated concurrently-active tasks/boards, each its own canonical
	// identity, standing in for many simultaneous coordinator-driven tasks.
	for i := 0; i < otherTasks; i++ {
		provided = append(provided, TaskBranchInfo{
			WorkspaceID:  testWorkspaceID,
			TaskID:       fmt.Sprintf("task-%02d", i),
			SessionID:    fmt.Sprintf("session-%02d", i),
			RepositoryID: fmt.Sprintf("repo-%02d", i),
			Owner:        "acme",
			Repo:         fmt.Sprintf("repo-%02d", i),
			Branch:       fmt.Sprintf("feature-%02d", i),
		})
	}
	poller.SetTaskBranchProvider(&mockTaskBranchProvider{tasks: provided})

	// One canned branch-search response per simulated cycle: every canonical
	// watch resolves to zero open PRs, so all 25 remain "searching" and
	// generate a fresh lookup on the following cycle too (worst case for
	// lookup-count amplification).
	emptyAliases := make([]string, canonicalWatches)
	for i := range emptyAliases {
		emptyAliases[i] = fmt.Sprintf(`"b%d":{"pullRequests":{"nodes":[]}}`, i)
	}
	branchResp := fmt.Sprintf(`{"data":{%s}}`, strings.Join(emptyAliases, ","))
	for i := 0; i < simulatedCycles; i++ {
		gh.branchResponses = append(gh.branchResponses, branchResp)
	}

	durations := make([]time.Duration, 0, simulatedCycles)
	var firstCycleWatchIDs, lastCycleWatchIDs map[string]struct{}

	for cycle := 0; cycle < simulatedCycles; cycle++ {
		start := time.Now()
		poller.checkPRWatches(ctx)
		durations = append(durations, time.Since(start))

		watches, err := svc.ListActivePRWatches(ctx)
		if err != nil {
			t.Fatalf("cycle %d: ListActivePRWatches: %v", cycle, err)
		}
		if got := len(watches); got != canonicalWatches {
			t.Fatalf("cycle %d: watch count = %d, want %d canonical watches (not %d historical entries)",
				cycle, got, canonicalWatches, historicalSessions+otherTasks)
		}
		ids := make(map[string]struct{}, len(watches))
		for _, w := range watches {
			ids[w.ID] = struct{}{}
		}
		if cycle == 0 {
			firstCycleWatchIDs = ids
		}
		lastCycleWatchIDs = ids
	}

	// Invariant 1: zero net inserts/deletes across the nine cycles following
	// convergence — the exact watch-ID set is unchanged.
	if len(firstCycleWatchIDs) != len(lastCycleWatchIDs) {
		t.Fatalf("watch set size changed across cycles: first=%d last=%d",
			len(firstCycleWatchIDs), len(lastCycleWatchIDs))
	}
	for id := range firstCycleWatchIDs {
		if _, ok := lastCycleWatchIDs[id]; !ok {
			t.Fatalf("watch %s present after cycle 0 but missing after cycle %d (creation loop / churn)",
				id, simulatedCycles-1)
		}
	}

	// Invariant 2: exactly one batched branch lookup per cycle — 25 canonical
	// watches collapse into one GraphQL call, not 74 (or 25 * N) calls.
	if got := len(gh.branchQueries); got != simulatedCycles {
		t.Fatalf("branch GraphQL calls = %d, want %d (one batched lookup per cycle)", got, simulatedCycles)
	}
	if got := len(gh.prQueries); got != 0 {
		t.Fatalf("numbered PR GraphQL calls = %d, want 0 (no watch ever resolved a PR number)", got)
	}

	// Invariant 3: stable per-cycle latency; a duplicate-row leak would make
	// later cycles measurably and increasingly slower than the first.
	const maxCycleLatency = 2 * time.Second
	var total time.Duration
	for i, d := range durations {
		total += d
		if d > maxCycleLatency {
			t.Fatalf("cycle %d latency = %v, want <= %v", i, d, maxCycleLatency)
		}
	}
	t.Logf("load validation: %d cycles, %d canonical watches (from %d historical-session entries), "+
		"%d branch GraphQL calls, total cycle time %v, average cycle time %v",
		simulatedCycles, canonicalWatches, historicalSessions+otherTasks,
		len(gh.branchQueries), total, total/time.Duration(simulatedCycles))
}
