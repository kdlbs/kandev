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

	firstErr := pty.Close()
	if firstErr != nil {
		t.Fatalf("first Close: %v", firstErr)
	}
	// The second Close must not panic and must not double-free. Assert on the
	// memoized value rather than just nil-ness: sync.Once means the second call
	// has to report whatever the first one did, so comparing the two is what
	// actually distinguishes "gated" from "ran twice and happened to succeed".
	if secondErr := pty.Close(); secondErr != firstErr {
		t.Errorf("second Close returned %v, want memoized %v", secondErr, firstErr)
	}
}
