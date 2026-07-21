//go:build linux

package shell

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func configureShellProcess(_ *exec.Cmd) {}

type shellProcessLifecycleHandle struct{}

func installShellProcessLifecycle(_ *exec.Cmd) (shellProcessLifecycleHandle, error) {
	return shellProcessLifecycleHandle{}, nil
}

func reapShellProcessLifecycle(_ shellProcessLifecycleHandle) error { return nil }

func ownsShellProcessLifecycle(_ shellProcessLifecycleHandle) bool { return false }

func killShellProcessGroup(p *os.Process) error {
	if p == nil {
		return nil
	}
	if err := syscall.Kill(-p.Pid, syscall.SIGKILL); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if killErr := p.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			return errors.Join(err, killErr)
		}
		return err
	}
	return nil
}

func shellProcessGroupAlive(p *os.Process) bool {
	if p == nil || p.Pid <= 0 {
		return false
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return processGroupResponds(p.Pid)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		if err == nil && processStatMatchesLiveGroup(string(stat), p.Pid) {
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
