//go:build linux

package launcher

import "syscall"

// buildSysProcAttr configures the child's process attributes. Pdeathsig is
// kill-path #2 of design 01's kill-paths list: it fires from the kernel on
// ANY parent exit, including SIGKILL, so it must be omitted entirely when
// the agent-survival capability is engaged for this launch
// (AC-EXECUTORS-SURVIVAL-001.2 forbids depending on any backend shutdown
// step). Setpgid stays set regardless -- it isolates agentctl from terminal
// Ctrl+C, unrelated to survival.
func buildSysProcAttr(survivalEnabled bool) *syscall.SysProcAttr {
	attr := &syscall.SysProcAttr{
		Setpgid: true,
	}
	if !survivalEnabled {
		attr.Pdeathsig = syscall.SIGTERM
	}
	return attr
}
