package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDispatchWatcherEvent_DoesNotReleaseUnclaimedReservation(t *testing.T) {
	source := &fakeWatcherSource{name: "linear", watchID: "watch", metadataKey: "linear", maxInflightTasks: ptrInt(2)}
	counter := newFakeWatcherCounter()
	counter.set("linear", "watch", 2)
	svc := nopServiceWithCounter(t, counter)
	svc.issueTaskCreator = &fakeTaskCreator{}
	svc.watcherCoordinator = newTestCoordinator(t, nil, false, nil)
	svc.dispatchWatcherEvent(t.Context(), source, "event")
	require.Zero(t, source.recordedRelease)
}
