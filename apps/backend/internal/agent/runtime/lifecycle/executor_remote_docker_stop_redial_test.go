package lifecycle

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/agent/docker"
)

// recordingEngine answers every Engine API request with success and records
// the method and path of each one.
type recordingEngine struct {
	mu    sync.Mutex
	calls []string
}

func (e *recordingEngine) handler(t *testing.T) sshStreamHandler {
	t.Helper()
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			e.mu.Lock()
			e.calls = append(e.calls, r.Method+" "+r.URL.Path)
			e.mu.Unlock()
			w.Header().Set("Api-Version", "1.51")
			w.WriteHeader(http.StatusNoContent)
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return func(_ string, stream ssh.Channel) int {
		clientSide, serverSide := net.Pipe()
		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = server.Serve(newOneShotListener(serverSide))
		}()
		go func() {
			_, _ = io.Copy(clientSide, stream)
			_ = clientSide.Close()
		}()
		_, _ = io.Copy(stream, clientSide)
		<-done
		return 0
	}
}

func (e *recordingEngine) called(substr string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, call := range e.calls {
		if strings.Contains(call, substr) {
			return true
		}
	}
	return false
}

func remoteDockerTargetMetadata() map[string]interface{} {
	return map[string]interface{}{
		MetadataKeySSHHost:            "docker-host.lan",
		MetadataKeySSHUser:            "kandev",
		MetadataKeySSHHostFingerprint: "SHA256:pinned",
	}
}

// launchForRedialTest records the target of a launched instance and then drops
// its session, which is what the keepalive watchdog does when the transport
// is lost.
func launchForRedialTest(t *testing.T, exec *RemoteDockerExecutor) {
	t.Helper()
	exec.connect = func(context.Context, *ExecutorCreateRequest) (*remoteDockerSession, error) {
		return &remoteDockerSession{}, nil
	}
	exec.reconnect = func(context.Context, *remoteDockerSession, *ExecutorCreateRequest) (*ExecutorInstance, error) {
		return nil, nil
	}
	exec.launch = func(context.Context, *remoteDockerSession, *ExecutorCreateRequest) (*ExecutorInstance, error) {
		return &ExecutorInstance{InstanceID: "instance-1", ContainerID: "container-1"}, nil
	}
	exec.watchTransport = func(string, *remoteDockerSession) {}

	req := remoteDockerRequest("instance-1", remoteDockerTargetMetadata())
	if _, err := exec.CreateInstance(context.Background(), req); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	exec.releaseSession("instance-1")
}

// TestRemoteDockerTeardownRedialsAfterTransportLoss keeps a lost connection
// from making a container unremovable. The watchdog drops the session, but the
// instance's target is still known, so teardown connects again and removes the
// container instead of leaving it running on the remote host.
func TestRemoteDockerTeardownRedialsAfterTransportLoss(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	defer server.Close()
	engine := &recordingEngine{}
	server.setStreamHandler(engine.handler(t))

	exec := NewRemoteDockerExecutor(dialerTestLogger(t))
	launchForRedialTest(t, exec)

	var redialTarget map[string]interface{}
	endpoints := &countingEndpointResolver{}
	exec.connect = func(_ context.Context, req *ExecutorCreateRequest) (*remoteDockerSession, error) {
		redialTarget = req.Metadata
		sshClient := server.dial(t)
		cli, err := docker.NewRemoteClient(NewSSHDockerDialer(sshClient, dialerTestLogger(t)), dialerTestLogger(t))
		if err != nil {
			return nil, err
		}
		return &remoteDockerSession{sshClient: sshClient, dockerClient: cli, endpoints: endpoints}, nil
	}

	err := exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID:  "instance-1",
		ContainerID: "container-1",
	}, true)
	if err != nil {
		t.Fatalf("StopInstance() error = %v, want the container removed over a new connection", err)
	}

	if got := getMetadataString(redialTarget, MetadataKeySSHHost); got != "docker-host.lan" {
		t.Fatalf("redialed host = %q, want the launched instance's target", got)
	}
	if !engine.called("DELETE ") || !engine.called("/containers/container-1") {
		t.Fatalf("no container removal reached the daemon; calls = %v", engine.calls)
	}
	if endpoints.closes != 1 {
		t.Errorf("redialed session closed %d time(s), want 1", endpoints.closes)
	}
	if len(exec.targets) != 0 {
		t.Errorf("stop left %d target(s) recorded, want 0", len(exec.targets))
	}
}

// TestRemoteDockerTeardownReportsAFailedRedial names the connection failure
// rather than a generic "no live connection", so the operator can fix it.
func TestRemoteDockerTeardownReportsAFailedRedial(t *testing.T) {
	exec := NewRemoteDockerExecutor(dialerTestLogger(t))
	launchForRedialTest(t, exec)

	dialErr := errors.New("connect to docker-host.lan: connection refused")
	exec.connect = func(context.Context, *ExecutorCreateRequest) (*remoteDockerSession, error) {
		return nil, dialErr
	}

	err := exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID:  "instance-1",
		ContainerID: "container-1",
	}, true)
	if !errors.Is(err, dialErr) {
		t.Fatalf("StopInstance() error = %v, want the redial failure", err)
	}
}

// TestRemoteDockerOrdinaryStopForgetsItsTarget keeps the target map bounded:
// every launch uses a fresh instance ID, so a target outliving its stop would
// never be read again.
func TestRemoteDockerOrdinaryStopForgetsItsTarget(t *testing.T) {
	exec := NewRemoteDockerExecutor(dialerTestLogger(t))
	launchForRedialTest(t, exec)

	if err := exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID:  "instance-1",
		ContainerID: "container-1",
	}, false); err != nil {
		t.Fatalf("StopInstance() error = %v", err)
	}
	if len(exec.targets) != 0 {
		t.Errorf("stop left %d target(s) recorded, want 0", len(exec.targets))
	}
}
