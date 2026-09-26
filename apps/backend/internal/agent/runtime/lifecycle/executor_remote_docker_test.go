package lifecycle

import (
	"context"
	"strings"
	"testing"
)

// TestRemoteDockerExecutor_CreateInstanceIsImplemented replaces the assertion
// that this runtime reports itself unimplemented. The stub it guarded is gone:
// a launch now fails on its own configuration, not on the runtime's existence.
func TestRemoteDockerExecutor_CreateInstanceIsImplemented(t *testing.T) {
	executor := NewRemoteDockerExecutor(newTestLogger())
	_, err := executor.CreateInstance(context.Background(), &ExecutorCreateRequest{InstanceID: "instance-1"})
	if err == nil {
		t.Fatal("CreateInstance with an empty request = nil error, want a configuration error")
	}
	if strings.Contains(err.Error(), "not yet implemented") {
		t.Fatalf("CreateInstance still reports itself unimplemented: %v", err)
	}
}
