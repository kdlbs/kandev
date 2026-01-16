//go:build linux

package launcher

import "syscall"

func buildSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGTERM,
		Setpgid:   true,
	}
}
