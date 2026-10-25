package lifecycle

import (
	"errors"
	"io"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/agent/docker"
)

// resultSession is a dial-stdio session whose command has already finished
// with a fixed Wait result.
type resultSession struct{ err error }

func (s resultSession) Wait() error  { return s.err }
func (s resultSession) Close() error { return nil }

func newResultConn(t *testing.T, waitErr error, onExit func(error)) *sshDockerConn {
	t.Helper()
	return &sshDockerConn{
		session: resultSession{err: waitErr},
		stdin:   nopWriteCloser{io.Discard},
		stdout:  emptyReader{},
		stderr:  &syncBuffer{},
		logger:  dialerTestLogger(t),
		onExit:  onExit,
	}
}

// TestSSHDockerConnReportsAMissingExitStatusAsTransportLoss keeps a dropped SSH
// channel from being reported as a Docker daemon problem. A channel that closes
// without an exit status never ran to completion on the remote, so the daemon
// said nothing about itself.
func TestSSHDockerConnReportsAMissingExitStatusAsTransportLoss(t *testing.T) {
	var reported []error
	conn := newResultConn(t, &ssh.ExitMissingError{}, func(err error) { reported = append(reported, err) })

	cause := conn.exitCause()

	if !errors.Is(cause, ErrSSHTransportLost) {
		t.Fatalf("exitCause() = %v, want ErrSSHTransportLost", cause)
	}
	if errors.Is(cause, docker.ErrRemoteDaemonUnreachable) {
		t.Fatalf("exitCause() = %v, a dropped channel must not name the daemon", cause)
	}
	if len(reported) != 1 || !errors.Is(reported[0], ErrSSHTransportLost) {
		t.Fatalf("onExit received %v, want one transport-loss cause", reported)
	}
}

// TestSSHDockerConnClosedLocallyReportsNothing covers this side ending the
// session. Close tears the channel down, which is what makes Wait return
// without an exit status; recording that as a failure would pin a cause no
// request ever hit.
func TestSSHDockerConnClosedLocallyReportsNothing(t *testing.T) {
	var reported []error
	conn := newResultConn(t, &ssh.ExitMissingError{}, func(err error) { reported = append(reported, err) })

	if err := conn.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if cause := conn.exitCause(); cause != nil {
		t.Fatalf("exitCause() = %v after a local close, want nil", cause)
	}
	if len(reported) != 0 {
		t.Fatalf("onExit received %v after a local close, want nothing", reported)
	}
}

// TestSSHDockerDialerClearsACauseOnACleanExit keeps an earlier connection's
// failure from explaining a later, unrelated one.
func TestSSHDockerDialerClearsACauseOnACleanExit(t *testing.T) {
	d := &sshDockerDialer{logger: dialerTestLogger(t)}

	newResultConn(t, &ssh.ExitMissingError{}, d.record).wait()
	if d.cause() == nil {
		t.Fatal("a failed connection recorded no cause")
	}

	newResultConn(t, nil, d.record).wait()

	if cause := d.cause(); cause != nil {
		t.Fatalf("cause() = %v after a clean exit, want the stale failure cleared", cause)
	}
}
