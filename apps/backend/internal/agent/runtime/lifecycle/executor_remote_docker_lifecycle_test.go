package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func remoteDockerRequest(instanceID string, metadata map[string]interface{}) *ExecutorCreateRequest {
	md := map[string]interface{}{MetadataKeySSHHost: "build-box"}
	for k, v := range metadata {
		md[k] = v
	}
	return &ExecutorCreateRequest{InstanceID: instanceID, TaskID: "task-1", Metadata: md}
}

// TestRemoteDockerTerminalStopReleasesItsSession is the other half: once the
// container is actually removed, the connection must not leak.
func TestRemoteDockerTerminalStopReleasesItsSession(t *testing.T) {
	exec := NewRemoteDockerExecutor(dialerTestLogger(t))
	exec.sessions["instance-1"] = &remoteDockerSession{}

	instance := &ExecutorInstance{
		InstanceID:  "instance-1",
		TaskID:      "task-1",
		ContainerID: "", // nothing provisioned, so teardown has no daemon work
		StopReason:  StopReasonTaskDeleted,
	}
	if err := exec.StopInstance(context.Background(), instance, false); err != nil {
		t.Fatalf("StopInstance: %v", err)
	}

	exec.mu.Lock()
	_, stillTracked := exec.sessions["instance-1"]
	exec.mu.Unlock()
	if stillTracked {
		t.Fatal("a terminal stop kept its SSH session, leaking the connection")
	}
}

// TestRemoteDockerReconnectsToAPreservedContainer is the resume half of
// finding 1. Launching a second container would abandon the first, along with
// the workspace the user expects to resume into.
func TestRemoteDockerReconnectsToAPreservedContainer(t *testing.T) {
	exec := NewRemoteDockerExecutor(dialerTestLogger(t))
	launched := 0
	reconnected := 0
	exec.connect = func(context.Context, *ExecutorCreateRequest) (*remoteDockerSession, error) {
		return &remoteDockerSession{}, nil
	}
	exec.launch = func(context.Context, *remoteDockerSession, *ExecutorCreateRequest) (*ExecutorInstance, error) {
		launched++
		return &ExecutorInstance{InstanceID: "instance-1", ContainerID: "fresh"}, nil
	}
	exec.reconnect = func(_ context.Context, _ *remoteDockerSession, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
		if getMetadataString(req.Metadata, MetadataKeyContainerID) == "" {
			return nil, nil
		}
		reconnected++
		return &ExecutorInstance{InstanceID: req.InstanceID, ContainerID: "preserved"}, nil
	}

	fresh, err := exec.CreateInstance(context.Background(), remoteDockerRequest("instance-1", nil))
	if err != nil {
		t.Fatalf("fresh CreateInstance: %v", err)
	}
	if fresh.ContainerID != "fresh" || launched != 1 {
		t.Fatalf("expected a fresh launch, got %+v (launched=%d)", fresh, launched)
	}

	resumed, err := exec.CreateInstance(context.Background(), remoteDockerRequest("instance-1",
		map[string]interface{}{MetadataKeyContainerID: "preserved"}))
	if err != nil {
		t.Fatalf("resume CreateInstance: %v", err)
	}
	if resumed.ContainerID != "preserved" {
		t.Fatalf("resume launched a new container %q instead of reattaching", resumed.ContainerID)
	}
	if launched != 1 {
		t.Fatalf("resume launched %d extra container(s); the preserved one was abandoned", launched-1)
	}
	if reconnected != 1 {
		t.Fatalf("reconnect ran %d time(s), want 1", reconnected)
	}
}

func TestRemoteDockerWorkspaceReuseFailureDoesNotLaunchAReplacement(t *testing.T) {
	exec := NewRemoteDockerExecutor(dialerTestLogger(t))
	launched := 0
	exec.connect = func(context.Context, *ExecutorCreateRequest) (*remoteDockerSession, error) {
		return &remoteDockerSession{}, nil
	}
	exec.reconnect = func(context.Context, *remoteDockerSession, *ExecutorCreateRequest) (*ExecutorInstance, error) {
		return nil, nil
	}
	exec.launch = func(context.Context, *remoteDockerSession, *ExecutorCreateRequest) (*ExecutorInstance, error) {
		launched++
		return &ExecutorInstance{InstanceID: "instance-1", ContainerID: "replacement"}, nil
	}
	exec.watchTransport = func(string, *remoteDockerSession) {}

	req := remoteDockerRequest("instance-1", map[string]interface{}{MetadataKeyContainerID: "preserved"})
	req.WorkspaceReuseRequired = true
	_, err := exec.CreateInstance(context.Background(), req)
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("CreateInstance() error = %v, want ErrWorkspaceReuseUnsafe", err)
	}
	if launched != 0 {
		t.Fatalf("workspace reuse failure launched %d replacement container(s), want 0", launched)
	}
}

func TestRemoteDockerWorkspaceReuseReconnectErrorDoesNotLaunchAReplacement(t *testing.T) {
	exec := NewRemoteDockerExecutor(dialerTestLogger(t))
	launched := 0
	exec.connect = func(context.Context, *ExecutorCreateRequest) (*remoteDockerSession, error) {
		return &remoteDockerSession{}, nil
	}
	exec.reconnect = func(context.Context, *remoteDockerSession, *ExecutorCreateRequest) (*ExecutorInstance, error) {
		return nil, errContainerEndpointResolution
	}
	exec.launch = func(context.Context, *remoteDockerSession, *ExecutorCreateRequest) (*ExecutorInstance, error) {
		launched++
		return &ExecutorInstance{InstanceID: "instance-1", ContainerID: "replacement"}, nil
	}
	exec.watchTransport = func(string, *remoteDockerSession) {}

	req := remoteDockerRequest("instance-1", map[string]interface{}{MetadataKeyContainerID: "preserved"})
	req.WorkspaceReuseRequired = true
	_, err := exec.CreateInstance(context.Background(), req)
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("CreateInstance() error = %v, want ErrWorkspaceReuseUnsafe", err)
	}
	if !errors.Is(err, errContainerEndpointResolution) {
		t.Fatalf("CreateInstance() error = %v, want the reconnect cause preserved", err)
	}
	if launched != 0 {
		t.Fatalf("workspace reuse reconnect error launched %d replacement container(s), want 0", launched)
	}
	if len(exec.sessions) != 0 {
		t.Fatalf("workspace reuse reconnect error retained %d session(s), want 0", len(exec.sessions))
	}
}

func TestRemoteDockerReconnectEndpointFailureDoesNotLaunchAReplacement(t *testing.T) {
	exec := NewRemoteDockerExecutor(dialerTestLogger(t))
	launched := 0
	exec.connect = func(context.Context, *ExecutorCreateRequest) (*remoteDockerSession, error) {
		return &remoteDockerSession{}, nil
	}
	exec.reconnect = func(context.Context, *remoteDockerSession, *ExecutorCreateRequest) (*ExecutorInstance, error) {
		return nil, errContainerEndpointResolution
	}
	exec.launch = func(context.Context, *remoteDockerSession, *ExecutorCreateRequest) (*ExecutorInstance, error) {
		launched++
		return &ExecutorInstance{InstanceID: "instance-1", ContainerID: "replacement"}, nil
	}
	exec.watchTransport = func(string, *remoteDockerSession) {}

	_, err := exec.CreateInstance(context.Background(), remoteDockerRequest("instance-1",
		map[string]interface{}{MetadataKeyContainerID: "preserved"}))
	if !errors.Is(err, errContainerEndpointResolution) {
		t.Fatalf("CreateInstance() error = %v, want endpoint resolution error", err)
	}
	if launched != 0 {
		t.Fatalf("endpoint resolution failure launched %d replacement container(s), want 0", launched)
	}
}

// TestRemoteDockerWatchesItsTransport is finding 2 from branch review. Without
// a watchdog a dropped connection leaves the session looking healthy.
func TestRemoteDockerWatchesItsTransport(t *testing.T) {
	exec := NewRemoteDockerExecutor(dialerTestLogger(t))
	watched := 0
	exec.connect = func(context.Context, *ExecutorCreateRequest) (*remoteDockerSession, error) {
		return &remoteDockerSession{}, nil
	}
	exec.launch = func(context.Context, *remoteDockerSession, *ExecutorCreateRequest) (*ExecutorInstance, error) {
		return &ExecutorInstance{InstanceID: "instance-1"}, nil
	}
	exec.watchTransport = func(string, *remoteDockerSession) { watched++ }

	if _, err := exec.CreateInstance(context.Background(), remoteDockerRequest("instance-1", nil)); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if watched != 1 {
		t.Fatalf("transport watchdog attached %d time(s), want 1", watched)
	}
}

// TestRemoteDockerStopWithoutAConnectionSaysSo keeps a stop that cannot reach
// the daemon from reporting success and silently leaving a container running.
func TestRemoteDockerStopWithoutAConnectionSaysSo(t *testing.T) {
	exec := NewRemoteDockerExecutor(dialerTestLogger(t))

	err := exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID:  "instance-1",
		ContainerID: "container-1",
		StopReason:  StopReasonTaskDeleted,
	}, true)
	if err == nil {
		t.Fatal("StopInstance with no connection = nil error; the container was left running")
	}
	if !strings.Contains(err.Error(), "container-1") {
		t.Fatalf("error %q does not name the stranded container", err)
	}
}
