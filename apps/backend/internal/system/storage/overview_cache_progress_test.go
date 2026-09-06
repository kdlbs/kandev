package storage

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestOverviewCacheReadStartsColdScanWithoutWaiting(t *testing.T) {
	provider := newProgressiveOverview()
	cache := NewOverviewCache(provider)

	started := time.Now()
	read, err := cache.Read(context.Background())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("cold Read took %s, want nonblocking response", elapsed)
	}
	if read.Snapshot != nil {
		t.Fatalf("cold snapshot = %#v, want nil", read.Snapshot)
	}
	if read.Analysis.State != AnalysisStateScanning {
		t.Fatalf("cold analysis state = %q, want scanning", read.Analysis.State)
	}
	if read.Analysis.Progress.TotalSources != len(storageAnalysisSources) {
		t.Fatalf("total sources = %d, want %d", read.Analysis.Progress.TotalSources, len(storageAnalysisSources))
	}
	<-provider.started

	joined, err := cache.Read(context.Background())
	if err != nil {
		t.Fatalf("joined Read: %v", err)
	}
	if joined.Analysis.Generation != read.Analysis.Generation {
		t.Fatalf("joined generation = %d, want %d", joined.Analysis.Generation, read.Analysis.Generation)
	}
	if provider.calls != 1 {
		t.Fatalf("progressive Summary calls = %d, want 1", provider.calls)
	}

	close(provider.release)
	waitForOverviewState(t, cache, AnalysisStateReady)
	complete, err := cache.Read(context.Background())
	if err != nil {
		t.Fatalf("completed Read: %v", err)
	}
	if complete.Snapshot == nil || complete.Snapshot.Summary.Workspaces != 42 {
		t.Fatalf("completed snapshot = %#v, want workspaces 42", complete.Snapshot)
	}
}

func TestOverviewCachePreservesSourceFailureOnSuccessfulScan(t *testing.T) {
	provider := newProgressiveOverview()
	provider.setSourceFailure(errors.New("go cache unavailable"))
	cache := NewOverviewCache(provider)

	if _, err := cache.Read(context.Background()); err != nil {
		t.Fatalf("Read: %v", err)
	}
	<-provider.started
	close(provider.release)
	read := waitForOverviewState(t, cache, AnalysisStateReady)

	progress, ok := read.Analysis.Progress.Sources[StorageSourceGoCache]
	if !ok {
		t.Fatalf("source progress does not include %q", StorageSourceGoCache)
	}
	if progress.State != SourceStateFailed {
		t.Fatalf("Go-cache source state = %q, want failed", progress.State)
	}
	if progress.Error == nil || *progress.Error != "go cache unavailable" {
		t.Fatalf("Go-cache source error = %v, want provider error", progress.Error)
	}
}

func TestOverviewCachePreservesCountersOnTerminalProgress(t *testing.T) {
	cache := NewOverviewCache(terminalProgressOverview{})
	if _, err := cache.Get(context.Background()); err != nil {
		t.Fatalf("Get: %v", err)
	}
	read, err := cache.Read(context.Background())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	progress := read.Analysis.Progress.Sources[StorageSourceWorkspaces]
	if progress.CompletedItems != 7 || progress.BytesScanned != 42 {
		t.Fatalf("terminal progress = %#v, want counters preserved", progress)
	}
	if progress.TotalItems == nil || *progress.TotalItems != 9 {
		t.Fatalf("terminal total_items = %v, want 9", progress.TotalItems)
	}
}

func TestOverviewCacheReadKeepsExpiredSnapshotWhileRefreshing(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	provider := newProgressiveOverview()
	provider.summary = Summary{Workspaces: 1}
	cache := newOverviewCacheForTest(provider, func() time.Time { return now })
	close(provider.release)
	first, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("initial Get: %v", err)
	}

	provider.setNextSummary(Summary{Workspaces: 2})
	now = now.Add(defaultOverviewCacheTTL)
	read, err := cache.Read(context.Background())
	if err != nil {
		t.Fatalf("expired Read: %v", err)
	}
	if read.Snapshot == nil || read.Snapshot.Summary.Workspaces != first.Summary.Workspaces {
		t.Fatalf("expired snapshot = %#v, want previous snapshot", read.Snapshot)
	}
	if !read.Analysis.Stale || read.Analysis.State != AnalysisStateScanning {
		t.Fatalf("expired analysis = %#v, want stale scanning", read.Analysis)
	}
	<-provider.nextStarted
	if provider.calls != 2 {
		t.Fatalf("refresh calls = %d, want 2", provider.calls)
	}

	close(provider.nextRelease)
	waitForOverviewState(t, cache, AnalysisStateReady)
	refreshed, err := cache.Read(context.Background())
	if err != nil {
		t.Fatalf("refreshed Read: %v", err)
	}
	if refreshed.Snapshot == nil || refreshed.Snapshot.Summary.Workspaces != 2 {
		t.Fatalf("refreshed snapshot = %#v, want workspaces 2", refreshed.Snapshot)
	}
}

func TestOverviewCacheReadFailurePreservesSnapshotAndReportsAttempt(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	provider := newProgressiveOverview()
	provider.summary = Summary{Workspaces: 7}
	cache := newOverviewCacheForTest(provider, func() time.Time { return now })
	close(provider.release)
	first, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("initial Get: %v", err)
	}

	provider.setNextError(errors.New("scan failed"))
	now = now.Add(defaultOverviewCacheTTL)
	read, err := cache.Read(context.Background())
	if err != nil {
		t.Fatalf("failed refresh Read: %v", err)
	}
	<-provider.nextStarted
	close(provider.nextRelease)
	waitForOverviewState(t, cache, AnalysisStateFailed)

	failed, err := cache.Read(context.Background())
	if err != nil {
		t.Fatalf("failed state Read: %v", err)
	}
	if failed.Snapshot == nil || failed.Snapshot.Summary.Workspaces != first.Summary.Workspaces {
		t.Fatalf("failed snapshot = %#v, want previous snapshot", failed.Snapshot)
	}
	if !failed.Analysis.Stale || failed.Analysis.Error == nil || *failed.Analysis.Error != "scan failed" {
		t.Fatalf("failed analysis = %#v, want stale error", failed.Analysis)
	}
	if !failed.Snapshot.AnalyzedAt.Equal(first.AnalyzedAt) {
		t.Fatalf("failed analyzed_at = %s, want %s", failed.Snapshot.AnalyzedAt, first.AnalyzedAt)
	}
	_ = read
}

func TestOverviewCacheInvalidationRejectsLateProgressAndCompletion(t *testing.T) {
	provider := newProgressiveOverview()
	provider.summary = Summary{Workspaces: 1}
	cache := NewOverviewCache(provider)
	first, err := cache.Read(context.Background())
	if err != nil {
		t.Fatalf("initial Read: %v", err)
	}
	<-provider.started

	cache.Invalidate()
	provider.setNextSummary(Summary{Workspaces: 2})
	second, err := cache.Read(context.Background())
	if err != nil {
		t.Fatalf("second Read: %v", err)
	}
	if second.Analysis.Generation <= first.Analysis.Generation {
		t.Fatalf("second generation = %d, want newer than %d", second.Analysis.Generation, first.Analysis.Generation)
	}
	<-provider.nextStarted

	close(provider.release)
	close(provider.nextRelease)
	waitForOverviewState(t, cache, AnalysisStateReady)
	current, err := cache.Read(context.Background())
	if err != nil {
		t.Fatalf("current Read: %v", err)
	}
	if current.Snapshot == nil || current.Snapshot.Summary.Workspaces != 2 {
		t.Fatalf("current snapshot = %#v, want newer completion", current.Snapshot)
	}
}

func waitForOverviewState(t *testing.T, cache *OverviewCache, want AnalysisStateName) OverviewRead {
	t.Helper()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for {
		read, err := cache.Read(context.Background())
		if err != nil {
			t.Fatalf("poll Read: %v", err)
		}
		if read.Analysis.State == want {
			return read
		}
		select {
		case <-deadline.C:
			t.Fatalf("analysis state = %q, want %q", read.Analysis.State, want)
		default:
		}
	}
}

type progressiveOverview struct {
	mu sync.Mutex

	calls         int
	summary       Summary
	next          Summary
	nextErr       error
	hasNext       bool
	started       chan struct{}
	nextStarted   chan struct{}
	release       chan struct{}
	nextRelease   chan struct{}
	sourceFailure error
}

func newProgressiveOverview() *progressiveOverview {
	return &progressiveOverview{
		summary:     Summary{Workspaces: 42},
		started:     make(chan struct{}),
		nextStarted: make(chan struct{}),
		release:     make(chan struct{}),
		nextRelease: make(chan struct{}),
	}
}

func (o *progressiveOverview) Summary(ctx context.Context) (Summary, error) {
	return o.SummaryWithProgress(ctx, nil)
}

func (o *progressiveOverview) SummaryWithProgress(
	ctx context.Context,
	notify OverviewProgressCallback,
) (Summary, error) {
	o.mu.Lock()
	o.calls++
	call := o.calls
	summary, err := o.summary, error(nil)
	release := o.release
	if o.hasNext {
		summary, err, release = o.next, o.nextErr, o.nextRelease
		o.hasNext = false
	}
	if call == 1 {
		close(o.started)
	} else {
		close(o.nextStarted)
	}
	o.mu.Unlock()
	if notify != nil {
		notify(OverviewProgress{Source: StorageSourceWorkspaces, State: SourceStateScanning})
	}
	select {
	case <-release:
	case <-ctx.Done():
		return Summary{}, ctx.Err()
	}
	if err != nil {
		return Summary{}, err
	}
	if notify != nil {
		o.mu.Lock()
		sourceFailure := o.sourceFailure
		o.mu.Unlock()
		if sourceFailure != nil {
			notify(OverviewProgress{
				Source: StorageSourceGoCache, State: SourceStateFailed, Err: sourceFailure,
			})
		}
		notify(OverviewProgress{
			Source: StorageSourceWorkspaces, State: SourceStateReady,
			CompletedItems: 1, TotalItems: intPointer(1), Value: summary.Workspaces,
		})
	}
	return summary, nil
}

func (o *progressiveOverview) Capabilities(context.Context, StorageMaintenanceSettings) Capabilities {
	return Capabilities{}
}

func (o *progressiveOverview) SettingsCapabilities(
	context.Context,
	StorageMaintenanceSettings,
) Capabilities {
	return Capabilities{}
}

func (o *progressiveOverview) setNextSummary(summary Summary) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.next, o.nextErr, o.hasNext = summary, nil, true
}

func (o *progressiveOverview) setNextError(err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.next, o.nextErr, o.hasNext = Summary{}, err, true
}

func (o *progressiveOverview) setSourceFailure(err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sourceFailure = err
}

func intPointer(value int) *int {
	return &value
}

type terminalProgressOverview struct{}

func (terminalProgressOverview) Summary(context.Context) (Summary, error) {
	return Summary{Workspaces: 1}, nil
}

func (terminalProgressOverview) SummaryWithProgress(
	_ context.Context,
	notify OverviewProgressCallback,
) (Summary, error) {
	notify(OverviewProgress{
		Source: StorageSourceWorkspaces, State: SourceStateScanning,
		CompletedItems: 7, TotalItems: intPointer(9), BytesScanned: 42,
	})
	notify(OverviewProgress{Source: StorageSourceWorkspaces, State: SourceStateReady, Value: 1})
	return Summary{Workspaces: 1}, nil
}

func (terminalProgressOverview) Capabilities(context.Context, StorageMaintenanceSettings) Capabilities {
	return Capabilities{}
}

func (terminalProgressOverview) SettingsCapabilities(
	context.Context,
	StorageMaintenanceSettings,
) Capabilities {
	return Capabilities{}
}
