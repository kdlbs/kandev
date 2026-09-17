package lifecycle

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"

	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/common/logger"
)

// dockerDialStdioCommand is the Docker CLI subcommand that proxies the Engine
// API over stdin/stdout. It requires the CLI on the remote host, not only a
// daemon, which is why a missing command is its own reported cause.
const dockerDialStdioCommand = "docker system dial-stdio"

// NewSSHDockerDialer returns a dialer that carries the Docker Engine API over
// an existing Kandev SSH connection.
//
// This deliberately does not use the Docker CLI's connection helper, which
// shells out to the system ssh binary. Kandev already owns native SSH dialing
// with host-key pinning, IdentityAgent expansion, and ProxyJump, and a second
// SSH implementation would not share any of it.
func NewSSHDockerDialer(client *ssh.Client, log *logger.Logger) docker.DialContextFunc {
	return func(ctx context.Context, _, _ string) (net.Conn, error) {
		return dialDockerOverSSH(ctx, client, log)
	}
}

// The context bounds the dial itself. It deliberately does not bound the
// returned connection: http.Transport pools connections across requests, so
// tearing one down when its dialing request ends would close a connection
// that later requests are still using.
func dialDockerOverSSH(ctx context.Context, client *ssh.Client, log *logger.Logger) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if client == nil {
		return nil, fmt.Errorf("remote docker: no SSH connection")
	}

	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("remote docker: open SSH session: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("remote docker: stdin pipe: %w", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("remote docker: stdout pipe: %w", err)
	}

	stderr := &syncBuffer{}
	session.Stderr = stderr

	if err := session.Start(dockerDialStdioCommand); err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("remote docker: start %q: %w", dockerDialStdioCommand, err)
	}

	return &sshDockerConn{
		session: session,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		logger:  log,
	}, nil
}

// sshDockerConn adapts one `docker system dial-stdio` session to net.Conn.
//
// The remote command's exit status is the only place a transport-level cause
// appears: a missing CLI or a denied socket produces a clean stream that
// simply ends, so without capturing it the caller sees an unexplained EOF.
// dialSession is the part of an SSH session this adapter uses. Narrowed from
// *ssh.Session so the close path's bounded wait is testable without a host.
type dialSession interface {
	Wait() error
	Close() error
}

// dialCloseWaitBudget bounds how long Close waits for the remote command to
// reap after stdin EOF. A remote that never exits must not be able to hold the
// caller, which still has to close the Docker client and the SSH client behind
// this connection.
const dialCloseWaitBudget = 5 * time.Second

type sshDockerConn struct {
	session dialSession
	stdin   io.WriteCloser
	stdout  io.Reader
	stderr  *syncBuffer
	logger  *logger.Logger

	// closeWaitBudget overrides dialCloseWaitBudget; zero means the default.
	closeWaitBudget time.Duration

	mu       sync.Mutex
	exitErr  error
	exited   bool
	closed   bool
	waitOnce sync.Once
}

func (c *sshDockerConn) Read(p []byte) (int, error) {
	n, err := c.stdout.Read(p)
	if err != nil && err != io.EOF {
		return n, err
	}
	if err == io.EOF {
		if cause := c.exitCause(); cause != nil {
			return n, cause
		}
	}
	return n, err
}

func (c *sshDockerConn) Write(p []byte) (int, error) {
	n, err := c.stdin.Write(p)
	if err != nil {
		if cause := c.exitCause(); cause != nil {
			return n, cause
		}
	}
	return n, err
}

// CloseWrite half-closes the stream so the remote sees stdin EOF while the
// response is still draining. http.Transport relies on this for request
// bodies; closing the whole session instead truncates the response.
func (c *sshDockerConn) CloseWrite() error {
	return c.stdin.Close()
}

func (c *sshDockerConn) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	_ = c.stdin.Close()
	// Closing the session unblocks a Wait that is not going to return on its
	// own, so a remote command that ignores stdin EOF costs a bounded delay
	// rather than stranding the caller's cleanup.
	c.waitBounded(c.closeBudget())
	return c.session.Close()
}

// closeBudget is the wait this connection allows on close.
func (c *sshDockerConn) closeBudget() time.Duration {
	if c.closeWaitBudget > 0 {
		return c.closeWaitBudget
	}
	return dialCloseWaitBudget
}

// waitBounded reaps the remote command, giving up after budget so Close can
// proceed. The reaper keeps running: it still records the exit cause if the
// command exits later, and Wait returns once the session is closed.
func (c *sshDockerConn) waitBounded(budget time.Duration) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.wait()
	}()
	timer := time.NewTimer(budget)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		c.logger.Warn("remote docker: dial-stdio did not exit after stdin EOF; closing the session",
			zap.Duration("budget", budget))
	}
}

// wait reaps the remote command once and classifies its failure.
func (c *sshDockerConn) wait() {
	c.waitOnce.Do(func() {
		err := c.session.Wait()
		exitCode := 0
		var exitErr *ssh.ExitError
		switch {
		case err == nil:
		case asExitError(err, &exitErr):
			exitCode = exitErr.ExitStatus()
		default:
			exitCode = -1
		}

		cause := docker.ClassifyRemoteDialError(exitCode, c.stderr.String())
		c.mu.Lock()
		c.exited = true
		c.exitErr = cause
		c.mu.Unlock()
	})
}

func (c *sshDockerConn) exitCause() error {
	c.wait()
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.exited {
		return nil
	}
	return c.exitErr
}

func asExitError(err error, target **ssh.ExitError) bool {
	if e, ok := err.(*ssh.ExitError); ok {
		*target = e
		return true
	}
	return false
}

func (c *sshDockerConn) LocalAddr() net.Addr  { return sshDockerAddr{} }
func (c *sshDockerConn) RemoteAddr() net.Addr { return sshDockerAddr{} }

// Deadlines are not supported on an SSH session. They are accepted as no-ops
// rather than errors because http.Transport sets them routinely and would
// otherwise fail every request; request cancellation runs through the
// context instead.
func (c *sshDockerConn) SetDeadline(time.Time) error      { return nil }
func (c *sshDockerConn) SetReadDeadline(time.Time) error  { return nil }
func (c *sshDockerConn) SetWriteDeadline(time.Time) error { return nil }

type sshDockerAddr struct{}

func (sshDockerAddr) Network() string { return "ssh" }
func (sshDockerAddr) String() string  { return dockerDialStdioCommand }
