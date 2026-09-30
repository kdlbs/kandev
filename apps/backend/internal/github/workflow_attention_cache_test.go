package github

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type workflowAttentionCacheClient struct {
	*MockClient
	runCalls atomic.Int32
	jobCalls atomic.Int32
	runsErr  error
	jobsErr  error
	started  chan struct{}
	release  chan struct{}
}

func (c *workflowAttentionCacheClient) ListWorkflowRuns(
	ctx context.Context, owner, repo, headSHA string,
) ([]WorkflowRun, error) {
	c.runCalls.Add(1)
	if c.started != nil {
		select {
		case c.started <- struct{}{}:
		default:
		}
	}
	if c.release != nil {
		<-c.release
	}
	if c.runsErr != nil {
		return nil, c.runsErr
	}
	return c.MockClient.ListWorkflowRuns(ctx, owner, repo, headSHA)
}

func (c *workflowAttentionCacheClient) ListWorkflowRunJobs(
	ctx context.Context, owner, repo string, runID int64, attempt int,
) ([]WorkflowJob, error) {
	c.jobCalls.Add(1)
	if c.jobsErr != nil {
		return nil, c.jobsErr
	}
	return c.MockClient.ListWorkflowRunJobs(ctx, owner, repo, runID, attempt)
}

func (c *workflowAttentionCacheClient) GetPRStatus(
	ctx context.Context, owner, repo string, number int,
) (*PRStatus, error) {
	return getPRStatus(ctx, c, owner, repo, number)
}

func (c *workflowAttentionCacheClient) GetPRFeedback(
	ctx context.Context, owner, repo string, number int,
) (*PRFeedback, error) {
	return getPRFeedback(ctx, c, owner, repo, number)
}

func TestWorkflowAttentionCacheExpiryAndInvalidation(t *testing.T) {
	ctx := context.Background()
	clock := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	client := &workflowAttentionCacheClient{MockClient: NewMockClient()}
	svc := newTestService(client)
	setWorkflowAttentionCacheClock(svc, func() time.Time { return clock })

	prOne := &PR{
		Number: 1, State: "open", HeadSHA: "same-sha", HeadBranch: "feature",
		RepoOwner: "acme", RepoName: "widget", HeadRepoOwner: "acme", HeadRepoName: "widget",
	}
	prTwo := *prOne
	prTwo.Number = 2
	client.AddPR(prOne)
	client.AddPR(&prTwo)
	client.ReplaceWorkflowRuns("acme", "widget", "same-sha", []WorkflowRun{{
		ID: 11, RunAttempt: 1, WorkflowID: 7, Name: "CI", Event: "pull_request",
		Status: workflowStatusCompleted, Conclusion: "success", HeadSHA: "same-sha",
		HeadBranch: "feature", HeadRepoOwner: "acme", HeadRepoName: "widget",
		CreatedAt: clock, UpdatedAt: clock,
	}})

	if _, err := svc.collectWorkflowAttention(ctx, client, "scope-a", "acme", "widget", prOne); err != nil {
		t.Fatalf("first workflow read: %v", err)
	}
	if _, err := svc.collectWorkflowAttention(ctx, client, "scope-a", "acme", "widget", &prTwo); err != nil {
		t.Fatalf("same-SHA second PR workflow read: %v", err)
	}
	if got := client.runCalls.Load(); got != 1 {
		t.Fatalf("same-SHA PRs used %d run reads, want 1", got)
	}

	// Service status reads use the same Actions observation as the batch path.
	svc.prStatusCache.clear()
	if _, err := svc.getPRStatus(ctx, client, "scope-a", "acme", "widget", prOne.Number); err != nil {
		t.Fatalf("service status read: %v", err)
	}
	if got := client.runCalls.Load(); got != 1 {
		t.Fatalf("service status reused no Actions cache: run reads = %d, want 1", got)
	}
	svc.prFeedbackCache.clear()
	if _, err := svc.getPRFeedback(ctx, client, "scope-a", "", "acme", "widget", prOne.Number); err != nil {
		t.Fatalf("service feedback read: %v", err)
	}
	if got := client.runCalls.Load(); got != 1 {
		t.Fatalf("service feedback reused no Actions cache: run reads = %d, want 1", got)
	}

	// A completed collection is retained for five minutes. A same-SHA rerun is
	// visible after that expiry and the next successful read.
	clock = clock.Add(5*time.Minute + time.Nanosecond)
	client.ReplaceWorkflowRuns("acme", "widget", "same-sha", []WorkflowRun{{
		ID: 12, RunAttempt: 2, WorkflowID: 7, Name: "CI", Event: "pull_request",
		Status: workflowStatusCompleted, Conclusion: "success", HeadSHA: "same-sha",
		HeadBranch: "feature", HeadRepoOwner: "acme", HeadRepoName: "widget",
		CreatedAt: clock, UpdatedAt: clock,
	}})
	svc.prStatusCache.clear()
	if _, err := svc.getPRStatus(ctx, client, "scope-a", "acme", "widget", prOne.Number); err != nil {
		t.Fatalf("same-SHA rerun read: %v", err)
	}
	if got := client.runCalls.Load(); got != 2 {
		t.Fatalf("same-SHA rerun remained cached: run reads = %d, want 2", got)
	}

	// Pending observations use the short expiry. Replace the data after the
	// short window and verify the completed result is read.
	clock = clock.Add(time.Second)
	client.ReplaceWorkflowRuns("acme", "widget", "same-sha", []WorkflowRun{{
		ID: 13, RunAttempt: 1, WorkflowID: 7, Name: "CI", Event: "pull_request",
		Status: "in_progress", HeadSHA: "same-sha", HeadBranch: "feature",
		HeadRepoOwner: "acme", HeadRepoName: "widget", CreatedAt: clock, UpdatedAt: clock,
	}})
	svc.invalidateWorkflowAttentionForPR("scope-a", "acme", "widget", prOne.Number, "same-sha")
	svc.prStatusCache.clear()
	if _, err := svc.getPRStatus(ctx, client, "scope-a", "acme", "widget", prOne.Number); err != nil {
		t.Fatalf("pending workflow read: %v", err)
	}
	clock = clock.Add(30*time.Second + time.Nanosecond)
	client.ReplaceWorkflowRuns("acme", "widget", "same-sha", []WorkflowRun{{
		ID: 14, RunAttempt: 1, WorkflowID: 7, Name: "CI", Event: "pull_request",
		Status: workflowStatusCompleted, Conclusion: "failure", HeadSHA: "same-sha",
		HeadBranch: "feature", HeadRepoOwner: "acme", HeadRepoName: "widget",
		CreatedAt: clock, UpdatedAt: clock,
	}})
	svc.prStatusCache.clear()
	if _, err := svc.getPRStatus(ctx, client, "scope-a", "acme", "widget", prOne.Number); err != nil {
		t.Fatalf("expired pending workflow read: %v", err)
	}
	if got := client.runCalls.Load(); got != 4 {
		t.Fatalf("short workflow expiry did not refetch: run reads = %d, want 4", got)
	}

	// Jobs include the run attempt in their key. A later attempt cannot reuse
	// an earlier attempt's observation.
	client.ReplaceWorkflowRunJobs("acme", "widget", 99, 1, []WorkflowJob{{
		ID: 991, Status: workflowStatusCompleted, Conclusion: "success",
	}})
	client.ReplaceWorkflowRunJobs("acme", "widget", 99, 2, []WorkflowJob{{
		ID: 992, Status: workflowStatusCompleted, Conclusion: "failure",
	}})
	if _, err := svc.cachedWorkflowJobs(ctx, client, "scope-a", "acme", "widget", 99, 1, false); err != nil {
		t.Fatalf("first job attempt: %v", err)
	}
	if _, err := svc.cachedWorkflowJobs(ctx, client, "scope-a", "acme", "widget", 99, 1, false); err != nil {
		t.Fatalf("cached first job attempt: %v", err)
	}
	if _, err := svc.cachedWorkflowJobs(ctx, client, "scope-a", "acme", "widget", 99, 2, false); err != nil {
		t.Fatalf("second job attempt: %v", err)
	}
	if got := client.jobCalls.Load(); got != 2 {
		t.Fatalf("job attempts were not isolated: job reads = %d, want 2", got)
	}

	// Errors are returned but never cached as an empty observation.
	readErr := errors.New("actions unavailable")
	client.runsErr = readErr
	if _, err := svc.cachedWorkflowRuns(ctx, client, "scope-a", "acme", "widget", "error-sha"); !errors.Is(err, readErr) {
		t.Fatalf("workflow read error = %v, want %v", err, readErr)
	}
	client.runsErr = nil
	if _, err := svc.cachedWorkflowRuns(ctx, client, "scope-a", "acme", "widget", "error-sha"); err != nil {
		t.Fatalf("workflow read after error: %v", err)
	}
	if got := client.runCalls.Load(); got != 6 {
		t.Fatalf("workflow error was cached: run reads = %d, want 6", got)
	}

	// Different credential generations use different cache namespaces.
	if _, err := svc.cachedWorkflowRuns(ctx, client, "scope-b", "acme", "widget", "error-sha"); err != nil {
		t.Fatalf("second credential scope: %v", err)
	}
	if got := client.runCalls.Load(); got != 7 {
		t.Fatalf("credential scopes shared Actions data: run reads = %d, want 7", got)
	}

	// Explicit refresh invalidates both the outer PR response and the Actions
	// observations. The next status read therefore sees a fresh provider value.
	outerKey := scopedCacheKey("scope-a", prStatusCacheKey("acme", "widget", prOne.Number))
	svc.prStatusCache.set(outerKey, "stale")
	svc.invalidateWorkflowAttentionForPR("scope-a", "acme", "widget", prOne.Number, "error-sha")
	if _, ok := svc.prStatusCache.get(outerKey); ok {
		t.Fatal("explicit refresh left the outer status cache populated")
	}
	if _, err := svc.cachedWorkflowRuns(ctx, client, "scope-a", "acme", "widget", "error-sha"); err != nil {
		t.Fatalf("workflow read after explicit refresh: %v", err)
	}
	if got := client.runCalls.Load(); got != 8 {
		t.Fatalf("explicit refresh reused Actions data: run reads = %d, want 8", got)
	}

	// Concurrent misses for one key coalesce, even when they arrive through
	// separate service callers.
	concurrent := newTestService(client)
	setWorkflowAttentionCacheClock(concurrent, func() time.Time { return clock })
	client.started = make(chan struct{}, 1)
	client.release = make(chan struct{})
	const callers = 12
	var ready sync.WaitGroup
	var done sync.WaitGroup
	ready.Add(callers)
	done.Add(callers)
	start := make(chan struct{})
	for range callers {
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			if _, err := concurrent.cachedWorkflowRuns(ctx, client, "scope-c", "acme", "widget", "concurrent-sha"); err != nil {
				t.Errorf("concurrent workflow read: %v", err)
			}
		}()
	}
	ready.Wait()
	close(start)
	<-client.started
	close(client.release)
	done.Wait()
	if got := client.runCalls.Load(); got != 9 {
		t.Fatalf("concurrent misses did not coalesce: run reads = %d, want 9", got)
	}

	// The run cache is bounded. Stagger fake time so eviction order is
	// deterministic, then ensure the oldest entry is fetched again.
	eviction := newTestService(client)
	setWorkflowAttentionCacheClock(eviction, func() time.Time { return clock })
	for index := 0; index < workflowAttentionCacheMaxEntries; index++ {
		if _, err := eviction.cachedWorkflowRuns(ctx, client, "scope-evict", "acme", "widget", "evict-"+itoa(index)); err != nil {
			t.Fatalf("eviction fill %d: %v", index, err)
		}
		clock = clock.Add(time.Nanosecond)
	}
	before := client.runCalls.Load()
	if _, err := eviction.cachedWorkflowRuns(ctx, client, "scope-evict", "acme", "widget", "evict-overflow"); err != nil {
		t.Fatalf("eviction overflow: %v", err)
	}
	if _, err := eviction.cachedWorkflowRuns(ctx, client, "scope-evict", "acme", "widget", "evict-0"); err != nil {
		t.Fatalf("evicted entry refill: %v", err)
	}
	if got := client.runCalls.Load(); got != before+2 {
		t.Fatalf("bounded eviction kept oldest entry: reads before=%d after=%d", before, got)
	}
}

func TestWorkflowAttentionCacheInvalidationDropsInFlightObservation(t *testing.T) {
	client := &workflowAttentionCacheClient{
		MockClient: NewMockClient(),
		started:    make(chan struct{}, 1),
		release:    make(chan struct{}),
	}
	svc := newTestService(client)
	ctx := context.Background()

	var firstErr error
	done := make(chan struct{})
	go func() {
		_, firstErr = svc.cachedWorkflowRuns(ctx, client, "scope", "acme", "widget", "head")
		close(done)
	}()
	<-client.started
	svc.invalidateWorkflowAttentionForPR("scope", "acme", "widget", 1, "head")
	close(client.release)
	<-done
	if firstErr != nil {
		t.Fatalf("in-flight workflow read: %v", firstErr)
	}
	if _, err := svc.cachedWorkflowRuns(ctx, client, "scope", "acme", "widget", "head"); err != nil {
		t.Fatalf("workflow read after invalidation: %v", err)
	}
	if got := client.runCalls.Load(); got != 2 {
		t.Fatalf("in-flight result repopulated invalidated cache: run reads = %d, want 2", got)
	}
}

func setWorkflowAttentionCacheClock(svc *Service, now func() time.Time) {
	svc.workflowRunsCache.now = now
	svc.workflowJobsCache.now = now
}

func itoa(value int) string {
	return fmt.Sprintf("%d", value)
}
