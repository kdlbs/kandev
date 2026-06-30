//go:build !windows

package shell

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStopKillsShellProcessGroupOnTimeout(t *testing.T) {
	sh, err := exec.LookPath("sh")
	require.NoError(t, err)

	pidFile := filepath.Join(t.TempDir(), "child.pid")
	script := fmt.Sprintf("sleep 30 & echo $! > %s; trap '' HUP TERM; wait", shellQuote(pidFile))
	session, err := NewSession(Config{
		WorkDir:      t.TempDir(),
		ShellCommand: sh,
		ShellArgs:    []string{"-c", script},
	}, newTestLogger())
	require.NoError(t, err)

	childPID := waitForShellChildPID(t, pidFile)
	require.NoError(t, session.Stop())
	require.Eventually(t, func() bool {
		return !shellProcessAlive(childPID)
	}, 5*time.Second, 50*time.Millisecond, "shell child process %d should be killed", childPID)
}

func TestStopKillsShellProcessGroupAfterLeaderExits(t *testing.T) {
	sh, err := exec.LookPath("sh")
	require.NoError(t, err)

	pidFile := filepath.Join(t.TempDir(), "child.pid")
	script := fmt.Sprintf("trap 'exit 0' HUP TERM; (trap '' HUP TERM; sleep 30) & echo $! > %s; wait", shellQuote(pidFile))
	session, err := NewSession(Config{
		WorkDir:      t.TempDir(),
		ShellCommand: sh,
		ShellArgs:    []string{"-c", script},
	}, newTestLogger())
	require.NoError(t, err)

	childPID := waitForShellChildPID(t, pidFile)
	require.NoError(t, session.Stop())
	require.Eventually(t, func() bool {
		return !shellProcessAlive(childPID)
	}, 5*time.Second, 50*time.Millisecond, "shell child process %d should be killed after shell leader exits", childPID)
}

func waitForShellChildPID(t *testing.T, pidFile string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(pidFile)
		if err == nil {
			pid, perr := strconv.Atoi(strings.TrimSpace(string(raw)))
			if perr == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for shell child pid file %s", pidFile)
	return 0
}

func shellProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err == nil {
		raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err != nil {
			return false
		}
		stat := string(raw)
		idx := strings.LastIndex(stat, ") ")
		if idx == -1 || idx+2 >= len(stat) {
			return false
		}
		return stat[idx+2] != 'Z'
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
