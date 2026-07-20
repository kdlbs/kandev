package process

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func TestWorkspaceTrackerConcurrentFreshStatusSharesObservation(t *testing.T) {
	wt := newStatusConcurrencyTracker(t)
	started := make(chan struct{})
	release := make(chan struct{})
	want := types.GitStatusUpdate{Timestamp: time.Unix(123, 0), Branch: "feature"}
	const callers = 6
	var observations atomic.Int32
	wt.gitStatusObserver = func(context.Context) (types.GitStatusUpdate, error) {
		if observations.Add(1) == 1 {
			close(started)
		}
		<-release
		return want, nil
	}
	joined := make(chan struct{}, callers)
	wt.gitStatusWaiterJoined = func() { joined <- struct{}{} }

	ready := make(chan struct{}, callers)
	results := make(chan types.GitStatusUpdate, callers)
	errs := make(chan error, callers)
	launch := make(chan struct{})
	for range callers {
		go func() {
			ready <- struct{}{}
			<-launch
			status, err := wt.GetGitStatus(context.Background(), true)
			results <- status
			errs <- err
		}()
	}
	for range callers {
		<-ready
	}
	close(launch)
	waitForSignal(t, started, "first status observation")
	waitForSignals(t, joined, callers, "fresh status waiters")
	close(release)

	for range callers {
		if err := <-errs; err != nil {
			t.Fatalf("GetGitStatus returned error: %v", err)
		}
		if got := <-results; got.Timestamp != want.Timestamp || got.Branch != want.Branch {
			t.Fatalf("GetGitStatus = %+v, want shared result %+v", got, want)
		}
	}
	if got := observations.Load(); got != 1 {
		t.Fatalf("live observations = %d, want 1", got)
	}
}

func TestWorkspaceTrackerStatusWaiterCancellationDoesNotCancelPeers(t *testing.T) {
	wt := newStatusConcurrencyTracker(t)
	started := make(chan struct{})
	release := make(chan struct{})
	observerCanceled := make(chan struct{})
	want := types.GitStatusUpdate{Timestamp: time.Unix(456, 0), Branch: "peer-result"}
	wt.gitStatusObserver = func(ctx context.Context) (types.GitStatusUpdate, error) {
		close(started)
		select {
		case <-release:
			return want, nil
		case <-ctx.Done():
			close(observerCanceled)
			return types.GitStatusUpdate{}, ctx.Err()
		}
	}
	joined := make(chan struct{}, 2)
	wt.gitStatusWaiterJoined = func() { joined <- struct{}{} }

	peerResult := make(chan types.GitStatusUpdate, 1)
	peerErr := make(chan error, 1)
	go func() {
		status, err := wt.GetGitStatus(context.Background(), true)
		peerResult <- status
		peerErr <- err
	}()
	waitForSignal(t, started, "shared status observation")
	waitForSignal(t, joined, "peer waiter admission")

	waiterCtx, cancelWaiter := context.WithCancel(context.Background())
	waiterErr := make(chan error, 1)
	go func() {
		_, err := wt.GetGitStatus(waiterCtx, true)
		waiterErr <- err
	}()
	waitForSignal(t, joined, "canceling waiter admission")
	cancelWaiter()
	err := <-waiterErr
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter error = %v, want context.Canceled", err)
	}
	select {
	case <-observerCanceled:
		t.Fatal("canceling one waiter canceled the shared observation")
	default:
	}

	close(release)
	if err := <-peerErr; err != nil {
		t.Fatalf("peer waiter returned error: %v", err)
	}
	if got := <-peerResult; got.Timestamp != want.Timestamp {
		t.Fatalf("peer timestamp = %v, want %v", got.Timestamp, want.Timestamp)
	}
}

func TestWorkspaceTrackerCanceledCallerStartsNoObservation(t *testing.T) {
	wt := newStatusConcurrencyTracker(t)
	var observations atomic.Int32
	wt.gitStatusObserver = func(context.Context) (types.GitStatusUpdate, error) {
		observations.Add(1)
		return types.GitStatusUpdate{}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := wt.GetGitStatus(ctx, true)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetGitStatus error = %v, want context.Canceled", err)
	}
	var probeCalls atomic.Int32
	_, _, _ = wt.gitStatusGroup.Do("live", func() (interface{}, error) {
		probeCalls.Add(1)
		return types.GitStatusUpdate{}, nil
	})
	if got := observations.Load(); got != 0 {
		t.Fatalf("live observations = %d, want 0", got)
	}
	if got := probeCalls.Load(); got != 1 {
		t.Fatalf("probe observations = %d, want 1 independent flight", got)
	}
}

func TestWorkspaceTrackerRefreshAndEmptyCacheReadShareObservation(t *testing.T) {
	wt := newStatusConcurrencyTracker(t)
	started := make(chan struct{})
	release := make(chan struct{})
	want := types.GitStatusUpdate{Timestamp: time.Unix(654, 0), Branch: "shared-refresh"}
	var observations atomic.Int32
	wt.gitStatusObserver = func(context.Context) (types.GitStatusUpdate, error) {
		if observations.Add(1) == 1 {
			close(started)
		}
		<-release
		return want, nil
	}
	joined := make(chan struct{}, 2)
	wt.gitStatusWaiterJoined = func() { joined <- struct{}{} }

	refreshDone := make(chan struct{})
	go func() {
		wt.RefreshGitStatus(context.Background())
		close(refreshDone)
	}()
	waitForSignal(t, started, "refresh observation")

	readResult := make(chan types.GitStatusUpdate, 1)
	readErr := make(chan error, 1)
	go func() {
		status, err := wt.GetGitStatus(context.Background(), false)
		readResult <- status
		readErr <- err
	}()
	waitForSignals(t, joined, 2, "refresh and empty-cache waiters")
	close(release)
	waitForSignal(t, refreshDone, "refresh completion")

	if err := <-readErr; err != nil {
		t.Fatalf("empty-cache read returned error: %v", err)
	}
	if got := <-readResult; got.Timestamp != want.Timestamp {
		t.Fatalf("empty-cache timestamp = %v, want %v", got.Timestamp, want.Timestamp)
	}
	if got := observations.Load(); got != 1 {
		t.Fatalf("live observations = %d, want 1", got)
	}
	wt.mu.RLock()
	cached := wt.currentStatus
	wt.mu.RUnlock()
	if cached.Timestamp != want.Timestamp {
		t.Fatalf("refresh cached timestamp = %v, want %v", cached.Timestamp, want.Timestamp)
	}
}

func TestWorkspaceTrackerDistinctRepositoriesObserveInParallel(t *testing.T) {
	first := newStatusConcurrencyTracker(t)
	second := newStatusConcurrencyTracker(t)
	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	release := make(chan struct{})
	first.gitStatusObserver = blockingStatusObserver(firstStarted, release, "first")
	second.gitStatusObserver = blockingStatusObserver(secondStarted, release, "second")

	var wg sync.WaitGroup
	for _, tracker := range []*WorkspaceTracker{first, second} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := tracker.GetGitStatus(context.Background(), true); err != nil {
				t.Errorf("GetGitStatus returned error: %v", err)
			}
		}()
	}
	waitForSignal(t, firstStarted, "first repository observation")
	waitForSignal(t, secondStarted, "second repository observation")
	close(release)
	wg.Wait()
}

func TestWorkspaceTrackerStopCancelsSharedObservationWithoutCaching(t *testing.T) {
	wt := newStatusConcurrencyTracker(t)
	started := make(chan struct{})
	wt.gitStatusObserver = func(ctx context.Context) (types.GitStatusUpdate, error) {
		close(started)
		<-ctx.Done()
		return types.GitStatusUpdate{Timestamp: time.Unix(789, 0), Branch: "partial"}, nil
	}

	done := make(chan struct{})
	go func() {
		wt.RefreshGitStatus(context.Background())
		close(done)
	}()
	waitForSignal(t, started, "shared status observation")
	wt.Stop()
	waitForSignal(t, done, "refresh cancellation")

	wt.mu.RLock()
	cached := wt.currentStatus
	wt.mu.RUnlock()
	if !cached.Timestamp.IsZero() {
		t.Fatalf("canceled observation cached partial status: %+v", cached)
	}
}

func TestWorkspaceTrackerStopWaitsForSharedObservationExit(t *testing.T) {
	wt := newStatusConcurrencyTracker(t)
	started := make(chan struct{})
	canceled := make(chan struct{})
	allowExit := make(chan struct{})
	wt.gitStatusObserver = func(ctx context.Context) (types.GitStatusUpdate, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		<-allowExit
		return types.GitStatusUpdate{}, ctx.Err()
	}

	refreshDone := make(chan struct{})
	go func() {
		wt.RefreshGitStatus(context.Background())
		close(refreshDone)
	}()
	waitForSignal(t, started, "shared status observation")

	stopDone := make(chan struct{})
	go func() {
		wt.Stop()
		close(stopDone)
	}()
	waitForSignal(t, canceled, "tracker cancellation")
	select {
	case <-stopDone:
		t.Fatal("Stop returned before the shared observation exited")
	default:
	}

	close(allowExit)
	waitForSignal(t, stopDone, "tracker stop")
	waitForSignal(t, refreshDone, "refresh exit")
}

func TestWorkspaceTrackerSharedObservationDeadlineDoesNotCachePartialStatus(t *testing.T) {
	wt := newStatusConcurrencyTracker(t)
	wt.gitStatusObserveTimeout = 0
	wt.gitStatusObserver = func(ctx context.Context) (types.GitStatusUpdate, error) {
		<-ctx.Done()
		return types.GitStatusUpdate{Timestamp: time.Unix(987, 0), Branch: "partial"}, nil
	}

	_, err := wt.GetGitStatus(context.Background(), true)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("GetGitStatus error = %v, want context.DeadlineExceeded", err)
	}

	wt.mu.RLock()
	cached := wt.currentStatus
	wt.mu.RUnlock()
	if !cached.Timestamp.IsZero() {
		t.Fatalf("timed-out observation cached partial status: %+v", cached)
	}
}

func TestWorkspaceTrackerZeroValueStatusRead(t *testing.T) {
	var wt WorkspaceTracker

	status, err := wt.GetGitStatus(context.Background(), true)
	if err != nil {
		t.Fatalf("zero-value GetGitStatus returned error: %v", err)
	}
	if status.Timestamp.IsZero() {
		t.Fatal("zero-value GetGitStatus returned an empty observation")
	}
}

func TestWorkspaceTrackerCachedStatusDoesNotObserveRepository(t *testing.T) {
	wt := newStatusConcurrencyTracker(t)
	want := types.GitStatusUpdate{Timestamp: time.Unix(246, 0), Branch: "cached"}
	wt.currentStatus = want
	wt.gitStatusObserver = func(context.Context) (types.GitStatusUpdate, error) {
		t.Fatal("cached status started a live observation")
		return types.GitStatusUpdate{}, nil
	}

	got, err := wt.GetGitStatus(context.Background(), false)
	if err != nil {
		t.Fatalf("GetGitStatus returned error: %v", err)
	}
	if got.Timestamp != want.Timestamp || got.Branch != want.Branch {
		t.Fatalf("GetGitStatus = %+v, want cached status %+v", got, want)
	}
}

func newStatusConcurrencyTracker(t *testing.T) *WorkspaceTracker {
	t.Helper()
	wt := NewWorkspaceTracker(t.TempDir(), newTestLogger(t))
	t.Cleanup(wt.Stop)
	return wt
}

func blockingStatusObserver(started chan struct{}, release <-chan struct{}, branch string) func(context.Context) (types.GitStatusUpdate, error) {
	return func(context.Context) (types.GitStatusUpdate, error) {
		close(started)
		<-release
		return types.GitStatusUpdate{Timestamp: time.Unix(1, 0), Branch: branch}, nil
	}
}

func waitForSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func waitForSignals(t *testing.T, signals <-chan struct{}, count int, description string) {
	t.Helper()
	for range count {
		waitForSignal(t, signals, description)
	}
}
