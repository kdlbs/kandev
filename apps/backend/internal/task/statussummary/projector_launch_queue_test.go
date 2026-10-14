package statussummary

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// launchQueueSource is a LoadLaunchQueue stand-in that counts reads and
// returns the current queue, so a test can change the authoritative queue and
// observe which events re-read it.
type launchQueueSource struct {
	mu    sync.Mutex
	queue *LaunchQueueSummary
	reads int
}

func newLaunchQueueSource(inUse int) *launchQueueSource {
	source := &launchQueueSource{}
	source.setInUse(inUse)
	return source
}

func (s *launchQueueSource) setInUse(inUse int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	queuedAt := time.Date(2026, 10, 12, 1, 0, 0, 0, time.UTC)
	s.queue = &LaunchQueueSummary{
		QueuedAt: queuedAt,
		Reason:   LaunchQueueReasonSessionCapacity,
		Retrying: true,
		Capacity: &LaunchQueueCapacity{InUse: inUse, Limit: 4, ObservedAt: queuedAt},
	}
}

func (s *launchQueueSource) load(context.Context, string) (*LaunchQueueSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads++
	return cloneLaunchQueue(s.queue), nil
}

func (s *launchQueueSource) takeReads() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	reads := s.reads
	s.reads = 0
	return reads
}

func newLaunchQueueProjector(source *launchQueueSource) (*Projector, *projectorTestStore) {
	store := newProjectorTestStore()
	projector := NewProjector(ProjectorConfig{
		Store: store,
		ResolveWorkspace: func(context.Context, string) (string, error) {
			return "workspace-1", nil
		},
		LoadLaunchQueue: source.load,
	})
	return projector, store
}

func handleTaskEvent(t *testing.T, projector *Projector, eventType, taskID string, extra map[string]interface{}) {
	t.Helper()
	data := map[string]interface{}{"task_id": taskID, "session_id": "session-1"}
	for key, value := range extra {
		data[key] = value
	}
	if err := projector.handleEvent(context.Background(), bus.NewEvent(eventType, "test", data)); err != nil {
		t.Fatalf("handle %s: %v", eventType, err)
	}
}

// primeLaunchQueue observes the task's queue and persists it with inUse
// sessions, then clears the read count.
func primeLaunchQueue(t *testing.T, projector *Projector, store *projectorTestStore, source *launchQueueSource, taskID string, inUse int) {
	t.Helper()
	handleTaskEvent(t, projector, events.TaskUpdated, taskID, nil)
	source.setInUse(inUse)
	handleTaskEvent(t, projector, events.TaskUpdated, taskID, nil)
	if got := storedInUse(t, store, taskID); got != inUse {
		t.Fatalf("primed capacity in use = %d, want %d", got, inUse)
	}
	source.takeReads()
}

func storedInUse(t *testing.T, store *projectorTestStore, taskID string) int {
	t.Helper()
	summary := store.summary(taskID)
	if summary == nil || summary.LaunchQueue == nil || summary.LaunchQueue.Capacity == nil {
		t.Fatalf("stored summary has no launch queue capacity: %+v", summary)
	}
	return summary.LaunchQueue.Capacity.InUse
}

// unrelatedLaunchQueueEvents cannot change a task's launch queue.
var unrelatedLaunchQueueEvents = []struct {
	eventType string
	extra     map[string]interface{}
}{
	{events.MessageAdded, map[string]interface{}{"author_type": "agent", "type": "message"}},
	{events.MessageUpdated, map[string]interface{}{"author_type": "agent", "type": "tool_call"}},
	{events.TurnStarted, nil},
	{events.TurnCompleted, nil},
	{events.TaskSessionActivityChanged, map[string]interface{}{"foreground_activity": "generating"}},
}

func TestProjectorLaunchQueueIsNotReReadByUnrelatedEvents(t *testing.T) {
	const taskID = "task-launch-queue-unrelated"
	source := newLaunchQueueSource(1)
	projector, store := newLaunchQueueProjector(source)
	primeLaunchQueue(t, projector, store, source, taskID, 4)

	// The authoritative queue moves, but none of these sources can carry it.
	source.setInUse(3)
	for _, event := range unrelatedLaunchQueueEvents {
		handleTaskEvent(t, projector, event.eventType, taskID, event.extra)
		if reads := source.takeReads(); reads != 0 {
			t.Fatalf("%s re-read the launch queue %d times, want 0", event.eventType, reads)
		}
	}
	if got := storedInUse(t, store, taskID); got != 4 {
		t.Fatalf("capacity in use after unrelated events = %d, want the last observation 4", got)
	}

	// The ceiling sweep and deferral writes publish task.updated, which
	// delivers the moved queue.
	handleTaskEvent(t, projector, events.TaskUpdated, taskID, nil)
	if reads := source.takeReads(); reads != 1 {
		t.Fatalf("task.updated read the launch queue %d times, want 1", reads)
	}
	if got := storedInUse(t, store, taskID); got != 3 {
		t.Fatalf("capacity in use after task.updated = %d, want 3", got)
	}
}

func TestProjectorLaunchQueueRefreshesFromItsSources(t *testing.T) {
	sources := []struct {
		eventType string
		extra     map[string]interface{}
	}{
		{events.TaskCreated, nil},
		{events.TaskUpdated, nil},
		{events.TaskStateChanged, nil},
		{events.TaskSessionStateChanged, map[string]interface{}{"new_state": "WAITING_FOR_INPUT", "is_primary": true}},
		{events.MessageQueueStatusChanged, map[string]interface{}{"queued_count": 0}},
	}
	for _, refresh := range sources {
		t.Run(refresh.eventType, func(t *testing.T) {
			taskID := "task-launch-queue-" + refresh.eventType
			source := newLaunchQueueSource(1)
			projector, store := newLaunchQueueProjector(source)
			primeLaunchQueue(t, projector, store, source, taskID, 4)

			source.setInUse(2)
			handleTaskEvent(t, projector, refresh.eventType, taskID, refresh.extra)
			if reads := source.takeReads(); reads != 1 {
				t.Fatalf("%s read the launch queue %d times, want 1", refresh.eventType, reads)
			}
			if got := storedInUse(t, store, taskID); got != 2 {
				t.Fatalf("capacity in use after %s = %d, want 2", refresh.eventType, got)
			}
		})
	}
}

func TestProjectorLaunchQueueObservedOnceForAColdTask(t *testing.T) {
	const taskID = "task-launch-queue-cold"
	source := newLaunchQueueSource(4)
	projector, _ := newLaunchQueueProjector(source)

	handleTaskEvent(t, projector, events.MessageAdded, taskID, map[string]interface{}{"author_type": "agent", "type": "message"})

	if reads := source.takeReads(); reads != 1 {
		t.Fatalf("first event read the launch queue %d times, want one observation", reads)
	}
	projector.mu.Lock()
	state := projector.state[taskID]
	projector.mu.Unlock()
	if state == nil || !state.launchQueueObserved || state.launchQueue == nil || state.launchQueue.Capacity.InUse != 4 {
		t.Fatalf("cold task state did not observe the launch queue: %+v", state)
	}
}
