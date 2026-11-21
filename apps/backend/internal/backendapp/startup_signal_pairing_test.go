package backendapp

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

func captureStartupExit(t *testing.T) chan int {
	t.Helper()
	exits := make(chan int, 2)
	old := startupExit
	startupExit = func(code int) { exits <- code }
	t.Cleanup(func() { startupExit = old })
	return exits
}

// One Ctrl+C on a shared console reaches the backend as an OS signal and also
// makes the launcher set the backend's cooperative event. When the cooperative
// request is taken first, the OS signal from the same Ctrl+C must not count as
// the second signal; only a further Ctrl+C forces exit.
func TestStartupSignalControllerAbsorbsConsoleSignalPairedWithCooperativeStop(t *testing.T) {
	exits := captureStartupExit(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	controller := newStartupSignalController(ctx, cancel, nil)
	defer controller.stopAndWait()

	controller.cooperative <- os.Interrupt
	select {
	case <-controller.first:
	case <-time.After(5 * time.Second):
		t.Fatal("cooperative request did not reach the startup signal controller")
	}
	controller.signals <- syscall.SIGINT // same Ctrl+C as the cooperative request
	controller.signals <- syscall.SIGINT // deliberate second Ctrl+C
	select {
	case code := <-exits:
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("deliberate second signal did not force exit")
	}
	// The forced exit must come from the second signal: the paired one is consumed
	// without exiting, so neither signal is left unread.
	if pending := len(controller.signals); pending != 0 {
		t.Fatalf("forced exit fired on the paired console signal; %d signal(s) left unread", pending)
	}
}
