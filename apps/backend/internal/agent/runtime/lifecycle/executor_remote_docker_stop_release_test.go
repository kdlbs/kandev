package lifecycle

import (
	"context"
	"testing"
)

// TestRemoteDockerOrdinaryStopReleasesItsSession keeps a preserved container
// from pinning its connection. Every launch uses a fresh instance ID, so no
// later call reaches this session again: resume dials a new one, and archive
// or delete reach the container through a fresh connection of their own.
func TestRemoteDockerOrdinaryStopReleasesItsSession(t *testing.T) {
	exec := NewRemoteDockerExecutor(dialerTestLogger(t))
	endpoints := &countingEndpointResolver{}
	exec.sessions["instance-1"] = &remoteDockerSession{endpoints: endpoints}

	err := exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID:  "instance-1",
		ContainerID: "preserved",
		StopReason:  "", // an ordinary stop preserves the container
	}, false)
	if err != nil {
		t.Fatalf("StopInstance() error = %v", err)
	}

	if endpoints.closes != 1 {
		t.Errorf("session closed %d time(s) on an ordinary stop, want 1", endpoints.closes)
	}
	if _, ok := exec.sessions["instance-1"]; ok {
		t.Error("an ordinary stop left the session registered")
	}
}
