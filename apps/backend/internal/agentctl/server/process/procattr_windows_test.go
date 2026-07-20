//go:build windows

package process

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestWindowsSetProcGroupDoesNotSuspendSharedHelpers(t *testing.T) {
	cmd := exec.Command("kandev")
	setProcGroup(cmd)

	require.NotNil(t, cmd.SysProcAttr)
	require.NotZero(t, cmd.SysProcAttr.CreationFlags&syscall.CREATE_NEW_PROCESS_GROUP)
	require.Zero(t, cmd.SysProcAttr.CreationFlags&windows.CREATE_SUSPENDED)
}

func TestWindowsSetAgentProcGroupStartsSuspended(t *testing.T) {
	cmd := exec.Command("kandev")
	setAgentProcGroup(cmd)

	require.NotNil(t, cmd.SysProcAttr)
	require.NotZero(t, cmd.SysProcAttr.CreationFlags&syscall.CREATE_NEW_PROCESS_GROUP)
	require.NotZero(t, cmd.SysProcAttr.CreationFlags&windows.CREATE_SUSPENDED)
}

func TestWindowsSetManagedProcGroupStartsSuspended(t *testing.T) {
	cmd := exec.Command("kandev")
	setManagedProcGroup(cmd)

	require.NotNil(t, cmd.SysProcAttr)
	require.NotZero(t, cmd.SysProcAttr.CreationFlags&syscall.CREATE_NEW_PROCESS_GROUP)
	require.NotZero(t, cmd.SysProcAttr.CreationFlags&windows.CREATE_SUSPENDED)
}

func TestWindowsProcessLifecycleJobKillsDescendants(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	cmd := fixtureCmd(fmt.Sprintf("delay-then-child %s 200 30", pidFile))
	setAgentProcGroup(cmd)
	require.NoError(t, cmd.Start())
	parentPID := cmd.Process.Pid
	waited := false
	t.Cleanup(func() {
		if waited {
			return
		}
		_ = killProcessGroup(parentPID)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	lifecycle, err := installProcessLifecycle(cmd)
	require.NoError(t, err)
	childPID := waitForWindowsChildPID(t, pidFile, 5*time.Second)

	releaseProcessLifecycle(lifecycle)
	_ = cmd.Wait()
	waited = true

	require.Eventually(t, func() bool {
		return !windowsProcessAlive(childPID)
	}, 5*time.Second, 50*time.Millisecond,
		"child process %d should be killed when the job handle is released", childPID)
}

func TestWindowsManagedLifecycleReapsDescendantAfterLeaderExit(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	cmd := fixtureCmd(fmt.Sprintf("exit-with-child %s 30", pidFile))
	setManagedProcGroup(cmd)
	require.NoError(t, cmd.Start())
	parentPID := cmd.Process.Pid
	waited := false
	t.Cleanup(func() {
		if waited {
			return
		}
		_ = killProcessGroup(parentPID)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	lifecycle, err := installProcessLifecycle(cmd)
	require.NoError(t, err)
	childPID := waitForWindowsChildPID(t, pidFile, 5*time.Second)
	require.NoError(t, cmd.Wait())
	waited = true
	require.True(t, windowsProcessAlive(childPID), "descendant should outlive its leader before job reap")

	require.NoError(t, reapProcessLifecycle(lifecycle))
	require.Eventually(t, func() bool {
		return !windowsProcessAlive(childPID)
	}, 5*time.Second, 50*time.Millisecond,
		"child process %d should be gone before job reap completes", childPID)
}

func TestWindowsProcessRunnerReapsDescendantAfterLeaderExit(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "runner-child.pid")
	command, env := fixtureShellExec(fmt.Sprintf("exit-with-child %s 30", pidFile))
	runner := NewProcessRunner(nil, newTestLogger(t), 1024)
	info, err := runner.Start(context.Background(), StartProcessRequest{
		SessionID: "windows-job-reap",
		Kind:      types.ProcessKind("test"),
		Command:   command,
		Env:       env,
	})
	require.NoError(t, err)
	childPID := waitForWindowsChildPID(t, pidFile, 5*time.Second)
	t.Cleanup(func() {
		_ = runProcessTaskkill("/F", "/T", "/PID", strconv.Itoa(childPID))
	})

	require.Eventually(t, func() bool {
		_, ok := runner.Get(info.ID, false)
		return !ok
	}, 5*time.Second, 20*time.Millisecond, "runner should retain the process until job reap completes")
	require.False(t, windowsProcessAlive(childPID), "runner returned ownership before its descendant exited")
}

func TestWindowsVscodeLifecycleReapsDescendantAfterLeaderExit(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "vscode-child.pid")
	cmd := fixtureCmd(fmt.Sprintf("exit-with-child %s 30", pidFile))
	setManagedProcGroup(cmd)
	require.NoError(t, cmd.Start())
	lifecycle, err := installProcessLifecycle(cmd)
	require.NoError(t, err)
	childPID := waitForWindowsChildPID(t, pidFile, 5*time.Second)
	require.NoError(t, cmd.Wait())
	t.Cleanup(func() {
		_ = runProcessTaskkill("/F", "/T", "/PID", strconv.Itoa(childPID))
	})

	vscode := &VscodeManager{logger: newTestLogger(t), lifecycle: lifecycle}
	require.NoError(t, vscode.ensureProcessGroupReaped(cmd.Process.Pid))
	require.False(t, windowsProcessAlive(childPID), "VS Code returned ownership before its descendant exited")
}

func TestWindowsProcessLifecycleInstallFailsForInvalidPID(t *testing.T) {
	_, err := installProcessLifecycle(&exec.Cmd{Process: &os.Process{Pid: -1}})
	require.Error(t, err)
}

func waitForWindowsChildPID(t *testing.T, pidFile string, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		raw, err := os.ReadFile(pidFile)
		if err == nil {
			pid, perr := strconv.Atoi(strings.TrimSpace(string(raw)))
			if perr == nil && pid > 0 {
				return pid
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for fixture to write child pid to %s", pidFile)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func windowsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	output, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH").Output()
	if err != nil {
		return false
	}
	text := strings.ToLower(string(output))
	if strings.Contains(text, "no tasks") {
		return false
	}
	return strings.Contains(text, strconv.Itoa(pid))
}
