package service

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/statussummary"
)

func TestStatusSummaryLaunchQueueEqualityIgnoresCapacityObservationTime(t *testing.T) {
	queuedAt := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	left := &statussummary.LaunchQueueSummary{
		SessionID: "session-luna", QueuedAt: queuedAt,
		Reason: statussummary.LaunchQueueReasonSessionCapacity, Retrying: true,
		Capacity: &statussummary.LaunchQueueCapacity{InUse: 5, Limit: 5, ObservedAt: queuedAt},
	}
	right := *left
	right.Capacity = &statussummary.LaunchQueueCapacity{InUse: 5, Limit: 5, ObservedAt: queuedAt.Add(time.Minute)}
	if !statussummaryLaunchQueueEqual(left, &right) {
		t.Fatal("capacity observation timestamp alone must not require persistence")
	}
}

func TestOverlayLaunchQueueObservationRefreshesResponseWithoutMutatingCurrent(t *testing.T) {
	queuedAt := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	oldObservedAt := queuedAt.Add(time.Minute)
	newObservedAt := queuedAt.Add(2 * time.Minute)
	current := &statussummary.TaskStatusSummary{
		Revision: 7,
		LaunchQueue: &statussummary.LaunchQueueSummary{
			SessionID: "session-luna", QueuedAt: queuedAt,
			Reason: statussummary.LaunchQueueReasonSessionCapacity, Retrying: true,
			Capacity: &statussummary.LaunchQueueCapacity{InUse: 5, Limit: 5, ObservedAt: oldObservedAt},
		},
	}
	observed := *current.LaunchQueue
	observed.Capacity = &statussummary.LaunchQueueCapacity{InUse: 5, Limit: 5, ObservedAt: newObservedAt}

	got := overlayLaunchQueueObservation(current, &observed, true)
	if got == current || got.LaunchQueue == current.LaunchQueue || got.LaunchQueue.Capacity == current.LaunchQueue.Capacity {
		t.Fatal("observation overlay must return cloned summary data")
	}
	if got.Revision != current.Revision || !got.LaunchQueue.Capacity.ObservedAt.Equal(newObservedAt) {
		t.Fatalf("overlay = %+v, want revision %d and observation %s", got, current.Revision, newObservedAt)
	}
	if !current.LaunchQueue.Capacity.ObservedAt.Equal(oldObservedAt) {
		t.Fatal("observation overlay mutated the current summary")
	}
}
