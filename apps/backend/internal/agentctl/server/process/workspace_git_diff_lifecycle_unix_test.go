//go:build unix

package process

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/subproc"
)

func TestCapDiffOutputCancellationClosesReader(t *testing.T) {
	restore := subproc.Git().SetCapForTest(1)
	defer restore()

	dir := t.TempDir()
	gitPath := filepath.Join(dir, "git")
	if err := os.WriteFile(gitPath, []byte(`#!/bin/sh
if [ "$1" = diff ]; then
  (trap '' TERM INT; while :; do sleep 1; done) &
  printf '%s\n' "$!" > "$GIT_TEST_CHILD_PID"
  printf started > "$GIT_TEST_STARTED"
  printf diff-output
  exit 0
fi
printf quick
`), 0o700); err != nil {
		t.Fatalf("write Git fixture: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	childPath := filepath.Join(dir, "child_pid")
	t.Setenv("GIT_TEST_CHILD_PID", childPath)
	t.Setenv("GIT_TEST_STARTED", filepath.Join(dir, "started"))

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan struct {
		output    string
		truncated bool
	}, 1)
	go func() {
		output, truncated := capDiffOutput(ctx, dir, "diff")
		result <- struct {
			output    string
			truncated bool
		}{output, truncated}
	}()
	waitForProcessGitFile(t, filepath.Join(dir, "started"), time.Second)
	cancel()

	select {
	case value := <-result:
		if value.output == "" || value.truncated {
			t.Fatalf("capDiffOutput result = (%q, %t), want partial output without truncation", value.output, value.truncated)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("capDiffOutput did not return after cancellation closed its reader")
	}

	childPID := readProcessGitPID(t, childPath)
	waitForProcessGitPIDExit(t, childPID, time.Second)

	quickCtx, quickCancel := context.WithTimeout(context.Background(), time.Second)
	defer quickCancel()
	quickCmd := subproc.NewGitCommand(quickCtx, "version")
	quickCmd.Env = []string{"PATH=" + os.Getenv("PATH")}
	output, runErr, execErr := subproc.RunGitOutputAfterAcquire(quickCtx, subproc.GitInteractive, time.Second, func(context.Context) *exec.Cmd {
		return quickCmd
	})
	if runErr != nil || execErr != nil {
		t.Fatalf("Git slot was not released after capDiffOutput cancellation: run=%v exec=%v output=%q", runErr, execErr, output)
	}
}

func waitForProcessGitFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for %s", path)
		}
	}
}

func readProcessGitPID(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Git child PID: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parse Git child PID: %v", err)
	}
	return pid
}

func waitForProcessGitPIDExit(t *testing.T, pid int, timeout time.Duration) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("Git child %d remained alive", pid)
		}
	}
}
