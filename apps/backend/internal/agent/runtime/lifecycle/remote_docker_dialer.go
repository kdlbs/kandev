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

// NewSSHDockerDialer returns a transport that carries the Docker Engine API
// over an existing Kandev SSH connection.
//
// This deliberately does not use the Docker CLI's connection helper, which
// shells out to the system ssh binary. Kandev already owns native SSH dialing
// with host-key pinning, IdentityAgent expansion, and ProxyJump, and a second
// SSH implementation would not share any of it.
func NewSSHDockerDialer(client *ssh.Client, log *logger.Logger) docker.RemoteTransport {
	d := &sshDockerDialer{client: client, logger: log}
	return docker.RemoteTransport{Dial: d.dial, Cause: d.cause}
}

// sshDockerDialer owns the connections it opens for one SSH client and retains
// the classified cause of the last `docker system dial-stdio` command that
// failed, because the Engine API client drops it from the error it returns.
type sshDockerDialer struct {
	client *ssh.Client
	logger *logger.Logger

	mu   sync.Mutex
	last error
}

func (d *sshDockerDialer) dial(ctx context.Context, _, _ string) (net.Conn, error) {
	return dialDockerOverSSH(ctx, d.client, d.logger, d.record)
}

// record keeps the newest connection outcome. A failure is logged, so the
// remote's own words reach the operator even when a caller only sees the
// Engine API client's wording; a clean exit (nil) clears it, so one
// connection's failure cannot explain a later, unrelated one.
func (d *sshDockerDialer) record(err error) {
	if err != nil {
		d.logger.Warn("remote docker: dial-stdio failed", zap.Error(err))
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.last = err
}

func (d *sshDockerDialer) cause() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.last
}

// The context bounds the dial itself. It deliberately does not bound the
// returned connection: http.Transport pools connections across requests, so
// tearing one down when its dialing request ends would close a connection
// that later requests are still using.
func dialDockerOverSSH(ctx context.Context, client *ssh.Client, log *logger.Logger, onExit func(error)) (net.Conn, error) {
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
		onExit:  onExit,
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
	// onExit reports the classified failure of the remote command to the
	// dialer that opened this connection. Optional.
	onExit func(error)

	// closeWaitBudget overrides dialCloseWaitBudget; zero means the default.
	closeWaitBudget time.Duration

	mu       sync.Mutex
	exitErr  error
	exited   bool
	closed   bool
	waitOnce sync.Once

	// stdinMu serializes stdin. x/crypto/ssh's sessionStdin is not safe for a
	// concurrent Write and Close, and http.Transport drives exactly that: its
	// write loop writes the request while its read loop closes the connection.
	stdinMu     sync.Mutex
	stdinClosed bool
}

// closeStdin half-closes the write side once. Repeat calls are a no-op rather
// than a second Close on the channel, which is what CloseWrite followed by
// Close would otherwise do.
func (c *sshDockerConn) closeStdin() error {
	c.stdinMu.Lock()
	defer c.stdinMu.Unlock()
	if c.stdinClosed {
		return nil
	}
	c.stdinClosed = true
	return c.stdin.Close()
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
	c.stdinMu.Lock()
	if c.stdinClosed {
		c.stdinMu.Unlock()
		if cause := c.exitCause(); cause != nil {
			return 0, cause
		}
		return 0, io.ErrClosedPipe
	}
	n, err := c.stdin.Write(p)
	c.stdinMu.Unlock()
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
	return c.closeStdin()
}

func (c *sshDockerConn) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	// Close the SSH session first. A Docker request can be blocked in
	// stdin.Write while the remote channel window is full; waiting for
	// stdinMu before closing the session would leave that write with no way to
	// make progress. Session.Close interrupts the channel write and the later
	// state update prevents any new writes from entering it.
	sessionErr := c.session.Close()
	c.stdinMu.Lock()
	c.stdinClosed = true
	c.stdinMu.Unlock()
	// The closed session also unblocks a Wait that is not going to return on
	// its own, so a remote command that ignores stdin EOF costs a bounded delay
	// rather than stranding the caller's cleanup.
	c.waitBounded(c.closeBudget())
	return sessionErr
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
		c.mu.Lock()
		closedLocally := c.closed
		c.mu.Unlock()

		cause, report := classifyDialStdioExit(err, c.stderr.String(), closedLocally)
		c.mu.Lock()
		c.exited = true
		c.exitErr = cause
		c.mu.Unlock()
		if report && c.onExit != nil {
			c.onExit(cause)
		}
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

// classifyDialStdioExit turns the remote command's Wait result into a cause,
// and reports whether the outcome says anything about the transport.
//
// Only an exit status comes from the remote command itself. Any other result,
// such as a channel that closed without one, means the SSH transport ended
// under the command, so it is reported as transport loss rather than as a
// daemon failure. When this side closed the connection, that is expected and
// reports nothing.
func classifyDialStdioExit(err error, stderr string, closedLocally bool) (error, bool) {
	if err == nil {
		return nil, true
	}
	var exitErr *ssh.ExitError
	if asExitError(err, &exitErr) {
		return docker.ClassifyRemoteDialError(exitErr.ExitStatus(), stderr), true
	}
	if closedLocally {
		return nil, false
	}
	return fmt.Errorf("%w: %q ended without an exit status: %v", ErrSSHTransportLost, dockerDialStdioCommand, err), true
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
