//go:build unix && !linux

package launcher

import "syscall"

func buildSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		// Keep standalone agentctl out of the terminal foreground process
		// group so Ctrl+C is sequenced by the backend shutdown path.
		Setpgid: true,
	}
}
