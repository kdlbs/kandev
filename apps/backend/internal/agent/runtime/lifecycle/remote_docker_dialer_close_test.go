package lifecycle

import (
	"io"
	"sync"
	"testing"
	"time"
)

// hangingSession models a remote `docker system dial-stdio` that does not exit
// when stdin closes. Wait returns only once Close is called, which is what a
// real SSH session does.
type hangingSession struct {
	mu        sync.Mutex
	closed    bool
	closeOnce sync.Once
	released  chan struct{}
}

func newHangingSession() *hangingSession {
	return &hangingSession{released: make(chan struct{})}
}

func (h *hangingSession) Wait() error {
	<-h.released
	return nil
}

func (h *hangingSession) Close() error {
	h.closeOnce.Do(func() {
		h.mu.Lock()
		h.closed = true
		h.mu.Unlock()
		close(h.released)
	})
	return nil
}

func (h *hangingSession) wasClosed() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.closed
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// TestSSHDockerConnCloseIsBounded covers a review finding: Close reaped the
// remote command before closing the session, so a command that ignores stdin
// EOF held Close forever. The caller still has to close the Docker client and
// the SSH client behind this connection, so an unbounded wait strands the
// whole cleanup chain, including after a step timeout.
func TestSSHDockerConnCloseIsBounded(t *testing.T) {
	session := newHangingSession()
	conn := &sshDockerConn{
		session: session,
		stdin:   nopWriteCloser{io.Discard},
		stdout:  emptyReader{},
		stderr:  &syncBuffer{},
		logger:  dialerTestLogger(t),
		// Shorter than the production budget so the test does not pay for it.
		closeWaitBudget: 50 * time.Millisecond,
	}

	done := make(chan error, 1)
	go func() { done <- conn.Close() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close() error = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("close path never returned; a remote that ignores stdin EOF blocks cleanup")
	}

	if !session.wasClosed() {
		t.Error("the session was not closed, so the remote command is never reaped")
	}
}

// blockingWriteCloser models an SSH channel whose remote window is full. Its
// Write cannot finish until the session is closed, so a full connection close
// must interrupt the write before it waits for stdinMu.
type blockingWriteCloser struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingWriteCloser() *blockingWriteCloser {
	return &blockingWriteCloser{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (w *blockingWriteCloser) Write(p []byte) (int, error) {
	close(w.started)
	<-w.release
	return len(p), nil
}

func (w *blockingWriteCloser) Close() error {
	w.once.Do(func() { close(w.release) })
	return nil
}

type writeInterruptingSession struct {
	*hangingSession
	stdin *blockingWriteCloser
}

func (s *writeInterruptingSession) Close() error {
	_ = s.stdin.Close()
	return s.hangingSession.Close()
}

// TestSSHDockerConnCloseInterruptsBlockedWrite covers cancellation while a
// Docker archive is blocked by the remote SSH channel window. Close must reach
// the session before waiting for the writer's stdin mutex.
func TestSSHDockerConnCloseInterruptsBlockedWrite(t *testing.T) {
	stdin := newBlockingWriteCloser()
	session := &writeInterruptingSession{
		hangingSession: newHangingSession(),
		stdin:          stdin,
	}
	conn := &sshDockerConn{
		session:         session,
		stdin:           stdin,
		stdout:          emptyReader{},
		stderr:          &syncBuffer{},
		logger:          dialerTestLogger(t),
		closeWaitBudget: 50 * time.Millisecond,
	}

	writeDone := make(chan error, 1)
	go func() {
		_, err := conn.Write([]byte("archive"))
		writeDone <- err
	}()

	select {
	case <-stdin.started:
	case <-time.After(2 * time.Second):
		t.Fatal("Write did not reach the blocked SSH channel")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- conn.Close() }()

	select {
	case <-writeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not interrupt the blocked Write")
	}
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return after interrupting the blocked Write")
	}
}

type emptyReader struct{}

func (emptyReader) Read([]byte) (int, error) { return 0, io.EOF }
