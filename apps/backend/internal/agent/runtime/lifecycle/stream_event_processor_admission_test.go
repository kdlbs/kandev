package lifecycle

import (
	"context"
	"testing"
	"time"
)

func TestStreamProjectionWorkerWaitsForEnqueueAccounting(t *testing.T) {
	tracker := &streamProjectionTracker{}
	progress := &streamProjectionProgress{ownerID: 1, tracker: tracker}
	tracker.install(progress.ownerID)
	processor := &streamEventProcessor{ctx: context.Background(), progress: progress}
	admission := streamEventAdmission{accepted: make(chan struct{})}
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		close(started)
		if processor.waitForAdmission(admission) {
			progress.completed(1)
		}
		close(done)
	}()
	<-started
	select {
	case <-done:
		t.Fatal("projection completed before queue admission was recorded")
	default:
	}

	progress.enqueued()
	close(admission.accepted)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("projection worker did not resume after admission")
	}

	snapshot := progress.snapshot()
	if snapshot.enqueued != 1 || snapshot.completed != 1 || snapshot.pending != 0 {
		t.Fatalf("projection snapshot after handoff = %+v, want 1 admitted and completed", snapshot)
	}
}
