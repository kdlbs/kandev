package loginpty

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

func newTestManager(t *testing.T, onExit func(string, int, error)) *Manager {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return NewManager(log, onExit)
}

// drainOutput consumes the subscriber channel until it closes; returns all bytes received.
func drainOutput(sess *Session) []byte {
	ch := make(chan []byte, 32)
	sess.Subscribe(ch)
	defer sess.Unsubscribe(ch)

	var buf bytes.Buffer
	deadline := time.After(2 * time.Second)
	for {
		select {
		case data, ok := <-ch:
			if !ok {
				return buf.Bytes()
			}
			buf.Write(data)
		case <-deadline:
			return buf.Bytes()
		}
	}
}

func TestStart_RunsCommandAndExits(t *testing.T) {
	exited := make(chan struct {
		agent string
		code  int
	}, 1)
	mgr := newTestManager(t, func(agent string, code int, _ error) {
		exited <- struct {
			agent string
			code  int
		}{agent, code}
	})

	sess, err := mgr.Start("test-agent", []string{"sh", "-c", "echo hello && exit 0"}, 80, 24)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if sess.ID == "" {
		t.Fatal("expected session id")
	}

	// Subscribe right away to catch the output (some may be missed if we race
	// the readLoop; BufferedOutput() compensates after exit).
	got := drainOutput(sess)

	select {
	case info := <-exited:
		if info.agent != "test-agent" {
			t.Errorf("agent = %s", info.agent)
		}
		if info.code != 0 {
			t.Errorf("code = %d, want 0", info.code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("supervise loop did not fire onExit")
	}

	// Output may have been received via the subscriber or buffered; check both.
	combined := append([]byte(nil), got...)
	combined = append(combined, sess.BufferedOutput()...)
	if !bytes.Contains(combined, []byte("hello")) {
		t.Errorf("expected 'hello' in output, got %q", combined)
	}
}

func TestStart_IdempotentPerAgent(t *testing.T) {
	mgr := newTestManager(t, nil)

	first, err := mgr.Start("test-agent", []string{"sh", "-c", "sleep 5"}, 80, 24)
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Stop("test-agent") })

	second, err := mgr.Start("test-agent", []string{"sh", "-c", "sleep 5"}, 80, 24)
	if !errors.Is(err, ErrSessionAlreadyRunning) {
		t.Fatalf("err = %v, want ErrSessionAlreadyRunning", err)
	}
	if first.ID != second.ID {
		t.Errorf("expected same session id, got %s and %s", first.ID, second.ID)
	}
}

func TestStart_EmptyCommand(t *testing.T) {
	mgr := newTestManager(t, nil)
	_, err := mgr.Start("test-agent", nil, 80, 24)
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestStop_TerminatesRunningSession(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	mgr := newTestManager(t, func(_ string, _ int, _ error) {
		wg.Done()
	})

	_, err := mgr.Start("test-agent", []string{"sh", "-c", "sleep 30"}, 80, 24)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := mgr.Stop("test-agent"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("session did not exit after Stop")
	}
}

func TestWriteAndResize_WhenNotRunning(t *testing.T) {
	mgr := newTestManager(t, nil)
	sess, err := mgr.Start("test-agent", []string{"true"}, 80, 24)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Wait for exit.
	deadline := time.After(2 * time.Second)
	for sess.Status().Running {
		select {
		case <-deadline:
			t.Fatal("session never exited")
		case <-time.After(20 * time.Millisecond):
		}
	}
	if _, err := sess.Write([]byte("x")); !errors.Is(err, ErrSessionNotRunning) {
		t.Errorf("Write err = %v, want ErrSessionNotRunning", err)
	}
	if err := sess.Resize(120, 40); !errors.Is(err, ErrSessionNotRunning) {
		t.Errorf("Resize err = %v, want ErrSessionNotRunning", err)
	}
}
