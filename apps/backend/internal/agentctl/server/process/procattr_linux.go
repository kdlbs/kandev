//go:build linux

package process

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// setProcGroup configures the command to run in its own process group.
// This allows us to kill all child processes together.
// On Linux, we also set Pdeathsig to ensure the child is killed if the parent dies
// unexpectedly (SIGKILL, crash, etc.) without calling Stop().
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGTERM,
	}
}

func setAgentProcGroup(cmd *exec.Cmd) {
	setProcGroup(cmd)
}

func setManagedProcGroup(cmd *exec.Cmd) {
	setProcGroup(cmd)
}

type processLifecycleHandle struct{}

func installProcessLifecycle(_ *exec.Cmd) (processLifecycleHandle, error) {
	return processLifecycleHandle{}, nil
}

func releaseProcessLifecycle(_ processLifecycleHandle) {}

func reapProcessLifecycle(_ processLifecycleHandle) error { return nil }

func ownsProcessLifecycle(_ processLifecycleHandle) bool { return false }

// killProcessGroup kills the entire process group for the given PID.
// Returns nil if successful, or an error if the kill failed.
func killProcessGroup(pid int) error {
	// Kill the entire process group by using negative PID
	return syscall.Kill(-pid, syscall.SIGKILL)
}

// terminateProcessGroup sends SIGTERM to the entire process group for graceful shutdown.
func terminateProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGTERM)
}

func processGroupAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return processGroupResponds(pid)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		memberPID, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		stat, err := os.ReadFile("/proc/" + strconv.Itoa(memberPID) + "/stat")
		if err == nil && processStatMatchesLiveGroup(string(stat), pid) {
			return true
		}
	}
	return false
}

func processStatMatchesLiveGroup(stat string, pgid int) bool {
	commandEnd := strings.LastIndex(stat, ") ")
	if commandEnd < 0 {
		return false
	}
	fields := strings.Fields(stat[commandEnd+2:])
	if len(fields) < 3 || fields[0] == "Z" || fields[0] == "X" {
		return false
	}
	group, err := strconv.Atoi(fields[2])
	return err == nil && group == pgid
}

func processGroupResponds(pid int) bool {
	err := syscall.Kill(-pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func isProcessGroupMissing(err error) bool {
	return errors.Is(err, syscall.ESRCH)
}
