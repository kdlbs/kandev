package workflowsync

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/github"
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
	<-started
	<-progressed
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
	blockedStarted := make(chan struct{}, automaticSyncWorkerLimit)
	readyProgress := make(chan struct{})

	s := newAutomaticScheduler(ctx, automaticSyncWorkerLimit, func(_ context.Context, job automaticJob) automaticJobResult {
		if job.workspaceID != "ready-fifth" && len(job.workspaceID) > 0 {
			blockedStarted <- struct{}{}
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
		close(readyProgress)
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
	for i := 0; i < automaticSyncWorkerLimit; i++ {
		<-blockedStarted
	}
	<-readyProgress

	close(released)
	close(admissionChanged)
	s.stop()
}

func TestAutomaticScheduler_RequeuesAfterAdmissionSignalWithoutAnotherPoll(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	admissionChanged := make(chan struct{})
	started := make(chan struct{})
	resumed := make(chan struct{})
	var runs atomic.Int32

	s := newAutomaticScheduler(ctx, 1, func(ctx context.Context, _ automaticJob) automaticJobResult {
		if runs.Add(1) == 1 {
			close(started)
			return automaticJobResult{wait: func(ctx context.Context) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-admissionChanged:
					return nil
				}
			}}
		}
		close(resumed)
		return automaticJobResult{}
	}, func() {})
	defer s.stop()

	if !s.enqueue(automaticJob{workspaceID: "waiting"}) {
		t.Fatal("failed to enqueue job")
	}
	<-started
	close(admissionChanged)
	<-resumed
}

func TestAutomaticScheduler_RequeuesWhenRateTrackerBecomesReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracker := github.NewRateTracker(nil, nil)
	tracker.ObserveSecondary(
		github.ResourceCore, time.Now().Add(24*time.Hour),
		github.RetrySourceConservativeFallback, "fixture",
	)
	attempted := make(chan struct{}, 2)
	completed := make(chan struct{})
	discarded := make(chan struct{})
	var runs atomic.Int32

	s := newAutomaticScheduler(ctx, 1, func(ctx context.Context, _ automaticJob) automaticJobResult {
		if runs.Add(1) == 1 {
			deferred := &github.AdmissionDeferredError{
				Delay:          24 * time.Hour,
				TrackerChanged: tracker.Changed(),
				Reason:         "fixture",
			}
			attempted <- struct{}{}
			return automaticJobResult{
				wait:    deferred.Wait,
				discard: func() { close(discarded) },
			}
		}
		attempted <- struct{}{}
		close(completed)
		return automaticJobResult{}
	}, func() {})
	defer s.stop()

	if !s.enqueue(automaticJob{workspaceID: "github"}) {
		t.Fatal("failed to enqueue job")
	}
	<-attempted
	tracker.ObserveSuccess(github.ResourceCore)
	<-attempted
	<-completed
	select {
	case <-discarded:
		t.Fatal("ready job was discarded")
	default:
	}
}

func TestAutomaticScheduler_CancellationDiscardsDeferredWatcher(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	discarded := make(chan struct{})

	s := newAutomaticScheduler(ctx, 1, func(ctx context.Context, _ automaticJob) automaticJobResult {
		close(started)
		return automaticJobResult{
			wait: func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
			discard: func() { close(discarded) },
		}
	}, func() {})

	if !s.enqueue(automaticJob{workspaceID: "cancelled"}) {
		t.Fatal("failed to enqueue job")
	}
	<-started
	cancel()
	<-discarded
	s.stop()
}

func TestAutomaticScheduler_CloseDiscardsDeferredJobRequeuedBeforeShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newAutomaticScheduler(ctx, 1, func(context.Context, automaticJob) automaticJobResult {
		return automaticJobResult{}
	}, func() {})

	var discarded atomic.Int32
	if !s.enqueueOwned(queuedAutomaticJob{
		job:     automaticJob{workspaceID: "requeued"},
		discard: func() { discarded.Add(1) },
	}) {
		t.Fatal("failed to enqueue deferred job")
	}
	s.close()
	if got := discarded.Load(); got != 1 {
		t.Fatalf("discard calls = %d, want exactly one", got)
	}
	s.stop()
}

func TestAutomaticScheduler_RejectedDeferredJobDiscardsExactlyOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newAutomaticScheduler(ctx, 1, func(context.Context, automaticJob) automaticJobResult {
		return automaticJobResult{}
	}, func() {})
	s.close()

	var discarded atomic.Int32
	if s.enqueueOwned(queuedAutomaticJob{
		job:     automaticJob{workspaceID: "rejected"},
		discard: func() { discarded.Add(1) },
	}) {
		t.Fatal("closed scheduler accepted deferred job")
	}
	if got := discarded.Load(); got != 1 {
		t.Fatalf("discard calls = %d, want exactly one", got)
	}
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
	<-started
	cancel()
	s.stop()
	if got := ran.Load(); got != 1 {
		t.Fatalf("ran %d jobs, want only active job", got)
	}
}
