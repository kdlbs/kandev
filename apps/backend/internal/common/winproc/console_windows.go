//go:build windows

package winproc

import (
	"sync"

	"golang.org/x/sys/windows"
)

var (
	getConsoleWindowOnce sync.Once
	getConsoleWindowProc *windows.LazyProc
)

// hasConsoleWindow reports whether this process is attached to a console that
// owns a window, visible or hidden. A child started with HideWindow inherits
// that console. Without one, Windows creates a new console for the child, and
// when Windows Terminal is the default terminal that console opens as a
// visible window regardless of HideWindow.
var hasConsoleWindow = func() bool {
	getConsoleWindowOnce.Do(func() {
		getConsoleWindowProc = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")
	})
	if getConsoleWindowProc.Find() != nil {
		return true
	}
	hwnd, _, _ := getConsoleWindowProc.Call()
	return hwnd != 0
}

// NoConsoleWindowFlag returns CREATE_NO_WINDOW when this process has no
// console window for a console child to inherit, and zero otherwise. Windows
// documents that CREATE_NO_WINDOW leaves the console handle unset; it does not
// promise a console for descendants to inherit. When a console window exists,
// returning zero preserves the caller's existing console flags.
func NoConsoleWindowFlag() uint32 {
	if hasConsoleWindow() {
		return 0
	}
	return windows.CREATE_NO_WINDOW
}
