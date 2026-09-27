package database

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLogicalStatsCacheSharesColdScanAcrossReads(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	var calls atomic.Int32
	cache := newLogicalStatsCache(context.Background(), func(context.Context) (logicalStorageStats, error) {
		calls.Add(1)
		close(started)
		<-release
		return logicalStorageStats{messageContent: 7}, nil
	}, logicalStatsCacheOptions{})
	t.Cleanup(cache.Close)

	first := cache.Read()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("cold scan did not start")
	}
	second := cache.Read()
	if first.state != "pending" || second.state != "pending" {
		t.Fatalf("cold reads = %q and %q, want pending", first.state, second.state)
	}
	if calls.Load() != 1 {
		t.Fatalf("scan calls = %d, want one shared scan", calls.Load())
	}
	cache.mu.Lock()
	flight := cache.flight
	cache.mu.Unlock()
	releaseOnce.Do(func() { close(release) })
	select {
	case <-flight.done:
	case <-time.After(time.Second):
		t.Fatal("shared scan did not finish")
	}
	ready := cache.Read()
	if ready.state != "ready" || ready.snapshot == nil || ready.snapshot.values.messageContent != 7 {
		t.Fatalf("read after cold scan = %#v, want ready snapshot", ready)
	}
}

func TestLogicalStatsCacheKeepsStaleSnapshotThroughFailureAndBackoff(t *testing.T) {
	clock := newTestClock(time.Date(2026, time.September, 27, 12, 0, 0, 0, time.UTC))
	var calls atomic.Int32
	cache := newLogicalStatsCache(context.Background(), func(context.Context) (logicalStorageStats, error) {
		switch calls.Add(1) {
		case 1:
			return logicalStorageStats{messageContent: 10}, nil
		case 2:
			return logicalStorageStats{}, errors.New("database busy")
		default:
			return logicalStorageStats{messageContent: 20}, nil
		}
	}, logicalStatsCacheOptions{
		ttl: 10 * time.Minute, retryMin: 5 * time.Minute, retryMax: 10 * time.Minute,
		now: clock.Now,
	})
	t.Cleanup(cache.Close)

	cache.Read()
	waitLogicalStatsCache(t, cache)
	ready := cache.Read()
	if ready.state != "ready" || ready.snapshot.values.messageContent != 10 {
		t.Fatalf("initial read = %#v, want ready snapshot", ready)
	}

	clock.Advance(10 * time.Minute)
	refreshing := cache.Read()
	if refreshing.state != "refreshing" || refreshing.snapshot == nil || refreshing.snapshot.values.messageContent != 10 {
		t.Fatalf("expired read = %#v, want refreshing with last good values", refreshing)
	}
	waitLogicalStatsCache(t, cache)
	stale := cache.Read()
	if stale.state != "stale" || stale.snapshot == nil || stale.snapshot.values.messageContent != 10 || stale.error != logicalStatsErrorScanFailed {
		t.Fatalf("read after failed refresh = %#v, want stale last-good snapshot and stable error", stale)
	}
	cache.Read()
	if calls.Load() != 2 {
		t.Fatalf("scan calls during backoff = %d, want 2", calls.Load())
	}

	clock.Advance(5 * time.Minute)
	cache.Read()
	waitLogicalStatsCache(t, cache)
	recovered := cache.Read()
	if recovered.state != "ready" || recovered.snapshot == nil || recovered.snapshot.values.messageContent != 20 || recovered.error != "" {
		t.Fatalf("read after recovery = %#v, want new ready snapshot", recovered)
	}
}

func TestLogicalStatsCacheExplicitRetryBypassesBackoff(t *testing.T) {
	clock := newTestClock(time.Date(2026, time.September, 27, 12, 0, 0, 0, time.UTC))
	retryStarted := make(chan struct{})
	releaseRetry := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(releaseRetry) })
	var calls atomic.Int32
	cache := newLogicalStatsCache(context.Background(), func(context.Context) (logicalStorageStats, error) {
		switch calls.Add(1) {
		case 1:
			return logicalStorageStats{}, errors.New("database busy")
		case 2:
			close(retryStarted)
			<-releaseRetry
			return logicalStorageStats{messageContent: 23}, nil
		default:
			return logicalStorageStats{}, nil
		}
	}, logicalStatsCacheOptions{
		retryMin: 5 * time.Minute,
		retryMax: 10 * time.Minute,
		now:      clock.Now,
	})
	t.Cleanup(cache.Close)

	cache.Read()
	waitLogicalStatsCache(t, cache)
	if got := cache.Read(); got.state != "unavailable" || got.error != logicalStatsErrorScanFailed {
		t.Fatalf("read after failed scan = %#v, want unavailable", got)
	}
	cache.Read()
	if calls.Load() != 1 {
		t.Fatalf("automatic reads during backoff started %d scans, want one", calls.Load())
	}
	if !cache.Retry() {
		t.Fatal("explicit retry was rejected")
	}
	select {
	case <-retryStarted:
	case <-time.After(time.Second):
		t.Fatal("explicit retry did not start a scan during backoff")
	}
	if got := cache.Read(); got.state != "pending" {
		t.Fatalf("read during retry = %#v, want pending", got)
	}

	releaseOnce.Do(func() { close(releaseRetry) })
	waitLogicalStatsCache(t, cache)
	if got := cache.Read(); got.state != "ready" || got.snapshot == nil || got.snapshot.values.messageContent != 23 {
		t.Fatalf("read after explicit retry = %#v, want ready snapshot", got)
	}
}

func TestLogicalStatsCacheDoesNotOverlapAnInvalidatedScan(t *testing.T) {
	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	releaseSecond := make(chan struct{})
	var firstReleaseOnce, secondReleaseOnce atomic.Bool
	release := func(ch chan struct{}, once *atomic.Bool) {
		if once.CompareAndSwap(false, true) {
			close(ch)
		}
	}
	defer release(releaseFirst, &firstReleaseOnce)
	defer release(releaseSecond, &secondReleaseOnce)

	var calls atomic.Int32
	cache := newLogicalStatsCache(context.Background(), func(context.Context) (logicalStorageStats, error) {
		call := calls.Add(1)
		switch call {
		case 1:
			close(firstStarted)
			<-releaseFirst
			return logicalStorageStats{messageContent: 1}, nil
		case 2:
			close(secondStarted)
			<-releaseSecond
			return logicalStorageStats{messageContent: 2}, nil
		default:
			return logicalStorageStats{}, nil
		}
	}, logicalStatsCacheOptions{})
	t.Cleanup(cache.Close)

	cache.Read()
	var firstFlight *logicalStatsFlight
	select {
	case <-firstStarted:
		cache.mu.Lock()
		firstFlight = cache.flight
		cache.mu.Unlock()
	case <-time.After(time.Second):
		t.Fatal("first scan did not start")
	}

	cache.Invalidate(true)
	cache.Read()
	select {
	case <-secondStarted:
		t.Fatal("replacement scan overlapped the invalidated scan")
	case <-time.After(50 * time.Millisecond):
	}

	release(releaseFirst, &firstReleaseOnce)
	select {
	case <-firstFlight.done:
	case <-time.After(time.Second):
		t.Fatal("invalidated scan did not finish")
	}
	cache.Read()
	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("replacement scan did not start after the old scan finished")
	}
	release(releaseSecond, &secondReleaseOnce)
	cache.mu.Lock()
	secondFlight := cache.flight
	cache.mu.Unlock()
	select {
	case <-secondFlight.done:
	case <-time.After(time.Second):
		t.Fatal("replacement scan did not finish")
	}
	read := cache.Read()
	if read.state != "ready" || read.snapshot == nil || read.snapshot.values.messageContent != 2 {
		t.Fatalf("read after replacement = %#v, want the second completed snapshot", read)
	}
}

func waitLogicalStatsCache(t *testing.T, cache *logicalStatsCache) {
	t.Helper()
	cache.mu.Lock()
	flight := cache.flight
	finished := flight == nil && (cache.snapshot != nil || cache.errorCode != "")
	cache.mu.Unlock()
	if finished {
		return
	}
	if flight == nil {
		t.Fatal("logical stats scan did not start")
	}
	select {
	case <-flight.done:
	case <-time.After(time.Second):
		t.Fatal("logical stats scan did not finish")
	}
}

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock(now time.Time) *testClock { return &testClock{now: now} }

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(duration time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(duration)
	c.mu.Unlock()
}
