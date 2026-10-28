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

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

const (
	progressTestPRNumber = 4733
	progressTestTaskID   = "task-progress"
)

// progressGraphQLClient keeps real provider admission in front of canned
// GraphQL responses, so a deferral is the production pacing signal rather than
// a test flag.
type progressGraphQLClient struct {
	*MockClient
	admission *RateAdmission
	inner     *progressBatchExecutor
}

func (c *progressGraphQLClient) ExecuteGraphQL(
	ctx context.Context, query string, vars map[string]any, out any,
) error {
	release, err := c.admission.acquire(ctx, ResourceGraphQL)
	if err != nil {
		return err
	}
	defer release()
	return c.inner.ExecuteGraphQL(ctx, query, vars, out)
}

// progressBatchExecutor serves the numbered and branch phase shapes from
// generated fixtures and counts each provider read.
type progressBatchExecutor struct {
	mu            sync.Mutex
	numberedCalls int
	branchCalls   int
	pageCalls     int
	prNumber      int
	reviewThreads int
	checksState   string
	deferredPages int
	// deferredBranches defers that many branch phases before answering them.
	deferredBranches int
	// deferredNumbered defers that many numbered phases before answering them.
	deferredNumbered int
	// deferDelay is the boundary reported by a test deferral.
	deferDelay    time.Duration
	numberedState []string
	onNumbered    func()
	onBranch      func()
}

func (e *progressBatchExecutor) ExecuteGraphQL(
	_ context.Context, query string, _ map[string]any, out any,
) error {
	switch {
	case strings.Contains(query, "ReviewThreadPages"):
		return e.executeReviewThreadPage(out)
	case strings.Contains(query, "query Branches"):
		return e.executeBranchBatch(query, out)
	default:
		return e.executeNumberedBatch(query, out)
	}
}

func (e *progressBatchExecutor) executeNumberedBatch(query string, out any) error {
	e.mu.Lock()
	call := e.numberedCalls
	e.numberedCalls++
	hook := e.onNumbered
	checksState := e.checksState
	prNumber := e.prNumber
	threads := e.reviewThreads
	delay := e.deferDelay
	deferred := e.deferredNumbered > 0
	if deferred {
		e.deferredNumbered--
	}
	if call < len(e.numberedState) && e.numberedState[call] != "" {
		checksState = e.numberedState[call]
	}
	e.mu.Unlock()
	if hook != nil {
		hook()
	}
	if deferred {
		return progressTestDeferral("test numbered pacing", delay)
	}
	data := make(map[string]any)
	for _, match := range batchedRepoAliasPattern.FindAllStringSubmatch(query, -1) {
		data["repo"+match[1]] = map[string]any{
			"pr0": progressPRFixture(prNumber, threads, checksState),
		}
	}
	payload, err := json.Marshal(map[string]any{"data": data})
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, out)
}

func (e *progressBatchExecutor) executeBranchBatch(query string, out any) error {
	e.mu.Lock()
	e.branchCalls++
	delay := e.deferDelay
	deferred := e.deferredBranches > 0
	hook := e.onBranch
	if deferred {
		e.deferredBranches--
	}
	e.mu.Unlock()
	if hook != nil {
		hook()
	}
	if deferred {
		return progressTestDeferral("test branch pacing", delay)
	}
	data := make(map[string]any)
	for index := 0; ; index++ {
		if !strings.Contains(query, fmt.Sprintf("b%d:", index)) {
			break
		}
		data[fmt.Sprintf("b%d", index)] = map[string]any{
			"pullRequests": map[string]any{"nodes": []any{}},
		}
	}
	payload, err := json.Marshal(map[string]any{"data": data})
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, out)
}

func (e *progressBatchExecutor) executeReviewThreadPage(out any) error {
	e.mu.Lock()
	e.pageCalls++
	defer e.mu.Unlock()
	if e.deferredPages > 0 {
		e.deferredPages--
		return &AdmissionDeferredError{Delay: time.Millisecond, Reason: "test review thread pacing"}
	}
	remaining := e.reviewThreads - 1
	nodes := make([]map[string]bool, remaining)
	for index := range nodes {
		nodes[index] = map[string]bool{"isResolved": false}
	}
	payload, err := json.Marshal(map[string]any{"data": map[string]any{
		"repo0": map[string]any{"pr0": map[string]any{
			"reviewThreads": reviewThreadsFixture(remaining, nodes, false, ""),
		}},
	}})
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, out)
}

func (e *progressBatchExecutor) counts() (numbered, branch int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.numberedCalls, e.branchCalls
}

func progressTestDeferral(reason string, delay time.Duration) *AdmissionDeferredError {
	if delay <= 0 {
		delay = 5 * time.Millisecond
	}
	return &AdmissionDeferredError{
		Delay: delay, RetryAt: time.Now().Add(delay), Reason: reason,
	}
}

func progressPRFixture(number, threads int, checksState string) map[string]any {
	fixture := map[string]any{
		"state": "OPEN", "title": "Progress PR", "url": fmt.Sprintf("https://x/%d", number),
		"isDraft": false, "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
		"headRefName": "feat", "baseRefName": "main", "headRefOid": "abc",
		"author":    map[string]any{"login": "alice"},
		"createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-02T00:00:00Z",
		"reviews":        map[string]any{"nodes": []any{}},
		"reviewRequests": map[string]any{"totalCount": 0},
		"reviewThreads":  reviewThreadsFixture(0, nil, false, ""),
		"commits":        map[string]any{"nodes": []any{}},
	}
	if checksState != "" {
		fixture["commits"] = map[string]any{"nodes": []any{
			map[string]any{"commit": map[string]any{
				"statusCheckRollup": map[string]any{"state": checksState},
			}},
		}}
	}
	if threads > 0 {
		fixture["reviewThreads"] = reviewThreadsFixture(
			threads, []map[string]bool{{"isResolved": false}}, true, "cursor-1",
		)
	}
	return fixture
}

func newPRStatusProgressTest(t *testing.T) (*Poller, *Service, *Store, *progressBatchExecutor) {
	t.Helper()
	poller, service, mockClient, store := setupPollerTest(t)
	executor := &progressBatchExecutor{prNumber: progressTestPRNumber, checksState: "SUCCESS"}
	coordinator := NewRateCoordinator(nil, nil)
	_, admission := coordinator.coordinate(defaultGitHubHost, AuthPrincipal{
		Kind: AuthPrincipalHuman, Login: "pr-status-progress",
	}, nil)
	client := &progressGraphQLClient{MockClient: mockClient, admission: admission, inner: executor}
	service.client = client
	configureTestWorkspaceAuth(t, service, client, testWorkspaceID)
	return poller, service, store, executor
}

func seedProgressWatch(t *testing.T, store *Store, sessionID, taskID string, prNumber int, branch string) *PRWatch {
	t.Helper()
	watch := withTestWorkspace(&PRWatch{
		SessionID: sessionID, TaskID: taskID, Owner: "o", Repo: "r",
		PRNumber: prNumber, Branch: branch,
	})
	require.NoError(t, store.CreatePRWatch(context.Background(), watch))
	return watch
}

func countTaskPRUpdatedEvents(t *testing.T, eventBus bus.EventBus, taskID string) func() int {
	t.Helper()
	var mu sync.Mutex
	count := 0
	_, err := eventBus.Subscribe(events.GitHubTaskPRUpdated, func(_ context.Context, event *bus.Event) error {
		row, ok := event.Data.(*TaskPR)
		if !ok || row == nil || row.TaskID != taskID {
			return nil
		}
		mu.Lock()
		count++
		mu.Unlock()
		return nil
	})
	require.NoError(t, err)
	return func() int {
		mu.Lock()
		defer mu.Unlock()
		return count
	}
}

func requireProgressTaskPRState(t *testing.T, store *Store, taskID, checksState string) {
	t.Helper()
	rows, err := store.ListTaskPRsByTask(context.Background(), taskID)
	require.NoError(t, err)
	require.Len(t, rows, 1, "known PR must be persisted while unrelated discovery is deferred")
	require.Equal(t, checksState, rows[0].ChecksState)
}

// runPRStatusProgressCycle drives one monitor pass the way the poller does:
// attempt the whole set, and on a deferral resume only the unfinished phase.
// The known PR must already be published before each wait.
func runPRStatusProgressCycle(
	t *testing.T, service *Service, numbered, searching []*PRWatch,
	assertPublished func(),
) {
	t.Helper()
	ctx := pollerContext(context.Background())
	watches := append(append([]*PRWatch{}, numbered...), searching...)
	for attempt := 0; attempt < 6; attempt++ {
		_, err := service.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, watches)
		if err == nil {
			return
		}
		var deferred *AdmissionDeferredError
		if !errors.As(err, &deferred) {
			t.Fatalf("cycle attempt %d error = %v, want admission deferral", attempt, err)
		}
		assertPublished()
		if waitErr := deferred.Wait(ctx); waitErr != nil {
			t.Fatalf("cycle attempt %d wait for admission boundary: %v", attempt, waitErr)
		}
		if strings.Contains(err.Error(), "batched branch query") {
			watches = append([]*PRWatch{}, searching...)
		}
	}
	t.Fatal("monitor cycle did not complete within its admission attempts")
}

// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-001.6
// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-001.7
// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-002.7
func TestPRStatusProgress_CompletedKnownPhaseSurvivesDiscoveryPacing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, service, store, executor := newPRStatusProgressTest(t)
		updated := countTaskPRUpdatedEvents(t, service.eventBus, progressTestTaskID)
		numbered := []*PRWatch{seedProgressWatch(t, store, "session-progress", progressTestTaskID, progressTestPRNumber, "feat")}
		assertPublished := func() {
			requireProgressTaskPRState(t, store, progressTestTaskID, "success")
			require.Equal(t, 1, updated(), "unchanged PR state must not publish a duplicate summary source event")
		}

		var searching []*PRWatch
		for cycle := 0; cycle < 5; cycle++ {
			searching = append(searching, seedProgressWatch(
				t, store, fmt.Sprintf("session-search-%d", cycle),
				fmt.Sprintf("task-search-%d", cycle), 0, fmt.Sprintf("feature/search-%d", cycle),
			))
			runPRStatusProgressCycle(t, service, numbered, searching, assertPublished)
		}
		// Control cycles keep the same search set: churn is not what publishes
		// the known PR, so the numbered cadence must not change.
		for control := 0; control < 2; control++ {
			runPRStatusProgressCycle(t, service, numbered, searching, assertPublished)
		}

		numberedCalls, branchCalls := executor.counts()
		require.Equal(t, 7, numberedCalls, "one known-PR read per monitor cycle; searching churn must not restart it")
		require.GreaterOrEqual(t, branchCalls, 7)
		assertPublished()
	})
}

// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-001.6
// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-002.4
// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-002.7
func TestPRStatusProgress_PreservesEvidenceAndFences(t *testing.T) {
	t.Run("incomplete review-thread evidence is not published", func(t *testing.T) {
		_, service, store, executor := newPRStatusProgressTest(t)
		executor.reviewThreads = 3
		executor.deferredPages = 1
		watch := seedProgressWatch(t, store, "session-evidence", "task-evidence", progressTestPRNumber, "feat")
		ctx := pollerContext(context.Background())

		completed := false
		for attempt := 0; attempt < 5 && !completed; attempt++ {
			_, err := service.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, []*PRWatch{watch})
			if err == nil {
				completed = true
				continue
			}
			var deferred *AdmissionDeferredError
			require.ErrorAs(t, err, &deferred, "incomplete pagination must leave the phase unfinished")
			rows, listErr := store.ListTaskPRsByTask(ctx, "task-evidence")
			require.NoError(t, listErr)
			require.Empty(t, rows, "partially decoded aggregates must not be published")
			require.NoError(t, deferred.Wait(ctx))
		}
		require.True(t, completed, "pagination must complete once the page read is admitted")

		rows, listErr := store.ListTaskPRsByTask(ctx, "task-evidence")
		require.NoError(t, listErr)
		require.Len(t, rows, 1)
		require.Equal(t, 3, rows[0].UnresolvedReviewThreads, "complete evidence must include every page")
	})

	t.Run("shared target fans out one provider read", func(t *testing.T) {
		_, service, store, executor := newPRStatusProgressTest(t)
		watches := []*PRWatch{
			seedProgressWatch(t, store, "session-shared-a", "task-shared-a", progressTestPRNumber, "feat"),
			seedProgressWatch(t, store, "session-shared-b", "task-shared-b", progressTestPRNumber, "feat"),
		}
		_, err := service.SyncWorkspaceWatchesBatched(context.Background(), testWorkspaceID, watches)
		require.NoError(t, err)
		numberedCalls, _ := executor.counts()
		require.Equal(t, 1, numberedCalls, "shared target must issue one read for both consumers")
		for _, taskID := range []string{"task-shared-a", "task-shared-b"} {
			rows, listErr := store.ListTaskPRsByTask(context.Background(), taskID)
			require.NoError(t, listErr)
			require.Len(t, rows, 1, "shared target consumer %s must receive the result", taskID)
		}
	})

	t.Run("retired watch rejects a held result", func(t *testing.T) {
		_, service, store, executor := newPRStatusProgressTest(t)
		watch := seedProgressWatch(t, store, "session-retired", "task-retired", progressTestPRNumber, "feat")
		updated := countTaskPRUpdatedEvents(t, service.eventBus, "task-retired")
		started := make(chan struct{})
		var once sync.Once
		release := make(chan struct{})
		executor.onNumbered = func() {
			once.Do(func() { close(started) })
			<-release
		}
		done := make(chan error, 1)
		go func() {
			_, err := service.SyncWorkspaceWatchesBatched(
				context.Background(), testWorkspaceID, []*PRWatch{watch},
			)
			done <- err
		}()
		<-started
		_, err := store.RetirePRWatch(context.Background(), watch.ID, time.Now().UTC())
		require.NoError(t, err)
		close(release)
		require.NoError(t, <-done)

		rows, listErr := store.ListTaskPRsByTask(context.Background(), "task-retired")
		require.NoError(t, listErr)
		require.Empty(t, rows, "a retired lifetime must reject the held provider result")
		require.Zero(t, updated(), "a rejected result must not emit a publication event")
	})
}

// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-002.5
func TestTriggerPRSyncAll_DeferredAdmissionSkipsFallback(t *testing.T) {
	deferred := func() *AdmissionDeferredError {
		return &AdmissionDeferredError{Delay: time.Second, Reason: rateLimitBlockBackgroundPacing}
	}
	for _, testCase := range []struct {
		name string
		err  error
	}{
		{name: "plain deferral", err: deferred()},
		{name: "wrapped deferral", err: fmt.Errorf("wrapped provider gate: %w", deferred())},
		{name: "joined deferral", err: errors.Join(errors.New("joined detail"), deferred())},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, service, gh, store := setupBatchedPollerTest(t)
			ctx := context.Background()
			counted := &fallbackCountingClient{Client: gh}
			service.client = counted
			configureTestWorkspaceAuth(t, service, counted, testWorkspaceID)
			seedTask(t, store, "task-passive-deferral", false)
			require.NoError(t, store.CreateTaskPR(ctx, &TaskPR{
				TaskID: "task-passive-deferral", WorkspaceID: testWorkspaceID, RepositoryID: "repo-1",
				Owner: "o", Repo: "r", PRNumber: 42, PRURL: "https://x/42", PRTitle: "Previous",
				HeadBranch: "feat", BaseBranch: "main", State: "open", ChecksState: "success",
			}))
			require.NoError(t, store.CreatePRWatch(ctx, &PRWatch{
				WorkspaceID: testWorkspaceID, SessionID: "session-passive-deferral",
				TaskID: "task-passive-deferral", RepositoryID: "repo-1",
				Owner: "o", Repo: "r", PRNumber: 42, Branch: "feat",
			}))
			gh.prErr = testCase.err

			prs, _, err := service.TriggerPRSyncAllPermanentWithOptions(ctx, "task-passive-deferral", false)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrBackgroundAdmissionDeferred)
			findCalls, statusCalls := counted.fallbackCalls()
			require.Zero(t, findCalls, "a local deferral must not repeat the read over REST")
			require.Zero(t, statusCalls, "a local deferral must not probe PR status over REST")
			require.Len(t, prs, 1)
			require.Equal(t, "success", prs[0].ChecksState, "completed rows must be preserved")
		})
	}
}

// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-002.7
func TestPRStatusProgress_ExplicitRefreshSupersedesHeldPassiveResult(t *testing.T) {
	_, service, store, executor := newPRStatusProgressTest(t)
	watch := seedProgressWatch(t, store, "session-fence", "task-fence", progressTestPRNumber, "feat")
	executor.numberedState = []string{"FAILURE", "SUCCESS"}
	started := make(chan struct{})
	var once sync.Once
	release := make(chan struct{})
	executor.onNumbered = func() {
		once.Do(func() { close(started) })
		<-release
	}

	passiveDone := make(chan error, 1)
	go func() {
		passiveDone <- service.syncWorkspaceWatchesBatched(
			pollerContext(context.Background()), testWorkspaceID, []*PRWatch{watch}, false, false,
		).Err
	}()
	<-started

	explicitDone := make(chan error, 1)
	go func() {
		explicitDone <- service.syncWorkspaceWatchesBatched(
			context.Background(), testWorkspaceID, []*PRWatch{watch}, true, false,
		).Err
	}()
	require.Eventually(t, func() bool {
		executor.mu.Lock()
		defer executor.mu.Unlock()
		return executor.numberedCalls >= 2
	}, time.Second, time.Millisecond, "explicit refresh must start its own provider read")
	close(release)
	require.NoError(t, <-passiveDone)
	require.NoError(t, <-explicitDone)

	requireProgressTaskPRState(t, store, "task-fence", "success")
}
