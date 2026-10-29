package github

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// fallbackCountingClient records REST fallback probes so a local admission
// deferral can be proven not to reach the provider a second way.
type fallbackCountingClient struct {
	Client
	mu          sync.Mutex
	findCalls   int
	statusCalls int
}

func (c *fallbackCountingClient) ExecuteGraphQL(
	ctx context.Context, query string, vars map[string]any, out any,
) error {
	exec, ok := c.Client.(GraphQLExecutor)
	if !ok {
		return errGraphQLUnsupported
	}
	return exec.ExecuteGraphQL(ctx, query, vars, out)
}

func (c *fallbackCountingClient) FindPRByBranch(
	ctx context.Context, owner, repo, branch string,
) (*PR, error) {
	c.mu.Lock()
	c.findCalls++
	c.mu.Unlock()
	return c.Client.FindPRByBranch(ctx, owner, repo, branch)
}

func (c *fallbackCountingClient) GetPRStatus(
	ctx context.Context, owner, repo string, number int,
) (*PRStatus, error) {
	c.mu.Lock()
	c.statusCalls++
	c.mu.Unlock()
	return c.Client.GetPRStatus(ctx, owner, repo, number)
}

func (c *fallbackCountingClient) fallbackCalls() (find, status int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.findCalls, c.statusCalls
}

func startPRMonitorLoop(t *testing.T, poller *Poller) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(pollerContext(context.Background()))
	poller.wg.Add(1)
	go poller.prMonitorLoop(ctx)
	t.Cleanup(cancel)
	return cancel
}

func seedPRMonitorContinuationWatches(t *testing.T, store *Store) (*PRWatch, *PRWatch) {
	t.Helper()
	seedTask(t, store, "task-continuation-pr", false)
	seedTask(t, store, "task-continuation-search", false)
	numbered := withTestWorkspace(&PRWatch{
		SessionID: "session-continuation-pr", TaskID: "task-continuation-pr",
		Owner: "o", Repo: "r", PRNumber: progressTestPRNumber, Branch: "feat",
	})
	require.NoError(t, store.CreatePRWatch(context.Background(), numbered))
	searching := withTestWorkspace(&PRWatch{
		SessionID: "session-continuation-search", TaskID: "task-continuation-search",
		Owner: "o", Repo: "r", Branch: "feature/continuation",
	})
	require.NoError(t, store.CreatePRWatch(context.Background(), searching))
	return numbered, searching
}

func pendingWorkspaceIDs(poller *Poller) []string {
	poller.pendingMu.Lock()
	defer poller.pendingMu.Unlock()
	ids := make([]string, 0, len(poller.pending))
	for workspaceID := range poller.pending {
		ids = append(ids, workspaceID)
	}
	return ids
}

func pendingTargetKeys(poller *Poller) []string {
	poller.pendingMu.Lock()
	defer poller.pendingMu.Unlock()
	keys := make([]string, 0)
	for _, work := range poller.pending {
		for key := range work.targets {
			keys = append(keys, key)
		}
	}
	return keys
}

func pendingConsumerCount(poller *Poller) int {
	poller.pendingMu.Lock()
	defer poller.pendingMu.Unlock()
	total := 0
	for _, work := range poller.pending {
		total += prMonitorPendingConsumerCount(work)
	}
	return total
}

func seedTestWorkspaceConnection(
	t *testing.T, store *Store, workspaceID, login string, generation int64,
) {
	t.Helper()
	now := time.Now().UTC()
	require.NoError(t, store.UpsertWorkspaceConnection(context.Background(), &WorkspaceConnection{
		WorkspaceID:          workspaceID,
		Source:               ConnectionSourcePAT,
		GitHubHost:           defaultGitHubHost,
		Login:                login,
		Status:               ConnectionStatusActive,
		CredentialGeneration: generation,
		CreatedAt:            now,
		UpdatedAt:            now,
	}))
}

// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-002.6
// @covers AC-INTEGRATIONS-GITHUB-RATE-002.2
// @covers AC-INTEGRATIONS-GITHUB-RATE-002.7
func TestPRMonitorContinuation_ResumesAtAdmissionBoundary(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		poller, _, store, executor := newPRStatusProgressTest(t)
		seedPRMonitorContinuationWatches(t, store)
		startPRMonitorLoop(t, poller)
		synctest.Wait()

		numberedCalls, branchCalls := executor.counts()
		require.Equal(t, 1, numberedCalls, "the known-PR phase completes on the first pass")
		require.Zero(t, branchCalls, "the searching phase must be deferred by the first principal read")
		requireProgressTaskPRState(t, store, "task-continuation-pr", "success")
		require.Equal(t, []string{testWorkspaceID}, pendingWorkspaceIDs(poller))

		// A one-second deferral must resume before the one-minute inventory tick.
		time.Sleep(2 * time.Second)
		synctest.Wait()
		numberedCalls, branchCalls = executor.counts()
		require.Equal(t, 1, branchCalls, "the deferred phase resumes at its admission boundary")
		require.Equal(t, 1, numberedCalls, "resuming the deferred phase must not repeat the completed phase")
		require.Empty(t, pendingWorkspaceIDs(poller), "completed work releases its pending entry")
	})
}

// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-002.5
func TestPRMonitorContinuation_LocalDeferralSkipsFallback(t *testing.T) {
	poller, service, store, executor := newPRStatusProgressTest(t)
	counted := &fallbackCountingClient{Client: service.client}
	service.client = counted
	configureTestWorkspaceAuth(t, service, counted, testWorkspaceID)
	_, searching := seedPRMonitorContinuationWatches(t, store)
	_ = searching

	var feedback int
	_, err := poller.eventBus.Subscribe(events.GitHubPRFeedback, func(context.Context, *bus.Event) error {
		feedback++
		return nil
	})
	require.NoError(t, err)

	poller.checkPRWatches(pollerContext(context.Background()))

	findCalls, statusCalls := counted.fallbackCalls()
	require.Zero(t, findCalls, "local admission deferral must not repeat the request over REST")
	require.Zero(t, statusCalls, "local admission deferral must not probe PR status over REST")
	openAuth, openConfig, openTransient := poller.circuits.summary(time.Now().UTC())
	require.Zero(t, openAuth+openConfig+openTransient, "a local deferral is not a provider failure")
	require.Equal(t, 1, feedback, "only the completed known-PR phase publishes a status change")
	requireProgressTaskPRState(t, store, "task-continuation-pr", "success")
	require.Equal(t, []string{testWorkspaceID}, pendingWorkspaceIDs(poller))
	_, branchCalls := executor.counts()
	require.Zero(t, branchCalls)
}

// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-002.6
// @covers AC-INTEGRATIONS-GITHUB-RATE-002.3
func TestPRMonitorContinuation_PreservesFairnessAndBudgets(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		poller, service, store, deferredExecutor := newPRStatusProgressTest(t)
		seedPRMonitorContinuationWatches(t, store)
		deferredClient := &fallbackCountingClient{Client: service.client}
		service.client = deferredClient

		// A second, independently eligible principal must keep progressing while
		// the first one waits for its admission boundary.
		seedTask(t, store, "task-independent", false)
		independentExecutor := &progressBatchExecutor{prNumber: 999, checksState: "SUCCESS"}
		independent := &progressGraphQLClient{MockClient: NewMockClient(), inner: independentExecutor}
		// A transport failure stays inside its cycle budget and must not be
		// replayed by a continuation wake.
		seedTask(t, store, "task-transport", false)
		transportClient := &fallbackCountingClient{Client: &graphQLMockClient{
			MockClient: NewMockClient(),
			branchErr:  errors.New("graphql 500"),
		}}
		configureTestWorkspaceAuth(t, service, deferredClient,
			testWorkspaceID, "workspace-free", "workspace-transport")
		service.resolver.SetAutomationProvider(workspaceAutomationCredentialProvider{clients: map[string]Client{
			testWorkspaceID:       deferredClient,
			"workspace-free":      independent,
			"workspace-transport": transportClient,
		}})
		require.NoError(t, store.CreatePRWatch(context.Background(), &PRWatch{
			WorkspaceID: "workspace-free", SessionID: "session-independent", TaskID: "task-independent",
			Owner: "o", Repo: "r", PRNumber: 999, Branch: "feat",
		}))
		require.NoError(t, store.CreatePRWatch(context.Background(), &PRWatch{
			WorkspaceID: "workspace-transport", SessionID: "session-transport", TaskID: "task-transport",
			Owner: "o", Repo: "r", Branch: "feature/transport",
		}))

		poller.checkPRWatches(pollerContext(context.Background()))
		independentNumbered, _ := independentExecutor.counts()
		require.Equal(t, 1, independentNumbered, "an eligible principal completes while another is deferred")
		requireProgressTaskPRState(t, store, "task-independent", "success")
		transportFind, _ := transportClient.fallbackCalls()
		require.Greater(t, transportFind, 0, "a transport failure uses its bounded REST fallback")
		deferredFind, _ := deferredClient.fallbackCalls()
		require.Zero(t, deferredFind, "a local deferral must not enter REST fallback")

		time.Sleep(2 * time.Second)
		poller.resumePRMonitorPending(pollerContext(context.Background()))

		_, deferredBranch := deferredExecutor.counts()
		require.Equal(t, 1, deferredBranch)
		independentNumbered, _ = independentExecutor.counts()
		require.Equal(t, 1, independentNumbered, "continuation wakes must not repeat completed work")
		transportFindAfter, _ := transportClient.fallbackCalls()
		require.Equal(t, transportFind, transportFindAfter,
			"continuation wakes must not replenish a fallback budget")
		deferredFindAfter, _ := deferredClient.fallbackCalls()
		require.Zero(t, deferredFindAfter)
		require.Empty(t, pendingWorkspaceIDs(poller))
	})
}

// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-001.3
// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-002.7
func TestPRMonitorContinuation_PrunesObsoleteWorkAndStops(t *testing.T) {
	t.Run("retiring a deferred searching lifetime drops its work", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			poller, _, store, executor := newPRStatusProgressTest(t)
			_, searching := seedPRMonitorContinuationWatches(t, store)
			searchKey := prWatchDiscoveryTargetKey(searching)
			executor.deferredBranches = 1
			executor.deferDelay = 10 * time.Minute
			poller.checkPRWatches(pollerContext(context.Background()))
			require.Equal(t, []string{searchKey}, pendingTargetKeys(poller))

			_, err := store.RetirePRWatch(context.Background(), searching.ID, time.Now().UTC())
			require.NoError(t, err)
			poller.checkPRWatches(pollerContext(context.Background()))
			require.NotContains(t, pendingTargetKeys(poller), searchKey,
				"retired targets must not keep pending work")
		})
	})

	t.Run("retiring a deferred known-PR lifetime drops its work", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			poller, _, store, executor := newPRStatusProgressTest(t)
			numbered, _ := seedPRMonitorContinuationWatches(t, store)
			numberedKey := prWatchDiscoveryTargetKey(numbered)
			executor.deferredNumbered = 1
			executor.deferDelay = 10 * time.Minute
			poller.checkPRWatches(pollerContext(context.Background()))
			require.Contains(t, pendingTargetKeys(poller), numberedKey)

			_, err := store.RetirePRWatch(context.Background(), numbered.ID, time.Now().UTC())
			require.NoError(t, err)
			poller.checkPRWatches(pollerContext(context.Background()))
			require.NotContains(t, pendingTargetKeys(poller), numberedKey,
				"a retired known-PR lifetime must not keep pending work")
		})
	})

	t.Run("removal and shutdown clear retained work", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			poller, _, store, executor := newPRStatusProgressTest(t)
			_, searching := seedPRMonitorContinuationWatches(t, store)
			executor.deferredBranches = 1
			executor.deferDelay = 10 * time.Minute
			poller.checkPRWatches(pollerContext(context.Background()))
			require.NotEmpty(t, pendingTargetKeys(poller))

			require.NoError(t, store.DeletePRWatch(context.Background(), searching.ID))
			// Let the principal's pacing window clear so the surviving known-PR
			// phase can complete during this verification pass.
			time.Sleep(2 * time.Second)
			poller.checkPRWatches(pollerContext(context.Background()))
			require.Empty(t, pendingTargetKeys(poller), "removed targets must not keep pending work")

			// A fresh deferral on a new target is cleared by shutdown.
			seedTask(t, store, "task-stop", false)
			require.NoError(t, store.CreatePRWatch(context.Background(), &PRWatch{
				WorkspaceID: testWorkspaceID, SessionID: "session-stop", TaskID: "task-stop",
				Owner: "o", Repo: "r", Branch: "feature/stop",
			}))
			executor.deferredBranches = 1
			time.Sleep(2 * time.Second)
			poller.checkPRWatches(pollerContext(context.Background()))
			require.NotEmpty(t, pendingTargetKeys(poller))

			cancel := startPRMonitorLoop(t, poller)
			synctest.Wait()
			cancel()
			poller.wg.Wait()
			require.Empty(t, pendingTargetKeys(poller), "shutdown clears retained work")
		})
	})
}

// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-002.6
func TestPRMonitorContinuation_PreservesConsumersOnSharedTargetWake(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		poller, _, store, executor := newPRStatusProgressTest(t)
		for _, suffix := range []string{"a", "b"} {
			taskID := "task-shared-wake-" + suffix
			seedTask(t, store, taskID, false)
			require.NoError(t, store.CreatePRWatch(context.Background(), &PRWatch{
				WorkspaceID: testWorkspaceID, SessionID: "session-shared-wake-" + suffix,
				TaskID: taskID, Owner: "o", Repo: "r",
				PRNumber: progressTestPRNumber, Branch: "feat",
			}))
		}
		executor.deferredNumbered = 1
		executor.deferDelay = 10 * time.Second
		poller.checkPRWatches(pollerContext(context.Background()))
		require.Equal(t, 2, pendingConsumerCount(poller))

		// A new consumer joins the same provider target before the boundary. Its
		// own deferral must not evict the consumers already waiting.
		seedTask(t, store, "task-shared-wake-c", false)
		require.NoError(t, store.CreatePRWatch(context.Background(), &PRWatch{
			WorkspaceID: testWorkspaceID, SessionID: "session-shared-wake-c",
			TaskID: "task-shared-wake-c", Owner: "o", Repo: "r",
			PRNumber: progressTestPRNumber, Branch: "feat",
		}))
		executor.deferredNumbered = 1
		executor.deferDelay = 10 * time.Second
		time.Sleep(2 * time.Second)
		poller.checkPRWatches(pollerContext(context.Background()))

		require.Equal(t, 3, pendingConsumerCount(poller),
			"a new consumer must not drop siblings already waiting on the target")
	})
}

// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-002.7
func TestPRMonitorContinuation_ResumeSkipsRebranchedWatch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		poller, _, store, executor := newPRStatusProgressTest(t)
		seedTask(t, store, "task-rebranch", false)
		watch := &PRWatch{
			WorkspaceID: testWorkspaceID, SessionID: "session-rebranch",
			TaskID: "task-rebranch", Owner: "o", Repo: "r", Branch: "feature/rebranch-old",
		}
		require.NoError(t, store.CreatePRWatch(context.Background(), watch))
		executor.deferredBranches = 1
		executor.deferDelay = 10 * time.Minute
		poller.checkPRWatches(pollerContext(context.Background()))
		require.Equal(t, 1, pendingConsumerCount(poller))

		require.NoError(t, store.UpdatePRWatchBranchIfSearching(
			context.Background(), watch.ID, "feature/rebranch-new",
		))
		time.Sleep(2 * time.Second)
		poller.resumePRMonitorPending(pollerContext(context.Background()))

		_, branchCalls := executor.counts()
		require.Equal(t, 1, branchCalls, "a re-branched watch must not query its old branch")
		require.Empty(t, pendingTargetKeys(poller))

		poller.checkPRWatches(pollerContext(context.Background()))
		_, branchCalls = executor.counts()
		require.Equal(t, 2, branchCalls, "the next inventory tick polls the new branch")
	})
}

// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-001.6
func TestPRStatusProgress_PublishesNumberedPhaseWhileSearchingIsHeld(t *testing.T) {
	_, service, mockClient, store := setupPollerTest(t)
	executor := &progressBatchExecutor{prNumber: progressTestPRNumber, checksState: "SUCCESS"}
	client := &progressGraphQLClient{MockClient: mockClient, inner: executor}
	service.client = client
	configureTestWorkspaceAuth(t, service, client, testWorkspaceID)
	ctx := context.Background()
	seedTask(t, store, "task-held-known", false)
	seedTask(t, store, "task-held-search", false)
	require.NoError(t, store.CreatePRWatch(ctx, &PRWatch{
		WorkspaceID: testWorkspaceID, SessionID: "session-held-known", TaskID: "task-held-known",
		Owner: "o", Repo: "r", PRNumber: progressTestPRNumber, Branch: "feat",
	}))
	require.NoError(t, store.CreatePRWatch(ctx, &PRWatch{
		WorkspaceID: testWorkspaceID, SessionID: "session-held-search", TaskID: "task-held-search",
		Owner: "o", Repo: "r", Branch: "feature/held-search",
	}))
	watches, err := service.store.ListActivePRWatches(ctx)
	require.NoError(t, err)

	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	executor.onBranch = func() {
		once.Do(func() { close(started) })
		<-release
	}
	done := make(chan error, 1)
	go func() {
		done <- service.syncWorkspaceWatchesBatchedForPoller(ctx, testWorkspaceID, watches).Err
	}()
	<-started
	// The completed known-PR phase must be persisted while the unrelated search
	// is still in flight, not after it finishes or times out.
	requireProgressTaskPRState(t, store, "task-held-known", "success")
	close(release)
	require.NoError(t, <-done)
}

// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-002.6
func TestPRMonitorContinuation_RepeatedDeferralRearmsForward(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		poller, _, store, executor := newPRStatusProgressTest(t)
		seedTask(t, store, "task-rearm", false)
		require.NoError(t, store.CreatePRWatch(context.Background(), &PRWatch{
			WorkspaceID: testWorkspaceID, SessionID: "session-rearm", TaskID: "task-rearm",
			Owner: "o", Repo: "r", PRNumber: progressTestPRNumber, Branch: "feat",
		}))
		executor.deferredNumbered = 2
		executor.deferDelay = 3 * time.Second
		startPRMonitorLoop(t, poller)
		synctest.Wait()
		numberedCalls, _ := executor.counts()
		require.Equal(t, 1, numberedCalls)
		require.Equal(t, 1, pendingConsumerCount(poller))

		time.Sleep(3500 * time.Millisecond)
		synctest.Wait()
		numberedCalls, _ = executor.counts()
		require.Equal(t, 2, numberedCalls, "the second deferral must not retry before its own boundary")
		deadline, ok := poller.prMonitorPendingDeadline()
		require.True(t, ok)
		require.True(t, deadline.After(time.Now()),
			"a repeated deferral must replace its expired deadline, not keep it")

		time.Sleep(3 * time.Second)
		synctest.Wait()
		numberedCalls, _ = executor.counts()
		require.Equal(t, 3, numberedCalls)
		requireProgressTaskPRState(t, store, "task-rearm", "success")
		require.Empty(t, pendingTargetKeys(poller))
	})
}

// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-002.6
// @covers AC-INTEGRATIONS-GITHUB-RATE-002.7
func TestPRMonitorContinuation_HonorsExtendedProviderDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		poller, _, store, executor := newPRStatusProgressTest(t)
		seedTask(t, store, "task-provider-deadline", false)
		require.NoError(t, store.CreatePRWatch(context.Background(), &PRWatch{
			WorkspaceID: testWorkspaceID, SessionID: "session-provider-deadline",
			TaskID: "task-provider-deadline", Owner: "o", Repo: "r",
			PRNumber: progressTestPRNumber, Branch: "feat",
		}))
		executor.deferredNumbered = 1
		executor.deferDelay = 10 * time.Minute
		startPRMonitorLoop(t, poller)
		synctest.Wait()
		numberedCalls, _ := executor.counts()
		require.Equal(t, 1, numberedCalls)

		// Inventory ticks and the retry floor must not shorten a provider boundary.
		time.Sleep(8 * time.Minute)
		synctest.Wait()
		numberedCalls, _ = executor.counts()
		require.Equal(t, 1, numberedCalls, "a long provider boundary must not be reduced to the poll interval")

		time.Sleep(2 * time.Minute)
		synctest.Wait()
		numberedCalls, _ = executor.counts()
		require.Equal(t, 2, numberedCalls, "the target resumes at its provider boundary")
		requireProgressTaskPRState(t, store, "task-provider-deadline", "success")
	})
}

// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-002.6
func TestPRMonitorContinuation_ResumesEverySharedTargetConsumer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		poller, _, store, executor := newPRStatusProgressTest(t)
		for _, suffix := range []string{"a", "b"} {
			taskID := "task-shared-resume-" + suffix
			seedTask(t, store, taskID, false)
			require.NoError(t, store.CreatePRWatch(context.Background(), &PRWatch{
				WorkspaceID: testWorkspaceID,
				SessionID:   "session-shared-resume-" + suffix,
				TaskID:      taskID,
				Owner:       "o",
				Repo:        "r",
				PRNumber:    progressTestPRNumber,
				Branch:      "feat",
			}))
		}
		executor.deferredNumbered = 1
		executor.deferDelay = time.Second
		startPRMonitorLoop(t, poller)
		synctest.Wait()
		require.Equal(t, 2, pendingConsumerCount(poller),
			"both tasks watching one provider target must be retained")

		time.Sleep(2 * time.Second)
		synctest.Wait()
		numberedCalls, _ := executor.counts()
		require.Equal(t, 2, numberedCalls, "the resumed target must issue one provider read")
		requireProgressTaskPRState(t, store, "task-shared-resume-a", "success")
		requireProgressTaskPRState(t, store, "task-shared-resume-b", "success")
		require.Empty(t, pendingTargetKeys(poller))
	})
}

// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-002.7
func TestPRMonitorContinuation_CredentialReplacementDropsStaleBoundary(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		poller, _, store, executor := newPRStatusProgressTest(t)
		seedTask(t, store, "task-credential", false)
		seedTestWorkspaceConnection(t, store, testWorkspaceID, "principal-a", 1)
		require.NoError(t, store.CreatePRWatch(context.Background(), &PRWatch{
			WorkspaceID: testWorkspaceID, SessionID: "session-credential", TaskID: "task-credential",
			Owner: "o", Repo: "r", PRNumber: progressTestPRNumber, Branch: "feat",
		}))
		executor.deferredNumbered = 1
		executor.deferDelay = 10 * time.Minute

		poller.checkPRWatches(pollerContext(context.Background()))
		require.Equal(t, 1, pendingConsumerCount(poller))
		deadline, ok := poller.prMonitorPendingDeadline()
		require.True(t, ok)
		require.True(t, deadline.After(time.Now().Add(9*time.Minute)))

		// A different principal now owns the workspace: the old boundary must not
		// gate its first eligible attempt.
		seedTestWorkspaceConnection(t, store, testWorkspaceID, "principal-b", 2)
		time.Sleep(2 * time.Second)
		poller.checkPRWatches(pollerContext(context.Background()))

		numberedCalls, _ := executor.counts()
		require.Equal(t, 2, numberedCalls, "the new principal is admitted at the next normal attempt")
		require.Empty(t, pendingTargetKeys(poller))
		requireProgressTaskPRState(t, store, "task-credential", "success")
	})
}

// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-002.7
func TestPRMonitorContinuation_ResumeLifetimeDropsStaleSuppression(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		poller, _, store, executor := newPRStatusProgressTest(t)
		seedTask(t, store, "task-resume-lifetime", false)
		watch := &PRWatch{
			WorkspaceID: testWorkspaceID, SessionID: "session-resume-lifetime",
			TaskID: "task-resume-lifetime", Owner: "o", Repo: "r", Branch: "feature/resume-lifetime",
		}
		require.NoError(t, store.CreatePRWatch(context.Background(), watch))
		executor.deferredBranches = 1
		executor.deferDelay = 10 * time.Minute

		poller.checkPRWatches(pollerContext(context.Background()))
		require.Equal(t, 1, pendingConsumerCount(poller))

		now := time.Now().UTC()
		_, err := store.RetirePRWatch(context.Background(), watch.ID, now)
		require.NoError(t, err)
		resumed, err := store.ResumePRWatch(context.Background(), watch.ID, now)
		require.NoError(t, err)
		require.NotEqual(t, watch.ID, resumed.ID, "resumption starts a new watch lifetime")

		time.Sleep(2 * time.Second)
		poller.checkPRWatches(pollerContext(context.Background()))

		_, branchCalls := executor.counts()
		require.Equal(t, 2, branchCalls, "the resumed lifetime is admitted without waiting for the old boundary")
		require.Empty(t, pendingTargetKeys(poller))
	})
}
