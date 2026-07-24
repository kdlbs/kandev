package lifecycle

import (
	"context"
	"strings"
	"testing"
)

func TestRemoteDockerExecutor_CreateInstanceRemainsExplicitlyUnsupported(t *testing.T) {
	executor := NewRemoteDockerExecutor(newTestLogger())
	_, err := executor.CreateInstance(context.Background(), &ExecutorCreateRequest{InstanceID: "instance-1"})
	if err == nil || !strings.Contains(err.Error(), "not yet implemented") {
		t.Fatalf("CreateInstance error=%v; want explicit unsupported implementation error", err)
	}
}
