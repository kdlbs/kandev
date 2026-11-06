//go:build windows

package backendapp

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/winproc"
)

// The launcher stops a console-less backend through its cooperative shutdown
// event; the backend must treat that request as its first shutdown signal.
func TestStartupSignalControllerTreatsCooperativeShutdownAsFirstSignal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	controller := newStartupSignalController(ctx, cancel, nil)
	defer controller.stopAndWait()

	if err := winproc.RequestShutdown(os.Getpid()); err != nil {
		t.Fatalf("request cooperative shutdown: %v", err)
	}
	select {
	case sig := <-controller.first:
		if sig != os.Interrupt {
			t.Fatalf("first signal = %v, want %v", sig, os.Interrupt)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cooperative shutdown request did not reach the startup signal controller")
	}
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("startup context was not canceled by the cooperative shutdown request")
	}
}

// Releasing the controller must drop the event so a later stop request falls
// back to the caller's existing termination path instead of being swallowed.
func TestStartupSignalControllerReleasesCooperativeShutdownEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	controller := newStartupSignalController(ctx, cancel, nil)
	controller.stopAndWait()

	if err := winproc.RequestShutdown(os.Getpid()); err == nil {
		t.Fatal("request after controller release succeeded, want missing event error")
	}
}

// When the launcher and backend share a console, Ctrl+C reaches the backend
// directly and the launcher then sets the backend's cooperative event. That
// request must not count as the second signal that forces os.Exit(1).
func TestStartupSignalControllerIgnoresCooperativeRequestAfterConsoleSignal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	controller := newStartupSignalController(ctx, cancel, nil)

	controller.signals <- syscall.SIGINT
	select {
	case sig := <-controller.first:
		if sig != syscall.SIGINT {
			t.Fatalf("first signal = %v, want %v", sig, syscall.SIGINT)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("console signal did not reach the startup signal controller")
	}
	if err := winproc.RequestShutdown(os.Getpid()); err != nil {
		t.Fatalf("request cooperative shutdown during teardown: %v", err)
	}
	// A regression exits the test binary with status 1 here.
	time.Sleep(500 * time.Millisecond)
	controller.stopAndWait()
}
