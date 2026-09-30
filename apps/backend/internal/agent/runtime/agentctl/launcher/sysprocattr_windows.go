//go:build windows

package launcher

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// buildSysProcAttr configures the child's process attributes. survivalEnabled
// is unused on this platform: the capability is unavailable on Windows (see
// agentSurvivalAvailability in runtimeflags/registry.go), and the job-object
// kill path (design 01's kill-path #3) stays unconditional regardless.
func buildSysProcAttr(_ bool) *syscall.SysProcAttr {
	// CREATE_NEW_PROCESS_GROUP so Ctrl+C doesn't propagate directly.
	// CREATE_NO_WINDOW so agentctl never opens a console window, even when the
	// backend has no console.
	return &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW,
	}
}
