package storage

import (
	"context"
	"sync"
	"time"
)

const (
	defaultOverviewCacheTTL = 15 * time.Minute
	progressPublishInterval = 250 * time.Millisecond
)

// OverviewSnapshot is a successful storage overview and the time it was measured.
type OverviewSnapshot struct {
	Summary    Summary
	AnalyzedAt time.Time
}

type OverviewInvalidator interface {
	Invalidate()
}

// OverviewCache keeps one process-local successful overview snapshot and its active scan state.
type OverviewCache struct {
	provider  OverviewProvider
	ttl       time.Duration
	now       func() time.Time
	publisher func(StorageAnalysisUpdated)
	observer  func(StorageAnalysisCompletion)

	mu         sync.Mutex
	snapshot   *OverviewSnapshot
	flight     *overviewFlight
	analysis   StorageAnalysisState
	generation uint64
}

type overviewFlight struct {
	done             chan struct{}
	snapshot         OverviewSnapshot
	err              error
	generation       uint64
	startedAt        time.Time
	partialValues    map[string]any
	sourceStarted    map[string]time.Time
	sourceDurations  map[string]time.Duration
	sourceStates     map[string]SourceStateName
	partitionCount   int
	maxActiveWalkers int
	lastPublished    time.Time
}

func NewOverviewCache(provider OverviewProvider) *OverviewCache {
	return NewOverviewCacheWithOptions(provider, OverviewCacheOptions{})
}

func NewOverviewCacheWithPublisher(
	provider OverviewProvider,
	publisher func(StorageAnalysisUpdated),
) *OverviewCache {
	return NewOverviewCacheWithOptions(provider, OverviewCacheOptions{Publisher: publisher})
}

func NewOverviewCacheWithOptions(provider OverviewProvider, options OverviewCacheOptions) *OverviewCache {
	cache := &OverviewCache{
		provider: provider, ttl: defaultOverviewCacheTTL, now: time.Now,
		publisher: options.Publisher, observer: options.Observer,
	}
	cache.analysis = newAnalysisState(0, cache.ttl)
	return cache
}

// Read returns the latest snapshot and starts a missing or expired scan in the background.
func (c *OverviewCache) Read(ctx context.Context) (OverviewRead, error) {
	if err := ctx.Err(); err != nil {
		return OverviewRead{}, err
	}
	c.mu.Lock()
	var flight *overviewFlight
	if !c.snapshotFreshLocked() && c.flight == nil && c.shouldStartBackgroundLocked() {
		flight = c.startRefreshLocked()
	}
	if flight == nil && !c.snapshotFreshLocked() && c.flight != nil {
		recordStorageAnalysisJoined()
	}
	read := c.readLocked()
	c.mu.Unlock()
	if flight != nil {
		c.publishUpdate(flight.generation, AnalysisStateScanning)
		go c.refresh(context.WithoutCancel(ctx), flight)
	}
	return read, nil
}

func (c *OverviewCache) Get(ctx context.Context) (OverviewSnapshot, error) {
	c.mu.Lock()
	if c.snapshotFreshLocked() {
		snapshot := *c.snapshot
		c.mu.Unlock()
		return snapshot, nil
	}
	if c.flight != nil {
		flight := c.flight
		recordStorageAnalysisJoined()
		c.mu.Unlock()
		return waitForOverviewRefresh(ctx, flight)
	}
	flight := c.startRefreshLocked()
	c.mu.Unlock()
	c.publishUpdate(flight.generation, AnalysisStateScanning)
	go c.refresh(context.WithoutCancel(ctx), flight)
	return waitForOverviewRefresh(ctx, flight)
}

// Refresh bypasses the freshness window and replaces the shared snapshot on success.
func (c *OverviewCache) Refresh(ctx context.Context) (OverviewSnapshot, error) {
	c.mu.Lock()
	if c.flight != nil {
		flight := c.flight
		recordStorageAnalysisJoined()
		c.mu.Unlock()
		return waitForOverviewRefresh(ctx, flight)
	}
	flight := c.startRefreshLocked()
	c.mu.Unlock()
	c.publishUpdate(flight.generation, AnalysisStateScanning)
	c.refresh(ctx, flight)
	return waitForOverviewRefresh(ctx, flight)
}

func (c *OverviewCache) Capabilities(ctx context.Context, settings StorageMaintenanceSettings) Capabilities {
	return c.provider.Capabilities(ctx, settings)
}

func (c *OverviewCache) SettingsCapabilities(
	ctx context.Context,
	settings StorageMaintenanceSettings,
) Capabilities {
	return c.provider.SettingsCapabilities(ctx, settings)
}

func (c *OverviewCache) Invalidate() {
	c.mu.Lock()
	c.generation++
	c.snapshot = nil
	c.flight = nil
	c.analysis = newAnalysisState(c.generation, c.ttl)
	c.mu.Unlock()
}

func (c *OverviewCache) startRefreshLocked() *overviewFlight {
	c.generation++
	startedAt := c.now().UTC()
	flight := &overviewFlight{
		done: make(chan struct{}), generation: c.generation, startedAt: startedAt,
		partialValues: make(map[string]any), sourceStarted: make(map[string]time.Time),
		sourceDurations: make(map[string]time.Duration), sourceStates: make(map[string]SourceStateName),
	}
	c.flight = flight
	c.analysis = scanningAnalysisState(c.generation, c.ttl, startedAt, c.snapshot)
	recordStorageAnalysisStarted()
	return flight
}

func (c *OverviewCache) shouldStartBackgroundLocked() bool {
	return c.analysis.State != AnalysisStateFailed || c.analysis.Generation != c.generation
}

func (c *OverviewCache) snapshotFreshLocked() bool {
	return c.snapshot != nil && c.now().Sub(c.snapshot.AnalyzedAt) < c.ttl
}

func (c *OverviewCache) readLocked() OverviewRead {
	var snapshot *OverviewSnapshot
	if c.snapshot != nil {
		copy := *c.snapshot
		snapshot = &copy
	}
	return OverviewRead{Snapshot: snapshot, Analysis: cloneAnalysisState(c.analysis)}
}

func (c *OverviewCache) refresh(ctx context.Context, flight *overviewFlight) {
	var (
		summary Summary
		err     error
	)
	if progressive, ok := c.provider.(ProgressiveOverviewProvider); ok {
		summary, err = progressive.SummaryWithProgress(ctx, func(progress OverviewProgress) {
			c.recordProgress(flight, progress)
		})
	} else {
		summary, err = c.provider.Summary(ctx)
		if err == nil {
			c.recordLegacyProgress(flight, summary)
		}
	}

	completedAt := c.now().UTC()
	c.mu.Lock()
	current := c.flight == flight && flight.generation == c.generation
	var completion StorageAnalysisCompletion
	if err == nil {
		flight.snapshot = OverviewSnapshot{Summary: summary, AnalyzedAt: completedAt}
		if current {
			progress := cloneAnalysisProgress(c.analysis.Progress)
			c.snapshot = &flight.snapshot
			c.analysis = readyAnalysisStateWithProgress(
				flight.generation, c.ttl, flight.startedAt, completedAt, flight.snapshot.AnalyzedAt,
				progress,
			)
		}
	} else if current {
		c.analysis = failedAnalysisState(
			c.analysis, flight.startedAt, completedAt, err, c.snapshot != nil,
		)
	}
	flight.err = err
	completion = StorageAnalysisCompletion{
		Generation:       flight.generation,
		Duration:         completedAt.Sub(flight.startedAt),
		SourceDurations:  cloneDurations(flight.sourceDurations),
		SourceStates:     cloneSourceStates(flight.sourceStates),
		PartitionCount:   flight.partitionCount,
		MaxActiveWalkers: flight.maxActiveWalkers,
		Succeeded:        err == nil,
	}
	if c.flight == flight {
		c.flight = nil
	}
	close(flight.done)
	c.mu.Unlock()
	recordStorageAnalysisCompleted(completion)
	if c.observer != nil {
		c.observer(completion)
	}

	if current {
		state := AnalysisStateReady
		if err != nil {
			state = AnalysisStateFailed
		}
		c.publishUpdate(flight.generation, state)
	}
}

func (c *OverviewCache) recordLegacyProgress(flight *overviewFlight, summary Summary) {
	for _, progress := range []OverviewProgress{
		{Source: StorageSourceWorkspaces, State: SourceStateReady, Value: summary.Workspaces},
		{Source: StorageSourceGoCache, State: SourceStateReady, Value: summary.GoCache},
		{Source: StorageSourceQuarantine, State: SourceStateReady, Value: summary.Quarantine},
		{Source: StorageSourceTemporaryArtifacts, State: SourceStateReady, Value: summary.TemporaryArtifacts},
		{Source: StorageSourceDocker, State: SourceStateReady, Value: summary.Docker},
	} {
		c.recordProgress(flight, progress)
	}
}

func (c *OverviewCache) recordProgress(flight *overviewFlight, progress OverviewProgress) {
	if progress.Source == "" {
		return
	}
	c.mu.Lock()
	if !c.isCurrentFlightLocked(flight) {
		c.mu.Unlock()
		return
	}
	source, ok := c.analysis.Progress.Sources[progress.Source]
	if !ok {
		c.mu.Unlock()
		return
	}
	if progress.State != "" {
		source.State = progress.State
	}
	now := c.now()
	c.updateSourceProgressLocked(flight, progress, &source, now)
	c.analysis.Progress.Sources[progress.Source] = source
	c.updateFlightProgressLocked(flight, progress, source, now)
	c.analysis.Progress.CompletedSources = completedSourceCount(c.analysis.Progress.Sources)
	if progress.Value != nil {
		flight.partialValues[progress.Source] = progress.Value
		c.analysis.PartialSummary = summaryFromSourceValues(flight.partialValues)
	}
	shouldPublish := c.shouldPublishProgressLocked(flight, source.State == SourceStateReady || source.State == SourceStateFailed, now)
	if shouldPublish {
		flight.lastPublished = now
	}
	generation := c.analysis.Generation
	c.mu.Unlock()
	if shouldPublish {
		c.publishUpdate(generation, AnalysisStateScanning)
	}
}

func (c *OverviewCache) isCurrentFlightLocked(flight *overviewFlight) bool {
	return c.flight == flight && flight.generation == c.generation
}

func (c *OverviewCache) updateSourceProgressLocked(
	flight *overviewFlight,
	progress OverviewProgress,
	source *StorageSourceProgress,
	now time.Time,
) {
	if source.State == SourceStateScanning {
		if _, started := flight.sourceStarted[progress.Source]; !started {
			flight.sourceStarted[progress.Source] = now
		}
	}
	source.CompletedItems = progress.CompletedItems
	if progress.TotalItems != nil {
		total := *progress.TotalItems
		source.TotalItems = &total
	}
	source.BytesScanned = progress.BytesScanned
	if progress.Err != nil {
		errorText := progress.Err.Error()
		source.Error = &errorText
		source.State = SourceStateFailed
	}
}

func (c *OverviewCache) updateFlightProgressLocked(
	flight *overviewFlight,
	progress OverviewProgress,
	source StorageSourceProgress,
	now time.Time,
) {
	if progress.PartitionCount > flight.partitionCount {
		flight.partitionCount = progress.PartitionCount
	}
	if progress.MaxActiveWalkers > flight.maxActiveWalkers {
		flight.maxActiveWalkers = progress.MaxActiveWalkers
	}
	if source.State != SourceStateReady && source.State != SourceStateFailed {
		return
	}
	flight.sourceStates[progress.Source] = source.State
	if progress.Duration > 0 {
		flight.sourceDurations[progress.Source] = progress.Duration
	} else if started, ok := flight.sourceStarted[progress.Source]; ok {
		flight.sourceDurations[progress.Source] = now.Sub(started)
	}
}

func (c *OverviewCache) shouldPublishProgressLocked(
	flight *overviewFlight,
	terminal bool,
	now time.Time,
) bool {
	return terminal || now.Sub(flight.lastPublished) >= progressPublishInterval
}

func (c *OverviewCache) publishUpdate(generation uint64, state AnalysisStateName) {
	if c.publisher != nil {
		c.publisher(StorageAnalysisUpdated{Generation: generation, State: state})
	}
}

func waitForOverviewRefresh(ctx context.Context, flight *overviewFlight) (OverviewSnapshot, error) {
	select {
	case <-flight.done:
		return flight.snapshot, flight.err
	case <-ctx.Done():
		return OverviewSnapshot{}, ctx.Err()
	}
}

func newAnalysisState(generation uint64, ttl time.Duration) StorageAnalysisState {
	sources := make(map[string]StorageSourceProgress, len(storageAnalysisSources))
	for _, source := range storageAnalysisSources {
		sources[source] = StorageSourceProgress{State: SourceStatePending}
	}
	return StorageAnalysisState{
		Generation: generation, State: AnalysisStateReady, CacheTTLSeconds: int64(ttl / time.Second),
		Progress: StorageAnalysisProgress{
			TotalSources: len(storageAnalysisSources), Sources: sources,
		},
	}
}

func scanningAnalysisState(
	generation uint64,
	ttl time.Duration,
	startedAt time.Time,
	previous *OverviewSnapshot,
) StorageAnalysisState {
	state := newAnalysisState(generation, ttl)
	state.State = AnalysisStateScanning
	state.StartedAt = timePointer(startedAt)
	state.Stale = previous != nil
	if previous != nil {
		state.RefreshDueAt = timePointer(previous.AnalyzedAt.Add(ttl))
	}
	return state
}

func readyAnalysisState(
	generation uint64,
	ttl time.Duration,
	startedAt time.Time,
	completedAt time.Time,
	analyzedAt time.Time,
) StorageAnalysisState {
	return readyAnalysisStateWithProgress(
		generation, ttl, startedAt, completedAt, analyzedAt, StorageAnalysisProgress{},
	)
}

func readyAnalysisStateWithProgress(
	generation uint64,
	ttl time.Duration,
	startedAt time.Time,
	completedAt time.Time,
	analyzedAt time.Time,
	progress StorageAnalysisProgress,
) StorageAnalysisState {
	state := newAnalysisState(generation, ttl)
	state.State = AnalysisStateReady
	state.StartedAt = timePointer(startedAt)
	state.CompletedAt = timePointer(completedAt)
	state.DurationMS = durationPointer(completedAt.Sub(startedAt))
	state.RefreshDueAt = timePointer(analyzedAt.Add(ttl))
	if len(progress.Sources) > 0 {
		state.Progress = progress
	}
	for source, progress := range state.Progress.Sources {
		if progress.State == SourceStatePending || progress.State == SourceStateScanning {
			progress.State = SourceStateReady
			state.Progress.Sources[source] = progress
		}
	}
	state.Progress.CompletedSources = completedSourceCount(state.Progress.Sources)
	return state
}

func failedAnalysisState(
	state StorageAnalysisState,
	startedAt time.Time,
	completedAt time.Time,
	err error,
	stale bool,
) StorageAnalysisState {
	state.State = AnalysisStateFailed
	state.StartedAt = timePointer(startedAt)
	state.CompletedAt = timePointer(completedAt)
	state.DurationMS = durationPointer(completedAt.Sub(startedAt))
	state.Stale = stale
	errorText := err.Error()
	state.Error = &errorText
	if stale {
		state.PartialSummary = nil
	}
	return state
}

func completedSourceCount(sources map[string]StorageSourceProgress) int {
	completed := 0
	for _, source := range sources {
		if source.State == SourceStateReady || source.State == SourceStateFailed {
			completed++
		}
	}
	return completed
}

func summaryFromSourceValues(values map[string]any) *Summary {
	if len(values) == 0 {
		return nil
	}
	summary := &Summary{}
	if value, ok := values[StorageSourceWorkspaces]; ok {
		summary.Workspaces = value
	}
	if value, ok := values[StorageSourceGoCache]; ok {
		summary.GoCache = value
	}
	if value, ok := values[StorageSourceQuarantine]; ok {
		summary.Quarantine = value
	}
	if value, ok := values[StorageSourceTemporaryArtifacts]; ok {
		summary.TemporaryArtifacts = value
	}
	if value, ok := values[StorageSourceDocker]; ok {
		summary.Docker = value
	}
	return summary
}

func cloneAnalysisState(state StorageAnalysisState) StorageAnalysisState {
	clone := state
	clone.Progress = cloneAnalysisProgress(state.Progress)
	return clone
}

func cloneAnalysisProgress(progressState StorageAnalysisProgress) StorageAnalysisProgress {
	clone := progressState
	clone.Sources = make(map[string]StorageSourceProgress, len(progressState.Sources))
	for source, progress := range progressState.Sources {
		if progress.TotalItems != nil {
			total := *progress.TotalItems
			progress.TotalItems = &total
		}
		if progress.Error != nil {
			errorText := *progress.Error
			progress.Error = &errorText
		}
		clone.Sources[source] = progress
	}
	return clone
}

func cloneDurations(values map[string]time.Duration) map[string]time.Duration {
	clone := make(map[string]time.Duration, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneSourceStates(values map[string]SourceStateName) map[string]SourceStateName {
	clone := make(map[string]SourceStateName, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func timePointer(value time.Time) *time.Time {
	return &value
}

func durationPointer(value time.Duration) *int64 {
	milliseconds := value.Milliseconds()
	return &milliseconds
}
