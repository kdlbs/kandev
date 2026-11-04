//go:build darwin

package launcher

import "syscall"

func replaceServiceShell(invocation serviceShellInvocation) error {
	return syscall.Exec(invocation.Path, invocation.Args, invocation.Env)
}
