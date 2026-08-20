//go:build windows

package proclive

import (
	"os"
	"testing"
)

func TestAliveReportsUnknownOnWindows(t *testing.T) {
	alive, known := Alive(int64(os.Getpid()))
	if alive || known {
		t.Fatalf("Alive(self) = (%t, %t), want (false, false)", alive, known)
	}
}
