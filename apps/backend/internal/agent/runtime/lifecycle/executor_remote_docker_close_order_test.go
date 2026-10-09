package lifecycle

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/agent/docker"
)

// serveEngineAPIIgnoringEOF answers the Engine API and then keeps the stream
// open after stdin EOF, which is what a dead link looks like from this side:
// the remote never reports an exit, so Wait returns only when the SSH
// connection itself closes.
func serveEngineAPIIgnoringEOF(t *testing.T, release <-chan struct{}) sshStreamHandler {
	t.Helper()
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Api-Version", "1.51")
			w.WriteHeader(http.StatusOK)
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return func(_ string, stream ssh.Channel) int {
		clientSide, serverSide := net.Pipe()
		go func() { _ = server.Serve(newOneShotListener(serverSide)) }()
		go func() { _, _ = io.Copy(stream, clientSide) }()
		_, _ = io.Copy(clientSide, stream)
		<-release
		_ = clientSide.Close()
		return 0
	}
}

// TestRemoteDockerSessionCloseDoesNotWaitOnIdleConnections keeps teardown of
// a session with pooled Engine API connections prompt. Each pooled connection
// waits up to dialCloseWaitBudget for its remote command to exit, and on a dead
// link that exit arrives only once the SSH connection closes. Closing the
// Docker client first therefore costs the full budget per idle connection.
func TestRemoteDockerSessionCloseDoesNotWaitOnIdleConnections(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	defer server.Close()
	// Registered after server.Close, so it runs first: the server's close
	// waits for this handler.
	release := make(chan struct{})
	defer close(release)
	server.setStreamHandler(serveEngineAPIIgnoringEOF(t, release))
	sshClient := server.dial(t)

	cli, err := docker.NewRemoteClient(NewSSHDockerDialer(sshClient, dialerTestLogger(t)), dialerTestLogger(t))
	if err != nil {
		t.Fatalf("NewRemoteClient: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := cli.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	session := &remoteDockerSession{sshClient: sshClient, dockerClient: cli}
	start := time.Now()
	_ = session.close()

	if elapsed := time.Since(start); elapsed >= dialCloseWaitBudget {
		t.Fatalf("session close took %v; it waited on an idle connection the SSH close would have ended", elapsed)
	}
}
