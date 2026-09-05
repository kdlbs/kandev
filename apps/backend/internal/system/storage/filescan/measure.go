// Package filescan provides bounded, read-only filesystem measurements.
package filescan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
	Bytes int64
	Err   error
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
}

type rootPlan struct {
	root       Root
	partitions []partition
	err        error
}

type partitionResult struct {
	bytes int64
	err   error
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
	if l == nil {
		l = NewLimiter(4)
	}
	plans := make([]rootPlan, len(roots))
	partitions := make([]partition, 0)
	rootPartitionCounts := make([]int, len(roots))
	for index, root := range roots {
		planned, err := planRoot(ctx, index, root)
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
			tracker.completeRoot(index)
		} else if len(plan.partitions) == 0 {
			tracker.completeRoot(index)
		}
	}
	if len(partitions) > 0 {
		l.measurePartitions(ctx, partitions, partitionResults, tracker)
	}
	for index, plan := range plans {
		if plan.err != nil {
			continue
		}
		var errs []error
		for _, measured := range partitionResults[index] {
			if measured.err != nil {
				errs = append(errs, measured.err)
				continue
			}
			results[index].Bytes += measured.bytes
		}
		results[index].Err = errors.Join(errs...)
	}
	return results
}

func (l *Limiter) measurePartitions(
	ctx context.Context,
	partitions []partition,
	partitionResults [][]partitionResult,
	tracker *progressTracker,
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
				measured := l.measurePartition(ctx, item, tracker)
				partitionResults[item.rootIndex][item.partitionIndex] = measured
			}
		}()
	}
	workers.Wait()
}

func (l *Limiter) measurePartition(ctx context.Context, item partition, tracker *progressTracker) partitionResult {
	if err := l.acquire(ctx); err != nil {
		tracker.partitionCompleted(item, 0, err)
		return partitionResult{err: err}
	}
	tracker.partitionStarted(item)
	bytes, err := walkPartition(ctx, item.root, item.path)
	tracker.partitionCompleted(item, bytes, err)
	l.release()
	return partitionResult{bytes: bytes, err: err}
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

func planRoot(ctx context.Context, rootIndex int, root Root) ([]partition, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
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
			return nil, nil
		}
		return nil, fmt.Errorf("symlink found at %s", root.Path)
	}
	if !info.IsDir() {
		return []partition{{rootIndex: rootIndex, root: root, path: root.Path}}, nil
	}
	return planDirectory(ctx, rootIndex, root)
}

func planDirectory(ctx context.Context, rootIndex int, root Root) ([]partition, error) {
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
		if root.Exclude != nil && root.Exclude(path, entry) {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 && root.SymlinkPolicy == SkipSymlinks {
			continue
		}
		partitions = append(partitions, partition{
			rootIndex: rootIndex, partitionIndex: len(partitions), root: root, path: path,
		})
	}
	if len(partitions) == 0 {
		partitions = append(partitions, partition{rootIndex: rootIndex, root: root, path: root.Path})
	}
	return partitions, nil
}

func walkPartition(ctx context.Context, root Root, path string) (int64, error) {
	var total int64
	err := filepath.WalkDir(path, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if root.Exclude != nil && root.Exclude(path, entry) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if root.SymlinkPolicy == SkipSymlinks {
				return nil
			}
			return fmt.Errorf("symlink found at %s", path)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return 0, err
	}
	return total, nil
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
