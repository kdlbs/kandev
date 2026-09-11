package github

import (
	"context"
	"testing"
	"time"
)

func TestPRDiscoveryHealth(t *testing.T) {
	_, service, client, store := setupBatchedPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-health", false)
	watch := withTestWorkspace(&PRWatch{
		SessionID: "session-health",
		TaskID:    "task-health",
		Owner:     "o",
		Repo:      "r",
		Branch:    "feature/health",
	})
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}

	fixedNow := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	service.prDiscoveryHealth.setNow(func() time.Time { return fixedNow })
	scope := testAutomationScope(t, service, testWorkspaceID)
	client.branchResponses = []string{
		`{"errors":[{"type":"GRAPHQL_VALIDATION_FAILED","message":"Field 'cloneUrl' doesn't exist on type 'Repository'"}]}`,
		branchAliasResponse(t, 1, map[int]string{0: openPRNode(9, "feature/health", "alice")}),
	}

	if _, err := service.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, []*PRWatch{watch}); err != nil {
		t.Fatalf("health-aware discovery: %v", err)
	}
	failed := service.prDiscoveryHealth.snapshot(testWorkspaceID, scope, 0)
	if failed.State != PRDiscoveryHealthDegraded || failed.Category != PRDiscoveryHealthInvalidQuery {
		t.Fatalf("health after rejected discovery = %+v, want degraded invalid_query", failed)
	}
	if failed.FailedTargetCount != 1 || failed.RetryAt == nil || !failed.RetryAt.Equal(fixedNow.Add(PRDiscoveryRetryBase)) {
		t.Fatalf("health retry state = %+v, want one target and retry at %v", failed, fixedNow.Add(PRDiscoveryRetryBase))
	}
	if got := len(client.branchQueries); got != 1 {
		t.Fatalf("rejected discovery transport calls = %d, want 1", got)
	}

	// A full quota refresh is independent evidence. It must not erase the
	// discovery failure or make the rejected query eligible before its deadline.
	recordRateLimitResources(service.rateTracker, rateLimitResources{
		Core:    rateLimitBucket{Limit: 5000, Remaining: 5000, Reset: fixedNow.Add(time.Hour).Unix()},
		GraphQL: rateLimitBucket{Limit: 5000, Remaining: 5000, Reset: fixedNow.Add(time.Hour).Unix()},
		Search:  rateLimitBucket{Limit: 30, Remaining: 30, Reset: fixedNow.Add(time.Hour).Unix()},
	})
	if _, err := service.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, []*PRWatch{watch}); err != nil {
		t.Fatalf("suppressed discovery retry: %v", err)
	}
	stillFailed := service.prDiscoveryHealth.snapshot(testWorkspaceID, scope, 0)
	if stillFailed.State != PRDiscoveryHealthDegraded || stillFailed.Category != PRDiscoveryHealthInvalidQuery {
		t.Fatalf("health after suppressed retry = %+v, want unchanged degradation", stillFailed)
	}
	if got := len(client.branchQueries); got != 1 {
		t.Fatalf("suppressed discovery transport calls = %d, want one coalesced attempt", got)
	}

	fixedNow = fixedNow.Add(PRDiscoveryRetryBase)
	if _, err := service.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, []*PRWatch{watch}); err != nil {
		t.Fatalf("discovery after retry deadline: %v", err)
	}
	recovered := service.prDiscoveryHealth.snapshot(testWorkspaceID, scope, 0)
	if recovered.State != PRDiscoveryHealthHealthy || recovered.FailedTargetCount != 0 {
		t.Fatalf("health after newer successful discovery = %+v, want healthy", recovered)
	}
	if got := len(client.branchQueries); got != 2 {
		t.Fatalf("recovered discovery transport calls = %d, want two total attempts", got)
	}
}

func TestPRDiscoveryHealthStaleCompletionAndCredentialIsolation(t *testing.T) {
	fixedNow := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	health := newPRDiscoveryHealth(nil, nil)
	health.setNow(func() time.Time { return fixedNow })
	target := prDiscoveryHealthTarget{Owner: "o", Repo: "r", Branch: "feature/health"}

	first, admitted := health.begin("workspace-a", "credential-a", 1, target)
	if !admitted {
		t.Fatal("first target attempt was not admitted")
	}
	health.finishFailure("workspace-a", "credential-a", 1, target, first, PRDiscoveryHealthInvalidQuery, nil)
	fixedNow = fixedNow.Add(PRDiscoveryRetryBase)
	second, admitted := health.begin("workspace-a", "credential-a", 1, target)
	if !admitted {
		t.Fatal("target was not admitted after its retry deadline")
	}
	health.finishSuccess("workspace-a", "credential-a", 1, target, first)
	stale := health.snapshot("workspace-a", "credential-a", 1)
	if stale.State != PRDiscoveryHealthDegraded || stale.Category != PRDiscoveryHealthInvalidQuery {
		t.Fatalf("stale completion cleared newer failure: %+v", stale)
	}
	health.finishSuccess("workspace-a", "credential-a", 1, target, second)
	if recovered := health.snapshot("workspace-a", "credential-a", 1); recovered.State != PRDiscoveryHealthHealthy {
		t.Fatalf("newer completion did not recover target: %+v", recovered)
	}

	otherCredential, admitted := health.begin("workspace-a", "credential-b", 2, target)
	if !admitted {
		t.Fatal("replacement credential was not admitted independently")
	}
	health.finishFailure("workspace-a", "credential-b", 2, target, otherCredential, PRDiscoveryHealthUnavailable, nil)
	if isolated := health.snapshot("workspace-a", "credential-a", 1); isolated.State != PRDiscoveryHealthHealthy {
		t.Fatalf("replacement credential affected old health scope: %+v", isolated)
	}
	if replacement := health.snapshot("workspace-a", "credential-b", 2); replacement.State != PRDiscoveryHealthDegraded {
		t.Fatalf("replacement credential lost its own failure: %+v", replacement)
	}
}

func TestPRDiscoveryHealthScopeRecreationGetsNewRuntimeEpoch(t *testing.T) {
	health := newPRDiscoveryHealth(nil, nil)
	target := prDiscoveryHealthTarget{Owner: "o", Repo: "r", Branch: "feature/epoch"}
	first, admitted := health.begin("workspace-epoch", "credential-epoch", 1, target)
	if !admitted {
		t.Fatal("initial epoch target was not admitted")
	}
	health.finishSuccess("workspace-epoch", "credential-epoch", 1, target, first)
	firstSnapshot := health.snapshot("workspace-epoch", "credential-epoch", 1)
	health.clearWorkspace("workspace-epoch")
	second, admitted := health.begin("workspace-epoch", "credential-epoch", 1, target)
	if !admitted {
		t.Fatal("recreated epoch target was not admitted")
	}
	health.finishSuccess("workspace-epoch", "credential-epoch", 1, target, second)
	secondSnapshot := health.snapshot("workspace-epoch", "credential-epoch", 1)
	if secondSnapshot.RuntimeEpoch <= firstSnapshot.RuntimeEpoch {
		t.Fatalf("scope runtime epoch = %d after recreation, want greater than %d", secondSnapshot.RuntimeEpoch, firstSnapshot.RuntimeEpoch)
	}
}

func TestClassifyPRDiscoveryError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want PRDiscoveryHealthCategory
	}{
		{
			name: "HTTP 200 GraphQL validation",
			err:  errString("graphql error: Field 'cloneUrl' doesn't exist on type 'Repository'"),
			want: PRDiscoveryHealthInvalidQuery,
		},
		{
			name: "rate limit",
			err:  errString("HTTP 429: API rate limit exceeded"),
			want: PRDiscoveryHealthRateLimited,
		},
		{
			name: "transport unavailable",
			err:  errString("dial tcp: connection refused"),
			want: PRDiscoveryHealthUnavailable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyPRDiscoveryError(tt.err); got != tt.want {
				t.Fatalf("classifyPRDiscoveryError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPRDiscoveryHealth_PreservesGraphQLRateLimitResetDeadline(t *testing.T) {
	fixedNow := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	resetAt := fixedNow.Add(10 * time.Minute)
	executor := &stubGraphQLExecutor{response: `{"data":{"rateLimit":{"limit":5000,"remaining":0,"resetAt":"2026-09-11T12:10:00Z","cost":1}},"errors":[{"type":"RATE_LIMITED","message":"API rate limit exceeded"}]}`}
	_, err := runBatchedBranchQuery(context.Background(), executor, []graphQLBranchRef{{Owner: "o", Repo: "r", Branch: "feature/rate"}})
	if err == nil {
		t.Fatal("runBatchedBranchQuery succeeded, want GraphQL rate-limit error")
	}
	providerRetryAt := prDiscoveryRetryAtFromError(err)
	if providerRetryAt == nil || !providerRetryAt.Equal(resetAt) {
		t.Fatalf("GraphQL retry deadline = %v, want %v", providerRetryAt, resetAt)
	}

	health := newPRDiscoveryHealth(nil, nil)
	health.setNow(func() time.Time { return fixedNow })
	target := prDiscoveryHealthTarget{Owner: "o", Repo: "r", Branch: "feature/rate"}
	attempt, admitted := health.begin("workspace-rate", "credential-rate", 1, target)
	if !admitted {
		t.Fatal("rate-limited target was not admitted")
	}
	health.finishFailure("workspace-rate", "credential-rate", 1, target, attempt, PRDiscoveryHealthRateLimited, providerRetryAt)
	fixedNow = fixedNow.Add(2 * time.Minute)
	if _, admitted = health.begin("workspace-rate", "credential-rate", 1, target); admitted {
		t.Fatal("target was admitted before provider reset deadline")
	}
}

func TestPRDiscoveryHealth_RemovesFailedTargetAfterLastWatchConsumer(t *testing.T) {
	_, service, _, store := setupBatchedPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-health-consumers", false)
	watchA := withTestWorkspace(&PRWatch{
		SessionID: "session-health-consumer-a", TaskID: "task-health-consumers", Owner: "o", Repo: "r",
		Branch: "feature/health-consumers",
	})
	watchB := withTestWorkspace(&PRWatch{
		SessionID: "session-health-consumer-b", TaskID: "task-health-consumers", Owner: "o", Repo: "r",
		Branch: "feature/health-consumers",
	})
	for _, watch := range []*PRWatch{watchA, watchB} {
		if err := store.CreatePRWatch(ctx, watch); err != nil {
			t.Fatalf("create watch: %v", err)
		}
	}
	scope := testAutomationScope(t, service, testWorkspaceID)
	failWatch := func(watch *PRWatch) {
		attempt, admitted := service.beginPRDiscoveryWatch(testWorkspaceID, scope, 1, watch)
		if !admitted {
			t.Fatalf("watch %s was not admitted", watch.ID)
		}
		service.prDiscoveryHealth.finishFailure(
			testWorkspaceID, scope, 1, attempt.target, attempt.number, PRDiscoveryHealthUnavailable, nil,
		)
	}
	failWatch(watchA)
	// The second watch joins the same target, so one failed target remains.
	service.trackPRDiscoveryWatchConsumer(testWorkspaceID, scope, 1, watchB)
	failed := service.prDiscoveryHealth.snapshot(testWorkspaceID, scope, 0)
	if failed.FailedTargetCount != 1 {
		t.Fatalf("shared failed target count = %d, want 1", failed.FailedTargetCount)
	}

	if err := service.DeletePRWatch(ctx, watchA.ID); err != nil {
		t.Fatalf("delete first consumer: %v", err)
	}
	stillFailed := service.prDiscoveryHealth.snapshot(testWorkspaceID, scope, 0)
	if stillFailed.FailedTargetCount != 1 {
		t.Fatalf("health after deleting one shared consumer = %+v, want degraded", stillFailed)
	}
	if err := service.DeletePRWatch(ctx, watchB.ID); err != nil {
		t.Fatalf("delete last consumer: %v", err)
	}
	recovered := service.prDiscoveryHealth.snapshot(testWorkspaceID, scope, 0)
	if recovered.State != PRDiscoveryHealthHealthy || recovered.FailedTargetCount != 0 || recovered.Revision <= failed.Revision {
		t.Fatalf("health after deleting last consumer = %+v, want newer healthy projection", recovered)
	}
}

func TestPRDiscoveryHealth_PreservesMixedLiveTargets(t *testing.T) {
	_, service, _, store := setupBatchedPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-health-mixed", false)
	watchA := withTestWorkspace(&PRWatch{
		SessionID: "session-health-mixed-a", TaskID: "task-health-mixed", Owner: "o", Repo: "r",
		Branch: "feature/mixed-a",
	})
	watchB := withTestWorkspace(&PRWatch{
		SessionID: "session-health-mixed-b", TaskID: "task-health-mixed", Owner: "o", Repo: "r",
		Branch: "feature/mixed-b",
	})
	for _, watch := range []*PRWatch{watchA, watchB} {
		if err := store.CreatePRWatch(ctx, watch); err != nil {
			t.Fatalf("create mixed watch: %v", err)
		}
	}
	scope := testAutomationScope(t, service, testWorkspaceID)
	for _, watch := range []*PRWatch{watchA, watchB} {
		attempt, admitted := service.beginPRDiscoveryWatch(testWorkspaceID, scope, 1, watch)
		if !admitted {
			t.Fatalf("watch %s was not admitted", watch.ID)
		}
		service.prDiscoveryHealth.finishFailure(
			testWorkspaceID, scope, 1, attempt.target, attempt.number, PRDiscoveryHealthUnavailable, nil,
		)
	}
	failed := service.prDiscoveryHealth.snapshot(testWorkspaceID, scope, 0)
	if failed.FailedTargetCount != 2 {
		t.Fatalf("mixed target health = %+v, want two failed targets", failed)
	}

	if err := service.DeletePRWatch(ctx, watchA.ID); err != nil {
		t.Fatalf("delete first mixed target: %v", err)
	}
	remaining := service.prDiscoveryHealth.snapshot(testWorkspaceID, scope, 0)
	if remaining.State != PRDiscoveryHealthDegraded || remaining.FailedTargetCount != 1 {
		t.Fatalf("health after deleting one mixed target = %+v, want one remaining failure", remaining)
	}
	if _, admitted := service.beginPRDiscoveryWatch(testWorkspaceID, scope, 1, watchB); admitted {
		t.Fatal("remaining failed target was admitted before its retry deadline")
	}

	if err := service.DeletePRWatch(ctx, watchB.ID); err != nil {
		t.Fatalf("delete last mixed target: %v", err)
	}
	recovered := service.prDiscoveryHealth.snapshot(testWorkspaceID, scope, 0)
	if recovered.State != PRDiscoveryHealthHealthy || recovered.FailedTargetCount != 0 || recovered.Revision <= failed.Revision {
		t.Fatalf("health after deleting mixed targets = %+v, want newer healthy projection", recovered)
	}
}

func TestPRDiscoveryHealth_PrunesArchivedAndBranchSwitchedTargets(t *testing.T) {
	ctx := context.Background()
	t.Run("archive", func(t *testing.T) {
		_, service, _, store := setupBatchedPollerTest(t)
		seedTask(t, store, "task-health-archive", false)
		watch := withTestWorkspace(&PRWatch{
			SessionID: "session-health-archive", TaskID: "task-health-archive", Owner: "o", Repo: "r",
			Branch: "feature/archive",
		})
		if err := store.CreatePRWatch(ctx, watch); err != nil {
			t.Fatalf("create archive watch: %v", err)
		}
		scope := testAutomationScope(t, service, testWorkspaceID)
		attempt, admitted := service.beginPRDiscoveryWatch(testWorkspaceID, scope, 1, watch)
		if !admitted {
			t.Fatal("archive watch was not admitted")
		}
		service.prDiscoveryHealth.finishFailure(testWorkspaceID, scope, 1, attempt.target, attempt.number, PRDiscoveryHealthUnavailable, nil)
		service.pruneWatchesForTask(ctx, watch.TaskID, "archived")
		health := service.prDiscoveryHealth.snapshot(testWorkspaceID, scope, 0)
		if health.State != PRDiscoveryHealthHealthy || health.FailedTargetCount != 0 {
			t.Fatalf("archived target health = %+v, want healthy", health)
		}
	})

	t.Run("branch switch", func(t *testing.T) {
		_, service, _, store := setupBatchedPollerTest(t)
		seedTask(t, store, "task-health-branch", false)
		watch := withTestWorkspace(&PRWatch{
			SessionID: "session-health-branch", TaskID: "task-health-branch", Owner: "o", Repo: "r",
			Branch: "feature/old-branch",
		})
		if err := store.CreatePRWatch(ctx, watch); err != nil {
			t.Fatalf("create branch watch: %v", err)
		}
		scope := testAutomationScope(t, service, testWorkspaceID)
		attempt, admitted := service.beginPRDiscoveryWatch(testWorkspaceID, scope, 1, watch)
		if !admitted {
			t.Fatal("branch watch was not admitted")
		}
		service.prDiscoveryHealth.finishFailure(testWorkspaceID, scope, 1, attempt.target, attempt.number, PRDiscoveryHealthUnavailable, nil)
		failed := service.prDiscoveryHealth.snapshot(testWorkspaceID, scope, 0)
		if failed.FailedTargetCount != 1 {
			t.Fatalf("branch failure health = %+v, want degraded", failed)
		}
		if err := service.UpdatePRWatchBranchIfSearching(ctx, watch.ID, "feature/new-branch"); err != nil {
			t.Fatalf("switch branch: %v", err)
		}
		health := service.prDiscoveryHealth.snapshot(testWorkspaceID, scope, 0)
		if health.State != PRDiscoveryHealthHealthy || health.FailedTargetCount != 0 || health.Revision <= failed.Revision {
			t.Fatalf("branch-switched health = %+v, want newer healthy projection", health)
		}
	})
}

type errString string

func (e errString) Error() string { return string(e) }
