//go:build windows

package launcher

import "syscall"

func buildSysProcAttr() *syscall.SysProcAttr {
	// CREATE_NEW_PROCESS_GROUP so Ctrl+C doesn't propagate directly
	return &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}
