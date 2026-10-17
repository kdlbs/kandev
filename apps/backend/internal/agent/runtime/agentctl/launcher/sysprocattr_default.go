//go:build unix && !linux

package launcher

import "syscall"

// buildSysProcAttr configures the child's process attributes. survivalEnabled
// is unused on this platform: syscall.SysProcAttr here carries no Pdeathsig
// field (that kill path is Linux-only), so there is nothing to gate.
func buildSysProcAttr(_ bool) *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		// Isolate the managed control server and all of its descendants from
		// the terminal session so cleanup can target only this runtime tree.
		Setsid: true,
	}
}
