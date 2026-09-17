package lifecycle

import (
	"context"
	"testing"
)

// TestRemoteDockerClosesAReplacedSession covers the leak a review found: an
// ordinary stop deliberately keeps the session registered, so a later
// CreateInstance for the same instance reaches the assignment with a live
// entry present. Overwriting the map entry strands that session's SSH client,
// Docker client, port forwards, and keepalive watchdog for the backend's
// lifetime.
func TestRemoteDockerClosesAReplacedSession(t *testing.T) {
	exec := NewRemoteDockerExecutor(dialerTestLogger(t))

	firstEndpoints := &countingEndpointResolver{}
	secondEndpoints := &countingEndpointResolver{}
	next := []*remoteDockerSession{
		{endpoints: firstEndpoints},
		{endpoints: secondEndpoints},
	}
	exec.connect = func(context.Context, *ExecutorCreateRequest) (*remoteDockerSession, error) {
		session := next[0]
		next = next[1:]
		return session, nil
	}
	exec.launch = func(context.Context, *remoteDockerSession, *ExecutorCreateRequest) (*ExecutorInstance, error) {
		return &ExecutorInstance{InstanceID: "instance-1"}, nil
	}
	exec.watchTransport = func(string, *remoteDockerSession) {}

	for i := 0; i < 2; i++ {
		if _, err := exec.CreateInstance(context.Background(), remoteDockerRequest("instance-1", nil)); err != nil {
			t.Fatalf("CreateInstance %d: %v", i+1, err)
		}
	}

	if firstEndpoints.closes != 1 {
		t.Errorf("replaced session closed %d time(s), want 1; its forwards and clients leak otherwise",
			firstEndpoints.closes)
	}
	if secondEndpoints.closes != 0 {
		t.Errorf("the live session was closed %d time(s), want 0", secondEndpoints.closes)
	}
}

// countingEndpointResolver records Close so a test can tell whether the
// session that owned it was torn down.
type countingEndpointResolver struct {
	closes int
}

func (c *countingEndpointResolver) Resolve(
	_ context.Context, _ string, containerPort int, fallbackHost string,
) (string, int, error) {
	return fallbackHost, containerPort, nil
}

func (c *countingEndpointResolver) Close() error {
	c.closes++
	return nil
}
