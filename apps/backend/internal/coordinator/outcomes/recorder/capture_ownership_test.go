package recorder

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/coordinator"
)

type captureRequestKey struct{}

type capturedDecision struct {
	event coordinator.DecisionEvent
	value any
}

type gatedCaptureObserver struct {
	entered chan struct{}
	release chan struct{}
	exited  chan struct{}
	results chan capturedDecision
	once    sync.Once
}

func (o *gatedCaptureObserver) OnDecision(ctx context.Context, event coordinator.DecisionEvent) {
	o.once.Do(func() {
		close(o.entered)
		select {
		case <-o.release:
		case <-ctx.Done():
			close(o.exited)
		}
	})
	if ctx.Err() == nil {
		o.results <- capturedDecision{event: event, value: ctx.Value(captureRequestKey{})}
	}
}

func newGatedCaptureQueue(t *testing.T) (*decisionCaptureQueue, *gatedCaptureObserver) {
	t.Helper()
	sink := &gatedCaptureObserver{entered: make(chan struct{}), release: make(chan struct{}),
		exited: make(chan struct{}), results: make(chan capturedDecision, captureQueueCap+2)}
	q := newDecisionCaptureQueue(&Observers{log: zap.NewNop()}, sink)
	t.Cleanup(q.Stop)
	return q, sink
}

func TestCaptureQueueOwnsEventAndDetachedRequestIdentity(t *testing.T) {
	q, sink := newGatedCaptureQueue(t)
	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), captureRequestKey{}, "manager request"))
	cancel()
	q.OnDecision(ctx, coordinator.DecisionEvent{ProposalID: "first"})
	<-sink.entered
	fields := []string{"title"}
	q.OnDecision(ctx, coordinator.DecisionEvent{ProposalID: "second", EditedFields: fields})
	fields[0] = "context"
	close(sink.release)
	for _, id := range []string{"first", "second"} {
		select {
		case got := <-sink.results:
			if got.event.ProposalID != id || got.value != "manager request" {
				t.Fatalf("capture order or identity changed: %+v", got)
			}
			if id == "second" && (len(got.event.EditedFields) != 1 || got.event.EditedFields[0] != "title") {
				t.Fatalf("queued event borrowed caller fields: %+v", got.event)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("detached capture was abandoned or failed to drain")
		}
	}
}

func TestCaptureQueueBoundsAdmissionAndJoinsCancelledIO(t *testing.T) {
	q, sink := newGatedCaptureQueue(t)
	q.OnDecision(t.Context(), coordinator.DecisionEvent{ProposalID: "active"})
	<-sink.entered
	before := read(captureDroppedTotal, CaptureDroppedQueueFull)
	for range captureQueueCap + 1 {
		q.OnDecision(t.Context(), coordinator.DecisionEvent{ProposalID: "queued"})
	}
	if got := read(captureDroppedTotal, CaptureDroppedQueueFull); got != before+1 {
		t.Fatalf("queue overflow count = %d, want %d", got, before+1)
	}
	q.Stop()
	select {
	case <-sink.exited:
	default:
		t.Fatal("stop returned before capture I/O observed cancellation")
	}
	before = read(captureDroppedTotal, CaptureDroppedStopped)
	q.OnDecision(t.Context(), coordinator.DecisionEvent{ProposalID: "late"})
	if got := read(captureDroppedTotal, CaptureDroppedStopped); got != before+1 {
		t.Fatal("capture accepted work after shutdown")
	}
}
