//go:build linux

package launcher

import "syscall"

// buildSysProcAttr configures the child's process attributes. Pdeathsig is
// kill-path #2 of design 01's kill-paths list: it fires from the kernel on
// ANY parent exit, including SIGKILL, so it must be omitted entirely when
// the agent-survival capability is engaged for this launch
// (AC-EXECUTORS-SURVIVAL-001.2 forbids depending on any backend shutdown
// step). Setsid stays set regardless because it gives cleanup a process
// session owned by this runtime instead of the backend's terminal session.
func buildSysProcAttr(survivalEnabled bool) *syscall.SysProcAttr {
	attr := &syscall.SysProcAttr{
		Setsid: true,
	}
	if !survivalEnabled {
		attr.Pdeathsig = syscall.SIGTERM
	}
	return attr
}
