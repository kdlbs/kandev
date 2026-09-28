package lifecycle

import (
	"context"
	"testing"
	"time"
)

type closeSignalEndpointResolver struct {
	closed chan struct{}
}

func (r *closeSignalEndpointResolver) Resolve(
	_ context.Context, _ string, containerPort int, fallbackHost string,
) (string, int, error) {
	return fallbackHost, containerPort, nil
}

func (r *closeSignalEndpointResolver) Close() error {
	select {
	case <-r.closed:
	default:
		close(r.closed)
	}
	return nil
}

func TestRemoteDockerTransportLossDoesNotWaitOnItsOwnWatchdogLoop(t *testing.T) {
	withSSHKeepaliveTuning(t, 5*time.Millisecond, 20*time.Millisecond)
	server := newFakeSSHServer(t, nil)
	client := server.dial(t)
	server.setSilent(true)

	resolver := &closeSignalEndpointResolver{closed: make(chan struct{})}
	exec := NewRemoteDockerExecutor(dialerTestLogger(t))
	session := &remoteDockerSession{sshClient: client, endpoints: resolver}
	exec.sessions["instance-1"] = session
	exec.startTransportWatchdog("instance-1", session)

	select {
	case <-resolver.closed:
	case <-time.After(2 * time.Second):
		_ = client.Close()
		t.Fatal("transport-loss callback deadlocked while waiting for its own watchdog loop")
	}
}

func TestRemoteDockerSessionCloseUnblocksOutstandingKeepalive(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	client := server.dial(t)
	server.setSilent(true)

	watchdog := startSSHKeepaliveWatchdog(
		client,
		5*time.Millisecond,
		time.Hour,
		time.Now(),
		nil,
		nil,
	)
	session := &remoteDockerSession{sshClient: client, watchdog: watchdog}
	done := make(chan error, 1)
	go func() { done <- session.close() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("close() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		_ = client.Close()
		<-done
		t.Fatal("session close waited for a keepalive probe before closing its SSH client")
	}
}
