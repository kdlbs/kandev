package github

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

// countingFeedbackClient counts top-level GetPRFeedback calls so the cache /
// singleflight behavior of Service.GetPRFeedback can be asserted without
// reaching real GitHub.
type countingFeedbackClient struct {
	*MockClient
	calls atomic.Int32
}

func (c *countingFeedbackClient) GetPRFeedback(_ context.Context, owner, repo string, number int) (*PRFeedback, error) {
	c.calls.Add(1)
	return &PRFeedback{PR: &PR{RepoOwner: owner, RepoName: repo, Number: number}}, nil
}

// Repeated feedback fetches for the same PR within the TTL window must collapse
// to a single upstream call. Before the cache was added, every call hit GitHub
// (4 sequential REST calls each), so a render/mount burst on the task page
// fanned out into dozens of slow, rate-limited duplicate requests.
func TestService_GetPRFeedback_UsesCache(t *testing.T) {
	client := &countingFeedbackClient{MockClient: NewMockClient()}
	svc := newTestService(client)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, err := svc.GetPRFeedback(ctx, "o", "r", 1); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if got := client.calls.Load(); got != 1 {
		t.Fatalf("expected 1 upstream call, got %d", got)
	}

	if _, err := svc.GetPRFeedback(ctx, "o", "r", 2); err != nil {
		t.Fatal(err)
	}
	if got := client.calls.Load(); got != 2 {
		t.Fatalf("expected new fetch for different number, got %d", got)
	}
}

// blockingFeedbackClient blocks inside GetPRFeedback until released, so the
// test can force many concurrent in-flight fetches for the same PR and assert
// they coalesce into one upstream call (singleflight). `started` is signaled
// the moment a goroutine enters the upstream call so the test can be certain
// at least one fetch is in flight before releasing siblings.
type blockingFeedbackClient struct {
	*MockClient
	calls   atomic.Int32
	release chan struct{}
	started chan struct{}
}

func (c *blockingFeedbackClient) GetPRFeedback(_ context.Context, owner, repo string, number int) (*PRFeedback, error) {
	c.calls.Add(1)
	// Non-blocking signal so only the FIRST entrant fires `started`; later
	// arrivals queue behind the singleflight without re-signaling.
	select {
	case c.started <- struct{}{}:
	default:
	}
	<-c.release
	return &PRFeedback{PR: &PR{RepoOwner: owner, RepoName: repo, Number: number}}, nil
}

// Concurrent feedback fetches for the same PR must coalesce to a single
// upstream call. This reproduces the production pile-up: the task page fired ~8
// identical GetPRFeedback requests concurrently, and without singleflight each
// independently hammered GitHub, ballooning durations to 40-73s.
func TestService_GetPRFeedback_CoalescesConcurrentCalls(t *testing.T) {
	client := &blockingFeedbackClient{
		MockClient: NewMockClient(),
		release:    make(chan struct{}),
		started:    make(chan struct{}, 1),
	}
	svc := newTestService(client)

	const n = 16
	var done sync.WaitGroup
	done.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer done.Done()
			if _, err := svc.GetPRFeedback(context.Background(), "o", "r", 1); err != nil {
				t.Errorf("concurrent fetch: %v", err)
			}
		}()
	}
	// Wait for the leader to actually enter the upstream call (not just be
	// scheduled). Pre-fix this used launched.Wait, which only guaranteed the
	// goroutines had STARTED — late arrivals after release could have seen
	// a warm cache and skipped the singleflight, masking regressions.
	<-client.started
	close(client.release)
	done.Wait()

	if got := client.calls.Load(); got != 1 {
		t.Fatalf("expected concurrent calls to coalesce to 1 upstream call, got %d", got)
	}
}
