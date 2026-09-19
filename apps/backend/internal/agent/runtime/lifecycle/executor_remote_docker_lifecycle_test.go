package lifecycle

import (
	"context"
	"strings"
	"testing"
)

func remoteDockerRequest(instanceID string, metadata map[string]interface{}) *ExecutorCreateRequest {
	md := map[string]interface{}{MetadataKeySSHHost: "build-box"}
	for k, v := range metadata {
		md[k] = v
	}
	return &ExecutorCreateRequest{InstanceID: instanceID, TaskID: "task-1", Metadata: md}
}

// TestRemoteDockerStopPreservesItsSession is finding 1 from branch review.
//
// An ordinary stop preserves the container for resume. Releasing the SSH
// session at the same time strands it: the later archive or delete has no
// connection to reach the daemon, so the container is never removed.
func TestRemoteDockerStopPreservesItsSession(t *testing.T) {
	exec := NewRemoteDockerExecutor(dialerTestLogger(t))
	session := &remoteDockerSession{}
	exec.sessions["instance-1"] = session

	instance := &ExecutorInstance{
		InstanceID:  "instance-1",
		TaskID:      "task-1",
		ContainerID: "container-1",
		StopReason:  "", // an ordinary stop
	}
	if err := exec.StopInstance(context.Background(), instance, false); err != nil {
		t.Fatalf("StopInstance: %v", err)
	}

	exec.mu.Lock()
	_, stillTracked := exec.sessions["instance-1"]
	exec.mu.Unlock()
	if !stillTracked {
		t.Fatal("an ordinary stop released the SSH session; the preserved container is now unreachable")
	}
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
	exec.reconnect = func(_ context.Context, _ *remoteDockerSession, req *ExecutorCreateRequest) (*ExecutorInstance, bool) {
		if getMetadataString(req.Metadata, MetadataKeyContainerID) == "" {
			return nil, false
		}
		reconnected++
		return &ExecutorInstance{InstanceID: req.InstanceID, ContainerID: "preserved"}, true
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
