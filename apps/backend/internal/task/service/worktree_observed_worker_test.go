package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
)

func TestWorktreeObservationWorkerCoalescesLatestSnapshotPerEnvironment(t *testing.T) {
	started := make(chan string, 4)
	release := make(chan struct{})
	worker := newWorktreeObservationWorker(func(_ context.Context, payload *events.WorktreeInventoryData) {
		started <- payload.TaskID
		<-release
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker.start(ctx)

	worker.enqueue(&events.WorktreeInventoryData{TaskID: "first", TaskEnvironmentID: "env-1"})
	select {
	case taskID := <-started:
		if taskID != "first" {
			t.Fatalf("first processing = %q", taskID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not start processing")
	}
	worker.enqueue(&events.WorktreeInventoryData{TaskID: "latest", TaskEnvironmentID: "env-1"})
	worker.enqueue(&events.WorktreeInventoryData{TaskID: "other", TaskEnvironmentID: "env-2"})
	close(release)

	got := map[string]bool{}
	for len(got) < 2 {
		select {
		case taskID := <-started:
			got[taskID] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for coalesced snapshots, got %v", got)
		}
	}
	if !got["latest"] || !got["other"] || got["first"] {
		t.Fatalf("processed snapshots = %v, want latest for env-1 and other for env-2", got)
	}
	worker.stop()
}

func TestWorktreeObservationWorkerStopWaitsForProcessing(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	worker := newWorktreeObservationWorker(func(context.Context, *events.WorktreeInventoryData) {
		once.Do(func() { close(started) })
		<-release
	})
	worker.start(context.Background())
	worker.enqueue(&events.WorktreeInventoryData{TaskID: "task", TaskEnvironmentID: "env"})
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not start")
	}
	stopped := make(chan struct{})
	go func() {
		worker.stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("stop returned before processing finished")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("stop did not finish after processing completed")
	}
}
