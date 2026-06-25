//go:build !unix && !windows

package launcher

import "syscall"

func buildSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}
