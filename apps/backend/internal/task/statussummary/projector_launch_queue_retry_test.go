package statussummary

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

type launchQueueWriteFailureStore struct {
	*projectorTestStore
	fail bool
}

func (s *launchQueueWriteFailureStore) CompareAndUpdateTaskStatusSummary(ctx context.Context, row *StoredTaskStatusSummary) (bool, error) {
	if s.fail {
		return false, fmt.Errorf("transient summary write failure")
	}
	return s.projectorTestStore.CompareAndUpdateTaskStatusSummary(ctx, row)
}

// @covers AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.16
func TestProjectorLaunchQueueRetriesFailedPersistence(t *testing.T) {
	for _, clear := range []bool{false, true} {
		name := "enqueue"
		if clear {
			name = "clear"
		}
		t.Run(name, func(t *testing.T) {
			source := newLaunchQueueSource(1)
			if !clear {
				source.queue = nil
			}
			store := &launchQueueWriteFailureStore{projectorTestStore: newProjectorTestStore()}
			projector := NewProjector(ProjectorConfig{
				Store: store, LoadLaunchQueue: source.load,
				ResolveWorkspace: func(context.Context, string) (string, error) { return "workspace-1", nil },
			})
			handleTaskEvent(t, projector, events.TaskUpdated, "write-retry", nil)
			before := store.summary("write-retry")
			if clear {
				source.queue = nil
			} else {
				source.setInUse(1)
			}
			store.fail = true
			for attempt := 0; attempt < 2; attempt++ {
				err := projector.HandleEvent(context.Background(), bus.NewEvent(events.TaskUpdated, "test", map[string]interface{}{
					"task_id": "write-retry",
				}))
				if err == nil {
					t.Fatal("failed persistence stopped being retryable on the same observation")
				}
				if got := store.summary("write-retry"); got.Revision != before.Revision || (got.LaunchQueue != nil) != clear {
					t.Fatalf("failed write changed persisted summary: %+v", got)
				}
			}
			store.fail = false
			handleTaskEvent(t, projector, events.TaskUpdated, "write-retry", nil)
			got := store.summary("write-retry")
			if (got.LaunchQueue != nil) == clear || got.Revision != before.Revision+1 {
				t.Fatalf("repeated authoritative refresh did not commit queue transition: %+v", got)
			}
		})
	}
}
