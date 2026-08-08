//go:build windows

package ptyexec

import (
	"os/exec"
	"testing"
)

// TestWindowsPTY_DoubleCloseSafe pins down the sync.Once guard on
// windowsPTY.Close. The upstream UserExistsError/conpty library has no internal
// synchronization, so a second Close would double-free the underlying Windows
// handles and trigger STATUS_HEAP_CORRUPTION (0xC0000374) — the crash from issue
// #894. Both consumers of this package close from two directions (see the
// windowsPTY doc comment), so the guard has to hold here.
func TestWindowsPTY_DoubleCloseSafe(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "exit", "0")
	pty, err := Start(cmd, 80, 24)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := pty.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	// Second Close must not panic, must not double-free, and must return the
	// same memoized error value as the first call.
	if err := pty.Close(); err != nil {
		t.Fatalf("second Close returned error: %v (expected nil from sync.Once)", err)
	}
}
