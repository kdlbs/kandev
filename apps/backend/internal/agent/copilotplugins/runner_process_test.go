//go:build !windows

package copilotplugins

import (
	"context"
	"os"
	"path/filepath"
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
				if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
					return
				}
				time.Sleep(25 * time.Millisecond)
			}
			t.Fatalf("descendant pid %d survived runner cleanup", pid)
		})
	}
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
