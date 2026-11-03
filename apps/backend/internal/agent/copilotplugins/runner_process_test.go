//go:build !windows

package copilotplugins

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestExecRunnerCleansDescendantAfterParentExit(t *testing.T) {
	for _, exitCode := range []string{"0", "1"} {
		t.Run("exit_"+exitCode, func(t *testing.T) {
			workDir := t.TempDir()
			pidFile := filepath.Join(workDir, "descendant.pid")
			script := "sleep 30 & echo $! > \"$1\"; exit " + exitCode
			_, _ = ExecRunner{}.Run(context.Background(), []string{
				"sh", "-c", script, "sh", pidFile,
			}, map[string]string{"PATH": os.Getenv("PATH")}, workDir)
			pid := readDescendantPID(t, pidFile)
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				if processTerminated(pid) {
					return
				}
				time.Sleep(25 * time.Millisecond)
			}
			t.Fatalf("descendant pid %d survived runner cleanup", pid)
		})
	}
}

// processTerminated distinguishes a live process from a zombie. The runner
// kills the process group, but an unreaped descendant can remain visible to
// kill(pid, 0) in containers whose PID 1 does not reap promptly.
func processTerminated(pid int) bool {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err == nil {
			if closeParen := strings.LastIndex(string(data), ")"); closeParen >= 0 && len(data) > closeParen+2 {
				state := data[closeParen+2]
				return state == 'Z' || state == 'X'
			}
			return false
		}
		if os.IsNotExist(err) {
			return true
		}
	}
	return syscall.Kill(pid, 0) == syscall.ESRCH
}

func readDescendantPID(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read descendant pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		t.Fatalf("parse descendant pid %q: %v", data, err)
	}
	return pid
}

func TestProcessTerminatedTreatsZombieAsGone(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("zombie state is read from /proc")
	}
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Wait() })
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if processTerminated(cmd.Process.Pid) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("zombie pid %d reported alive", cmd.Process.Pid)
}
