//go:build unix

package subproc

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
)

func TestGitHelperCancellationReleasesSlot(t *testing.T) {
	restore := Git().SetCapForTest(1)
	defer restore()

	fakeDir, env := writeGitLifecycleFixture(t, `#!/bin/sh
if [ "$1" = hold ]; then
  (trap '' TERM INT; while :; do sleep 1; done) &
  printf '%s\n' "$!" > "$GIT_TEST_CHILD_PID"
  printf started > "$GIT_TEST_STARTED"
  wait
  exit 0
fi
printf quick
`)
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan struct {
		output  []byte
		runErr  error
		execErr error
	}, 1)
	go func() {
		output, runErr, execErr := RunGitOutputAfterAcquire(ctx, GitLifecycle, time.Second, func(execCtx context.Context) *exec.Cmd {
			cmd := NewGitCommand(execCtx, "hold")
			cmd.Env = append([]string(nil), env...)
			return cmd
		})
		result <- struct {
			output  []byte
			runErr  error
			execErr error
		}{output, runErr, execErr}
	}()

	waitForTestFile(t, filepath.Join(fakeDir, "started"), time.Second)
	cancel()
	resultValue := receiveGitLifecycleResult(t, result, time.Second)
	if resultValue.runErr == nil && resultValue.execErr == nil {
		t.Fatal("canceled Git helper unexpectedly succeeded")
	}

	quickCtx, quickCancel := context.WithTimeout(context.Background(), time.Second)
	defer quickCancel()
	quick, runErr, execErr := RunGitOutputAfterAcquire(quickCtx, GitLifecycle, time.Second, func(execCtx context.Context) *exec.Cmd {
		cmd := NewGitCommand(execCtx, "quick")
		cmd.Env = append([]string(nil), env...)
		return cmd
	})
	if runErr != nil || execErr != nil {
		t.Fatalf("Git slot was not released after cancellation: run=%v exec=%v output=%q", runErr, execErr, quick)
	}
	if string(quick) != "quick" {
		t.Fatalf("quick Git output = %q, want quick", quick)
	}
	killTestPID(t, filepath.Join(fakeDir, "child_pid"))
}

func TestGitDescendantPipeCleanup(t *testing.T) {
	fakeDir, env := writeGitLifecycleFixture(t, `#!/bin/sh
if [ "$1" = descendant ]; then
  (trap '' TERM INT; while :; do sleep 1; done) &
  printf '%s\n' "$!" > "$GIT_TEST_CHILD_PID"
  printf started > "$GIT_TEST_STARTED"
  printf parent-exited
  exit 0
fi
printf quick
`)
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	control := exec.Command("/bin/sh", "-c", "sleep 10")
	if err := control.Start(); err != nil {
		t.Fatalf("start control process: %v", err)
	}
	defer func() {
		_ = control.Process.Kill()
		_ = control.Wait()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan struct {
		output  []byte
		runErr  error
		execErr error
	}, 1)
	go func() {
		output, runErr, execErr := RunGitOutputAfterAcquire(ctx, GitLifecycle, time.Second, func(execCtx context.Context) *exec.Cmd {
			cmd := NewGitCommand(execCtx, "descendant")
			cmd.Env = append([]string(nil), env...)
			return cmd
		})
		result <- struct {
			output  []byte
			runErr  error
			execErr error
		}{output, runErr, execErr}
	}()

	waitForTestFile(t, filepath.Join(fakeDir, "started"), time.Second)
	cancel()
	resultValue := receiveGitLifecycleResult(t, result, 2*time.Second)
	if resultValue.runErr == nil && resultValue.execErr == nil {
		t.Fatal("descendant Git command unexpectedly succeeded after cancellation")
	}

	childPID := readTestPID(t, filepath.Join(fakeDir, "child_pid"))
	waitForPIDExit(t, childPID, time.Second)
	if err := control.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("unrelated control process was terminated: %v", err)
	}
}

func TestGitDescendantPipeCleanupAfterParentExit(t *testing.T) {
	fakeDir, env := writeGitLifecycleFixture(t, `#!/bin/sh
if [ "$1" = descendant ]; then
  (trap '' TERM INT; while :; do sleep 1; done) &
  printf '%s\n' "$!" > "$GIT_TEST_CHILD_PID"
  printf started > "$GIT_TEST_STARTED"
  printf parent-exited
  exit 0
fi
printf quick
`)
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	output, runErr, execErr := RunGitOutputAfterAcquire(
		context.Background(), GitLifecycle, time.Second,
		func(execCtx context.Context) *exec.Cmd {
			cmd := NewGitCommand(execCtx, "descendant")
			cmd.Env = append([]string(nil), env...)
			return cmd
		},
	)
	if execErr != nil {
		t.Fatalf("normal-exit Git execution context = %v", execErr)
	}
	if !errors.Is(runErr, exec.ErrWaitDelay) {
		t.Fatalf("normal-exit Git run error = %v, want exec.ErrWaitDelay from inherited pipe", runErr)
	}
	if !strings.Contains(string(output), "parent-exited") {
		t.Fatalf("normal-exit Git output = %q, want parent output", output)
	}
	childPID := readTestPID(t, filepath.Join(fakeDir, "child_pid"))
	waitForPIDExit(t, childPID, time.Second)
}

func TestGitLifecycleStartFailureReleasesSlot(t *testing.T) {
	restore := Git().SetCapForTest(1)
	defer restore()

	missing := filepath.Join(t.TempDir(), "missing-git")
	cmd := exec.CommandContext(context.Background(), missing, "version")
	if err := RunGitClass(context.Background(), GitLifecycle, cmd); err == nil {
		t.Fatal("missing Git executable unexpectedly succeeded")
	}

	quickCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	output, err := RunGitOutputClass(quickCtx, GitLifecycle, exec.CommandContext(quickCtx, "/bin/echo", "released"))
	if err != nil {
		t.Fatalf("Git slot remained held after start failure: %v", err)
	}
	if string(output) != "released\n" {
		t.Fatalf("post-failure output = %q", output)
	}
}

func TestGitLifecycleRejectsControllingTerminalAttributes(t *testing.T) {
	for name, attr := range map[string]syscall.SysProcAttr{
		"controlling terminal":       {Setctty: true},
		"caller-owned process group": {Setpgid: true},
	} {
		t.Run(name, func(t *testing.T) {
			cmd := NewGitCommand(context.Background(), "version")
			cmd.SysProcAttr = &attr
			if err := RunGitClass(context.Background(), GitLifecycle, cmd); err == nil {
				t.Fatal("Git command with incompatible process attributes unexpectedly started")
			} else if !strings.Contains(err.Error(), "Git command requests") {
				t.Fatalf("incompatible attribute error = %v", err)
			}
		})
	}
}

func writeGitLifecycleFixture(t *testing.T, script string) (string, []string) {
	t.Helper()
	dir := t.TempDir()
	gitPath := filepath.Join(dir, "git")
	if err := os.WriteFile(gitPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write Git fixture: %v", err)
	}
	return dir, []string{
		"PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"GIT_TEST_STARTED=" + filepath.Join(dir, "started"),
		"GIT_TEST_CHILD_PID=" + filepath.Join(dir, "child_pid"),
		"GIT_TERMINAL_PROMPT=1",
	}
}

func receiveGitLifecycleResult(t *testing.T, result <-chan struct {
	output  []byte
	runErr  error
	execErr error
}, timeout time.Duration) struct {
	output  []byte
	runErr  error
	execErr error
} {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(timeout):
		t.Fatal("Git lifecycle did not return within the cleanup bound")
		return struct {
			output  []byte
			runErr  error
			execErr error
		}{}
	}
}

func waitForTestFile(t *testing.T, path string, timeout time.Duration) {
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

func readTestPID(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read child PID: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parse child PID: %v", err)
	}
	return pid
}

func killTestPID(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return
	}
	killPID(t, readTestPID(t, path))
}

func killPID(t *testing.T, pid int) {
	t.Helper()
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("kill test child %d: %v", pid, err)
	}
}

func waitForPIDExit(t *testing.T, pid int, timeout time.Duration) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if testPIDExited(pid) {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			killPID(t, pid)
			t.Fatalf("Git descendant %d remained alive", pid)
		}
	}
}

func testPIDExited(pid int) bool {
	err := syscall.Kill(pid, 0)
	if errors.Is(err, syscall.ESRCH) {
		return true
	}
	if err != nil {
		return false
	}

	// A descendant is reparented when its Git leader exits. Some Unix test
	// runners retain the killed child as a zombie briefly, so signal-zero alone
	// still reports it as present even though it can no longer run.
	data, readErr := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if readErr != nil {
		return false
	}
	closeParen := strings.LastIndex(string(data), ")")
	return closeParen >= 0 && len(data) > closeParen+2 && (data[closeParen+2] == 'Z' || data[closeParen+2] == 'X')
}
