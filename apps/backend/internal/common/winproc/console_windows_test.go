//go:build windows

package winproc

import (
	"testing"

	"golang.org/x/sys/windows"
)

func withConsoleWindow(t *testing.T, present bool) {
	t.Helper()
	previous := hasConsoleWindow
	hasConsoleWindow = func() bool { return present }
	t.Cleanup(func() { hasConsoleWindow = previous })
}

func TestNoConsoleWindowFlagLeavesFlagsWithConsoleWindow(t *testing.T) {
	withConsoleWindow(t, true)
	if got := NoConsoleWindowFlag(); got != 0 {
		t.Fatalf("NoConsoleWindowFlag() = %#x with a console window, want 0", got)
	}
}

func TestNoConsoleWindowFlagSuppressesWindowWithoutConsole(t *testing.T) {
	withConsoleWindow(t, false)
	if got := NoConsoleWindowFlag(); got != windows.CREATE_NO_WINDOW {
		t.Fatalf("NoConsoleWindowFlag() = %#x without a console window, want CREATE_NO_WINDOW", got)
	}
}
