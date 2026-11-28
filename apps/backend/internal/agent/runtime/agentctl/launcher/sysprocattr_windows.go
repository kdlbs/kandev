//go:build windows

package launcher

import (
	"syscall"

	"github.com/kandev/kandev/internal/common/winproc"

	"golang.org/x/sys/windows"
)

// buildSysProcAttr configures the child's process attributes. survivalEnabled
// is unused on this platform: the capability is unavailable on Windows (see
// agentSurvivalAvailability in runtimeflags/registry.go), and the job-object
// kill path (design 01's kill-path #3) stays unconditional regardless.
func buildSysProcAttr(_ bool) *syscall.SysProcAttr {
	// CREATE_NEW_PROCESS_GROUP so Ctrl+C doesn't propagate directly. HideWindow
	// hides a newly created console without detaching agentctl from that console;
	// without a console window to share, CREATE_NO_WINDOW keeps it off the desktop.
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_SUSPENDED | winproc.NoConsoleWindowFlag(),
	}
}
