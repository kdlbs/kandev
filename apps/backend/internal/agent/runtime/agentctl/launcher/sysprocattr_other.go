//go:build !unix && !windows

package launcher

import "syscall"

// buildSysProcAttr configures the child's process attributes. survivalEnabled
// is unused on this platform: there is no parent-death primitive here to gate.
func buildSysProcAttr(_ bool) *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}
