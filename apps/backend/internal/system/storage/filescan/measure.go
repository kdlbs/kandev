// Package filescan provides bounded, read-only filesystem measurements.
package filescan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type SymlinkPolicy uint8

const (
	SkipSymlinks SymlinkPolicy = iota
	RejectSymlinks
)

type Root struct {
	Path          string
	SymlinkPolicy SymlinkPolicy
	MissingOK     bool
	Exclude       func(string, fs.DirEntry) bool
	ShouldSkip    func(string, fs.DirEntry) (bool, error)
	Validate      func() error
	OverlapRoots  []string
}

type MeasureOptions struct {
	TolerateEntryErrors bool
	CountSkipped        bool
	MaxWarnings         int
	ChildSummaryLimit   int
}

type ChildSummaryStatus string

const (
	ChildSummaryMeasured    ChildSummaryStatus = "measured"
	ChildSummaryPartial     ChildSummaryStatus = "partial"
	ChildSummaryUnavailable ChildSummaryStatus = "unavailable"
	maxChildSummaryEntries                     = 20
)

type ChildEntry struct {
	Name         string             `json:"name"`
	Kind         string             `json:"kind"`
	SizeBytes    *int64             `json:"size_bytes,omitempty"`
	Completeness ChildSummaryStatus `json:"completeness"`
}

type ProgressPhase string

const (
	PartitionStarted   ProgressPhase = "partition_started"
	PartitionCompleted ProgressPhase = "partition_completed"
	RootCompleted      ProgressPhase = "root_completed"
)

type Progress struct {
	Phase               ProgressPhase
	RootIndex           int
	PartitionIndex      int
	CompletedPartitions int
	TotalPartitions     int
	CompletedRoots      int
	TotalRoots          int
	BytesScanned        int64
}

type Result struct {
	Bytes              int64
	OverlappedBytes    int64
	Err                error
	Partial            bool
	SkippedCount       int
	Warnings           []string
	Children           []ChildEntry
	OtherObservedBytes int64
	OtherObservedCount int
	ChildSummaryStatus ChildSummaryStatus
}

type Limiter struct {
	maxPartitions int
	slots         chan struct{}
}

func NewLimiter(maxPartitions int) *Limiter {
	if maxPartitions <= 0 {
		maxPartitions = 4
	}
	return &Limiter{maxPartitions: maxPartitions, slots: make(chan struct{}, maxPartitions)}
}

type partition struct {
	rootIndex      int
	partitionIndex int
	root           Root
	path           string
	skip           bool
	childName      string
	childKind      string
}

type rootPlan struct {
	root       Root
	partitions []partition
	err        error
}

type partitionResult struct {
	bytes           int64
	overlappedBytes int64
	err             error
	partial         bool
	skippedCount    int
	warnings        []string
}

type progressTracker struct {
	mu                  sync.Mutex
	notifyMu            sync.Mutex
	completedPartitions int
	completedRoots      int
	bytesScanned        int64
	rootPartitions      []int
	rootCompleted       []int
	totalPartitions     int
	totalRoots          int
	notify              func(Progress)
}

func (l *Limiter) Measure(ctx context.Context, roots []Root, notify func(Progress)) []Result {
	return l.MeasureWithOptions(ctx, roots, MeasureOptions{}, notify)
}

func (l *Limiter) MeasureWithOptions(
	ctx context.Context,
	roots []Root,
	options MeasureOptions,
	notify func(Progress),
) []Result {
	if l == nil {
		l = NewLimiter(4)
	}
	if options.MaxWarnings <= 0 {
		options.MaxWarnings = 10
	}
	if options.ChildSummaryLimit > maxChildSummaryEntries {
		options.ChildSummaryLimit = maxChildSummaryEntries
	}
	plans := make([]rootPlan, len(roots))
	partitions := make([]partition, 0)
	rootPartitionCounts := make([]int, len(roots))
	for index, root := range roots {
		planned, err := planRoot(ctx, index, root, options)
		plans[index] = rootPlan{root: root, partitions: planned, err: err}
		if err == nil {
			rootPartitionCounts[index] = len(planned)
			partitions = append(partitions, planned...)
		}
	}
	tracker := &progressTracker{
		rootPartitions:  rootPartitionCounts,
		rootCompleted:   make([]int, len(roots)),
		totalPartitions: len(partitions),
		totalRoots:      len(roots),
		notify:          notify,
	}
	results := make([]Result, len(roots))
	partitionResults := make([][]partitionResult, len(roots))
	for index, plan := range plans {
		partitionResults[index] = make([]partitionResult, len(plan.partitions))
		if plan.err != nil {
			results[index].Err = plan.err
			if options.ChildSummaryLimit > 0 {
				results[index].ChildSummaryStatus = ChildSummaryUnavailable
			}
			tracker.completeRoot(index)
		} else if len(plan.partitions) == 0 {
			tracker.completeRoot(index)
		}
	}
	if len(partitions) > 0 {
		l.measurePartitions(ctx, partitions, partitionResults, tracker, options)
	}
	for index, plan := range plans {
		if plan.err != nil {
			continue
		}
		var errs []error
		for _, measured := range partitionResults[index] {
			if measured.err != nil {
				if options.TolerateEntryErrors {
					results[index].Bytes += measured.bytes
					results[index].OverlappedBytes += measured.overlappedBytes
					results[index].Partial = true
					results[index].SkippedCount += measured.skippedCount
					results[index].Warnings = appendBounded(
						results[index].Warnings, measured.warnings, options.MaxWarnings,
					)
				}
				errs = append(errs, measured.err)
				continue
			}
			results[index].Bytes += measured.bytes
			results[index].OverlappedBytes += measured.overlappedBytes
			results[index].Partial = results[index].Partial || measured.partial
			results[index].SkippedCount += measured.skippedCount
			results[index].Warnings = appendBounded(
				results[index].Warnings, measured.warnings, options.MaxWarnings,
			)
		}
		results[index].Err = errors.Join(errs...)
		if options.ChildSummaryLimit > 0 {
			results[index].Children,
				results[index].OtherObservedBytes,
				results[index].OtherObservedCount,
				results[index].ChildSummaryStatus = summarizeChildren(
				plan, partitionResults[index], options.ChildSummaryLimit,
				results[index].Partial, results[index].Err,
			)
		}
	}
	return results
}

func (l *Limiter) measurePartitions(
	ctx context.Context,
	partitions []partition,
	partitionResults [][]partitionResult,
	tracker *progressTracker,
	options MeasureOptions,
) {
	workerCount := l.maxPartitions
	if workerCount > len(partitions) {
		workerCount = len(partitions)
	}
	jobs := make(chan partition, len(partitions))
	for _, item := range partitions {
		jobs <- item
	}
	close(jobs)
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for item := range jobs {
				measured := l.measurePartition(ctx, item, tracker, options)
				partitionResults[item.rootIndex][item.partitionIndex] = measured
			}
		}()
	}
	workers.Wait()
}

func (l *Limiter) measurePartition(
	ctx context.Context,
	item partition,
	tracker *progressTracker,
	options MeasureOptions,
) partitionResult {
	if err := l.acquire(ctx); err != nil {
		tracker.partitionCompleted(item, 0, err)
		return partitionResult{err: err}
	}
	tracker.partitionStarted(item)
	measured := walkPartition(ctx, item.root, item.path, item.skip, options)
	tracker.partitionCompleted(item, measured.bytes, measured.err)
	l.release()
	return measured
}

func (l *Limiter) acquire(ctx context.Context) error {
	select {
	case l.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *Limiter) release() {
	<-l.slots
}

func planRoot(ctx context.Context, rootIndex int, root Root, options MeasureOptions) ([]partition, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if root.Validate != nil {
		if err := root.Validate(); err != nil {
			return nil, err
		}
	}
	info, err := os.Lstat(root.Path)
	if errors.Is(err, os.ErrNotExist) && root.MissingOK {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if root.SymlinkPolicy == SkipSymlinks {
			if options.CountSkipped {
				return []partition{{rootIndex: rootIndex, root: root, path: root.Path, skip: true}}, nil
			}
			return nil, nil
		}
		return nil, fmt.Errorf("symlink found at %s", root.Path)
	}
	if !info.IsDir() {
		return []partition{{rootIndex: rootIndex, root: root, path: root.Path}}, nil
	}
	return planDirectory(ctx, rootIndex, root, options)
}

func planDirectory(
	ctx context.Context,
	rootIndex int,
	root Root,
	options MeasureOptions,
) ([]partition, error) {
	entries, err := os.ReadDir(root.Path)
	if err != nil {
		return nil, err
	}
	partitions := make([]partition, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		path := filepath.Join(root.Path, entry.Name())
		skip, err := shouldSkip(root, path, entry)
		if err != nil {
			return nil, err
		}
		if skip {
			if options.CountSkipped || options.ChildSummaryLimit > 0 {
				partitions = append(partitions, partition{
					rootIndex: rootIndex, partitionIndex: len(partitions), root: root, path: path, skip: true,
					childName: childName(options, entry.Name()), childKind: childKind(options, entry),
				})
			}
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 && root.SymlinkPolicy == SkipSymlinks {
			if options.CountSkipped || options.ChildSummaryLimit > 0 {
				partitions = append(partitions, partition{
					rootIndex: rootIndex, partitionIndex: len(partitions), root: root, path: path, skip: true,
					childName: childName(options, entry.Name()), childKind: childKind(options, entry),
				})
			}
			continue
		}
		partitions = append(partitions, partition{
			rootIndex: rootIndex, partitionIndex: len(partitions), root: root, path: path,
			childName: childName(options, entry.Name()), childKind: childKind(options, entry),
		})
	}
	if len(partitions) == 0 {
		partitions = append(partitions, partition{rootIndex: rootIndex, root: root, path: root.Path})
	}
	return partitions, nil
}

func childName(options MeasureOptions, name string) string {
	if options.ChildSummaryLimit <= 0 {
		return ""
	}
	return name
}

func childKind(options MeasureOptions, entry fs.DirEntry) string {
	if options.ChildSummaryLimit <= 0 {
		return ""
	}
	switch {
	case entry.Type()&os.ModeSymlink != 0:
		return "symlink"
	case entry.IsDir():
		return "directory"
	case entry.Type().IsRegular() || entry.Type() == 0:
		return "file"
	default:
		return "other"
	}
}

func summarizeChildren(
	plan rootPlan,
	measured []partitionResult,
	limit int,
	rootPartial bool,
	rootErr error,
) ([]ChildEntry, int64, int, ChildSummaryStatus) {
	if rootErr != nil && len(plan.partitions) == 0 {
		return nil, 0, 0, ChildSummaryUnavailable
	}
	children, totalObservedBytes, totalCount, partial := summarizeChildPartitions(plan, measured, limit)
	status := ChildSummaryMeasured
	if rootPartial || partial {
		status = ChildSummaryPartial
	}
	otherBytes, otherCount := unsummarizedChildTotals(totalObservedBytes, totalCount, children)
	return children, otherBytes, otherCount, status
}

func summarizeChildPartitions(
	plan rootPlan,
	measured []partitionResult,
	limit int,
) ([]ChildEntry, int64, int, bool) {
	children := make([]ChildEntry, 0, limit)
	var totalObservedBytes int64
	totalCount := 0
	partial := false
	for index, item := range plan.partitions {
		if item.childName == "" {
			continue
		}
		totalCount++
		child, observedBytes, hasObservedBytes, childPartial := summarizeChild(item, childMeasurementAt(measured, index))
		if hasObservedBytes {
			totalObservedBytes = saturatedAdd(totalObservedBytes, observedBytes)
		}
		partial = partial || childPartial
		children = insertBoundedChild(children, child, limit)
	}
	return children, totalObservedBytes, totalCount, partial
}

func childMeasurementAt(measured []partitionResult, index int) partitionResult {
	if index < len(measured) {
		return measured[index]
	}
	return partitionResult{err: errors.New("child measurement result is unavailable")}
}

func summarizeChild(item partition, measured partitionResult) (ChildEntry, int64, bool, bool) {
	child := ChildEntry{Name: item.childName, Kind: item.childKind, Completeness: ChildSummaryMeasured}
	knownBytes := !item.skip && measured.err == nil && !measured.partial
	if !item.skip && measured.bytes > 0 {
		knownBytes = true
	}
	if knownBytes {
		observed := measured.bytes
		child.SizeBytes = &observed
	}
	partial := item.skip || measured.err != nil || measured.partial
	if partial {
		child.Completeness = ChildSummaryPartial
	}
	return child, measured.bytes, knownBytes, partial
}

func unsummarizedChildTotals(totalObservedBytes int64, totalCount int, children []ChildEntry) (int64, int) {
	var listedBytes int64
	for _, child := range children {
		if child.SizeBytes != nil {
			listedBytes = saturatedAdd(listedBytes, *child.SizeBytes)
		}
	}
	otherBytes := totalObservedBytes - listedBytes
	if otherBytes < 0 {
		otherBytes = 0
	}
	otherCount := totalCount - len(children)
	return otherBytes, otherCount
}

func insertBoundedChild(children []ChildEntry, child ChildEntry, limit int) []ChildEntry {
	index := len(children)
	for i, existing := range children {
		if childLess(child, existing) {
			index = i
			break
		}
	}
	if index >= limit {
		return children
	}
	children = append(children, ChildEntry{})
	copy(children[index+1:], children[index:])
	children[index] = child
	if len(children) > limit {
		children = children[:limit]
	}
	return children
}

func childLess(left, right ChildEntry) bool {
	if (left.SizeBytes == nil) != (right.SizeBytes == nil) {
		return left.SizeBytes != nil
	}
	if left.SizeBytes != nil && *left.SizeBytes != *right.SizeBytes {
		return *left.SizeBytes > *right.SizeBytes
	}
	return left.Name < right.Name
}

func saturatedAdd(left, right int64) int64 {
	maxInt64 := int64(^uint64(0) >> 1)
	if right > 0 && left > maxInt64-right {
		return maxInt64
	}
	return left + right
}

func walkPartition(
	ctx context.Context,
	root Root,
	path string,
	skip bool,
	options MeasureOptions,
) partitionResult {
	result := partitionResult{}
	if skip {
		if options.CountSkipped {
			result.partial = true
			result.skippedCount = 1
		}
		return result
	}
	if root.Validate != nil {
		if err := root.Validate(); err != nil {
			result.err = err
			return result
		}
	}
	err := filepath.WalkDir(path, func(path string, entry fs.DirEntry, walkErr error) error {
		return walkEntry(ctx, root, path, entry, walkErr, &result, options)
	})
	if err != nil {
		result.err = err
	}
	return result
}

func walkEntry(
	ctx context.Context,
	root Root,
	path string,
	entry fs.DirEntry,
	walkErr error,
	result *partitionResult,
	options MeasureOptions,
) error {
	if walkErr != nil {
		return tolerateWalkError(path, entry, walkErr, result, options)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	skip, err := shouldSkip(root, path, entry)
	if err != nil {
		return err
	}
	if skip {
		return skipEntry(entry, result, options)
	}
	if entry.Type()&os.ModeSymlink != 0 {
		return handleSymlink(path, root, result, options)
	}
	if entry.IsDir() {
		return nil
	}
	if !entry.Type().IsRegular() {
		markSkipped(result, options)
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := entry.Info()
	if err != nil {
		return tolerateInfoError(path, err, result, options)
	}
	result.bytes += info.Size()
	if pathWithinAny(path, root.OverlapRoots) {
		result.overlappedBytes += info.Size()
	}
	return nil
}

func tolerateWalkError(
	path string,
	entry fs.DirEntry,
	walkErr error,
	result *partitionResult,
	options MeasureOptions,
) error {
	if !options.TolerateEntryErrors {
		return walkErr
	}
	result.partial = true
	result.skippedCount++
	result.warnings = appendWarning(result.warnings, fmt.Errorf("%s: %w", path, walkErr), options.MaxWarnings)
	if entry != nil && entry.IsDir() {
		return fs.SkipDir
	}
	return nil
}

func skipEntry(entry fs.DirEntry, result *partitionResult, options MeasureOptions) error {
	markSkipped(result, options)
	if entry.IsDir() {
		return fs.SkipDir
	}
	return nil
}

func handleSymlink(path string, root Root, result *partitionResult, options MeasureOptions) error {
	if root.SymlinkPolicy != SkipSymlinks {
		return fmt.Errorf("symlink found at %s", path)
	}
	markSkipped(result, options)
	return nil
}

func tolerateInfoError(
	path string,
	infoErr error,
	result *partitionResult,
	options MeasureOptions,
) error {
	if !options.TolerateEntryErrors {
		return infoErr
	}
	result.partial = true
	result.skippedCount++
	result.warnings = appendWarning(result.warnings, fmt.Errorf("%s: %w", path, infoErr), options.MaxWarnings)
	return nil
}

func markSkipped(result *partitionResult, options MeasureOptions) {
	if !options.CountSkipped {
		return
	}
	result.partial = true
	result.skippedCount++
}

func shouldSkip(root Root, path string, entry fs.DirEntry) (bool, error) {
	if root.Exclude != nil && root.Exclude(path, entry) {
		return true, nil
	}
	if root.ShouldSkip != nil {
		return root.ShouldSkip(path, entry)
	}
	return false, nil
}

func appendWarning(warnings []string, err error, limit int) []string {
	if err == nil || len(warnings) >= limit {
		return warnings
	}
	return append(warnings, err.Error())
}

func appendBounded(warnings, additions []string, limit int) []string {
	for _, warning := range additions {
		if len(warnings) >= limit {
			break
		}
		warnings = append(warnings, warning)
	}
	return warnings
}

func pathWithinAny(path string, roots []string) bool {
	for _, root := range roots {
		if pathWithin(path, root) {
			return true
		}
	}
	return false
}

func pathWithin(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (t *progressTracker) partitionStarted(item partition) {
	t.mu.Lock()
	event := t.progressLocked(Progress{
		Phase: PartitionStarted, RootIndex: item.rootIndex, PartitionIndex: item.partitionIndex,
	})
	t.mu.Unlock()
	t.emitEvent(event)
}

func (t *progressTracker) partitionCompleted(item partition, bytes int64, err error) {
	t.notifyMu.Lock()
	defer t.notifyMu.Unlock()
	t.mu.Lock()
	t.completedPartitions++
	if err == nil {
		t.bytesScanned += bytes
	}
	t.rootCompleted[item.rootIndex]++
	rootDone := t.rootCompleted[item.rootIndex] == t.rootPartitions[item.rootIndex]
	event := t.progressLocked(Progress{
		Phase: PartitionCompleted, RootIndex: item.rootIndex, PartitionIndex: item.partitionIndex,
	})
	if rootDone {
		t.completedRoots++
	}
	var rootEvent *Progress
	if rootDone {
		event := t.progressLocked(Progress{
			Phase: RootCompleted, RootIndex: item.rootIndex, PartitionIndex: item.partitionIndex,
		})
		rootEvent = &event
	}
	t.mu.Unlock()
	t.emitEvent(event)
	if rootEvent != nil {
		t.emitEvent(*rootEvent)
	}
}

func (t *progressTracker) completeRoot(rootIndex int) {
	t.notifyMu.Lock()
	defer t.notifyMu.Unlock()
	t.mu.Lock()
	t.completedRoots++
	event := t.progressLocked(Progress{Phase: RootCompleted, RootIndex: rootIndex})
	t.mu.Unlock()
	t.emitEvent(event)
}

func (t *progressTracker) progressLocked(event Progress) Progress {
	event.CompletedPartitions = t.completedPartitions
	event.TotalPartitions = t.totalPartitions
	event.CompletedRoots = t.completedRoots
	event.TotalRoots = t.totalRoots
	event.BytesScanned = t.bytesScanned
	return event
}

func (t *progressTracker) emitEvent(event Progress) {
	if t.notify != nil {
		t.notify(event)
	}
}
