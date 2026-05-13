//go:build windows

package process

import (
	"os/exec"
	"testing"
)

// TestWindowsPTY_DoubleCloseSafe guards against the double-free that crashed
// the backend on Windows when a terminal tab was closed (issue #894). Both
// InteractiveRunner.Stop and InteractiveRunner.wait close the PTY handle, so
// the wrapper must collapse the second call into a no-op — otherwise the
// underlying conpty library double-frees its Windows handles and triggers
// STATUS_HEAP_CORRUPTION.
func TestWindowsPTY_DoubleCloseSafe(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "exit", "0")
	pty, err := startPTYWithSize(cmd, 80, 24)
	if err != nil {
		t.Fatalf("startPTYWithSize: %v", err)
	}

	if err := pty.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	// Second Close must not panic, must not double-free, and must return the
	// same error value as the first call.
	if err := pty.Close(); err != nil {
		t.Fatalf("second Close returned error: %v (expected nil from sync.Once)", err)
	}
}
