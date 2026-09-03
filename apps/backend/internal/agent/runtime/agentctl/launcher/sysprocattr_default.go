//go:build unix && !linux

package launcher

import "syscall"

// buildSysProcAttr configures the child's process attributes. survivalEnabled
// is unused on this platform: syscall.SysProcAttr here carries no Pdeathsig
// field (that kill path is Linux-only), so there is nothing to gate.
func buildSysProcAttr(_ bool) *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		// Keep standalone agentctl out of the terminal foreground process
		// group so Ctrl+C is sequenced by the backend shutdown path.
		Setpgid: true,
	}
}
