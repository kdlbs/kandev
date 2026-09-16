package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/common/logger"
)

func dialerTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

// serveEngineAPI answers the Engine API over one stream, which is the shape
// `docker system dial-stdio` presents: an HTTP conversation on stdin/stdout
// with no listener and no address.
func serveEngineAPI(t *testing.T, apiVersion string) sshStreamHandler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Api-Version", apiVersion)
		w.Header().Set("Ostype", "linux")
		if strings.HasSuffix(r.URL.Path, "/version") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"ApiVersion": apiVersion})
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	return func(_ string, stream ssh.Channel) int {
		clientSide, serverSide := net.Pipe()
		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = server.Serve(newOneShotListener(serverSide))
		}()
		// Closing the pipe when the remote stream ends is what lets both
		// directions unwind; without it the response copy blocks forever on a
		// pipe the HTTP server still holds open.
		go func() {
			_, _ = io.Copy(clientSide, stream)
			_ = clientSide.Close()
		}()
		_, _ = io.Copy(stream, clientSide)
		<-done
		return 0
	}
}

type oneShotListener struct {
	conn net.Conn
	done chan struct{}
}

func newOneShotListener(c net.Conn) net.Listener {
	return &oneShotListener{conn: c, done: make(chan struct{})}
}

func (l *oneShotListener) Accept() (net.Conn, error) {
	select {
	case <-l.done:
		return nil, net.ErrClosed
	default:
	}
	close(l.done)
	return l.conn, nil
}

func (l *oneShotListener) Close() error   { return nil }
func (l *oneShotListener) Addr() net.Addr { return l.conn.LocalAddr() }

// TestSSHDockerDialerCarriesEngineAPI is the end-to-end contract for the
// transport: a Docker client built on this dialer reaches a daemon that exists
// only on the far side of an SSH connection.
func TestSSHDockerDialerCarriesEngineAPI(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	defer server.Close()
	server.setStreamHandler(serveEngineAPI(t, "1.51"))

	sshClient := server.dial(t)
	defer func() { _ = sshClient.Close() }()

	cli, err := docker.NewRemoteClient(NewSSHDockerDialer(sshClient, dialerTestLogger(t)), dialerTestLogger(t))
	if err != nil {
		t.Fatalf("NewRemoteClient: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := cli.Ping(ctx); err != nil {
		t.Fatalf("Ping over SSH transport: %v", err)
	}

	call, ok := server.lastCommandContaining("dial-stdio")
	if !ok {
		t.Fatalf("no dial-stdio command ran; commands = %v", server.commands())
	}
	if !strings.Contains(call.Command, "docker system dial-stdio") {
		t.Fatalf("command = %q, want it to invoke docker system dial-stdio", call.Command)
	}
}

// TestSSHDockerDialerReportsMissingCLI proves the most common first-run
// failures stay distinguishable through the transport. The SSH user reaching
// the host but not the socket, and the host having no docker command, need
// different fixes, so a generic "daemon unavailable" is not good enough.
func TestSSHDockerDialerReportsMissingCLI(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
		stderr   string
		want     error
	}{
		{
			name:     "docker cli absent",
			exitCode: 127,
			stderr:   "bash: docker: command not found",
			want:     docker.ErrRemoteDockerCLIMissing,
		},
		{
			name:     "socket permission denied",
			exitCode: 1,
			stderr:   "permission denied while trying to connect to the Docker daemon socket",
			want:     docker.ErrRemoteSocketAccessDenied,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := newFakeSSHServer(t, nil)
			defer server.Close()
			// A missing CLI or a denied socket exits immediately without
			// reading stdin. The canned exec handler reads stdin to EOF
			// first, which no real failing command does and which would
			// deadlock against a client waiting for a response.
			server.setStreamHandler(func(_ string, stream ssh.Channel) int {
				_, _ = io.WriteString(stream.Stderr(), tc.stderr)
				return tc.exitCode
			})

			sshClient := server.dial(t)
			defer func() { _ = sshClient.Close() }()

			dial := NewSSHDockerDialer(sshClient, dialerTestLogger(t))
			cli, err := docker.NewRemoteClient(dial, dialerTestLogger(t))
			if err != nil {
				t.Fatalf("NewRemoteClient: %v", err)
			}
			t.Cleanup(func() { _ = cli.Close() })

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			pingErr := cli.Ping(ctx)
			if pingErr == nil {
				t.Fatal("Ping succeeded against a failing remote command")
			}
			if !errors.Is(pingErr, tc.want) {
				t.Fatalf("Ping error = %v, want it to wrap %v", pingErr, tc.want)
			}
		})
	}
}

// TestSSHDockerDialerRejectsClosedConnection fails fast when the SSH
// connection is already gone, rather than returning a connection that hangs.
func TestSSHDockerDialerRejectsClosedConnection(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	sshClient := server.dial(t)
	_ = sshClient.Close()
	server.Close()

	dial := NewSSHDockerDialer(sshClient, dialerTestLogger(t))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := dial(ctx, "tcp", "docker.example.invalid:80"); err == nil {
		t.Fatal("dial on a closed SSH connection = nil error, want error")
	}
}
