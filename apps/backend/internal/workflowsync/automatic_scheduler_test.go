package workflowsync

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAutomaticScheduler_BoundsBlockedWorkAndKeepsReadyJobsMoving(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	release := make(chan struct{})
	progressed := make(chan struct{})
	var startedOnce sync.Once
	var progressedOnce sync.Once
	var active atomic.Int32
	var maximum atomic.Int32

	s := newAutomaticScheduler(ctx, 2, func(_ context.Context, job automaticJob) automaticJobResult {
		current := active.Add(1)
		for {
			old := maximum.Load()
			if current <= old || maximum.CompareAndSwap(old, current) {
				break
			}
		}
		defer active.Add(-1)
		if job.workspaceID == "throttled" {
			startedOnce.Do(func() { close(started) })
			<-release
			return automaticJobResult{}
		}
		progressedOnce.Do(func() { close(progressed) })
		return automaticJobResult{}
	}, func() {})

	if !s.enqueue(automaticJob{workspaceID: "throttled"}) {
		t.Fatal("failed to enqueue throttled job")
	}
	for i := 0; i < 100; i++ {
		if !s.enqueue(automaticJob{workspaceID: "ready"}) {
			t.Fatal("failed to enqueue ready job")
		}
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("throttled job did not start")
	}
	select {
	case <-progressed:
	case <-time.After(time.Second):
		t.Fatal("ready jobs were head-of-line blocked")
	}
	if got := maximum.Load(); got > 2 {
		t.Fatalf("active automatic jobs = %d, want at most 2", got)
	}

	close(release)
	cancel()
	s.stop()
}

func TestAutomaticScheduler_RequeuesAdmissionWaitingJobsWithoutExhaustingWorkers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	admissionChanged := make(chan struct{})
	released := make(chan struct{})
	var readyProgress atomic.Int32
	var blockedRuns atomic.Int32

	s := newAutomaticScheduler(ctx, automaticSyncWorkerLimit, func(_ context.Context, job automaticJob) automaticJobResult {
		if job.workspaceID != "ready-fifth" && len(job.workspaceID) > 0 {
			blockedRuns.Add(1)
			select {
			case <-released:
				return automaticJobResult{}
			default:
			}
			return automaticJobResult{
				wait: func(ctx context.Context) error {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-admissionChanged:
						return nil
					}
				},
			}
		}
		readyProgress.Add(1)
		return automaticJobResult{}
	}, func() {})

	for i := 0; i < automaticSyncWorkerLimit; i++ {
		if !s.enqueue(automaticJob{workspaceID: "blocked-" + string(rune('a'+i))}) {
			t.Fatal("failed to enqueue blocked job")
		}
	}
	if !s.enqueue(automaticJob{workspaceID: "ready-fifth"}) {
		t.Fatal("failed to enqueue ready job")
	}
	deadline := time.After(time.Second)
	for readyProgress.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("ready fifth principal did not progress while four jobs awaited admission")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if blockedRuns.Load() != automaticSyncWorkerLimit {
		t.Fatalf("blocked jobs started = %d, want %d", blockedRuns.Load(), automaticSyncWorkerLimit)
	}

	close(released)
	close(admissionChanged)
	s.stop()
}

func TestAutomaticScheduler_StopDrainsQueuedJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	var ran atomic.Int32

	s := newAutomaticScheduler(ctx, 1, func(ctx context.Context, _ automaticJob) automaticJobResult {
		ran.Add(1)
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return automaticJobResult{}
	}, func() {})
	if !s.enqueue(automaticJob{workspaceID: "active"}) || !s.enqueue(automaticJob{workspaceID: "queued"}) {
		t.Fatal("failed to enqueue jobs")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("active job did not start")
	}
	cancel()
	s.stop()
	if got := ran.Load(); got != 1 {
		t.Fatalf("ran %d jobs, want only active job", got)
	}
}
