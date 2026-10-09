package statussummary

import (
	"context"
	"expvar"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestStatusSummaryOperationalMetricsPublished(t *testing.T) {
	for _, name := range []string{
		"task_status_summary_cas_retries_total",
		"task_status_summary_cas_exhaustions_total",
		"task_status_summary_event_handler_failures_total",
	} {
		if expvar.Get(name) == nil {
			t.Errorf("expvar %q is not published", name)
		}
	}
}

func TestSubscribedEventHandlerRecordsFailures(t *testing.T) {
	_, _, eventBus, _, _ := newProjectorTest(t)
	before := eventHandlerFailuresTotal.Value()
	event := bus.NewEvent(events.TaskCreated, "test", make(chan int))
	if err := eventBus.Publish(context.Background(), events.TaskCreated, event); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if delta := eventHandlerFailuresTotal.Value() - before; delta != 1 {
		t.Fatalf("subscribed handler failure metric delta = %d, want 1", delta)
	}
}
