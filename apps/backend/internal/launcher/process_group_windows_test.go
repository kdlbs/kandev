//go:build windows

package launcher

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

const launcherWindowsHelperEnv = "KANDEV_LAUNCHER_WINDOWS_HELPER"

func TestWindowsManagedProcessForceKillTreatsAlreadyExitedPIDAsGraceful(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestWindowsLauncherHelper")
	cmd.Env = append(os.Environ(), launcherWindowsHelperEnv+"=1")
	configureManagedProcess(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait helper: %v", err)
	}

	proc := &managedProcess{
		label: "already-exited",
		cmd:   cmd,
		done:  make(chan struct{}),
	}
	result := proc.forceKill("test")
	if result.err != nil {
		t.Fatalf("forceKill err = %v, want nil", result.err)
	}
	if !result.graceful || result.forceKilled {
		t.Fatalf("forceKill result graceful=%v forceKilled=%v, want true/false", result.graceful, result.forceKilled)
	}
	if result.pid != pid {
		t.Fatalf("forceKill pid = %d, want %d", result.pid, pid)
	}
}

func TestWindowsLauncherHelper(t *testing.T) {
	if os.Getenv(launcherWindowsHelperEnv) != "1" {
		return
	}
	os.Exit(0)
}

const launcherCooperativeHelperEnv = "KANDEV_LAUNCHER_COOPERATIVE_HELPER"

// A console-less child (as launched by the desktop shell) cannot receive
// taskkill's window-close request, so the graceful stop must reach it through
// the cooperative shutdown request and let its signal-driven teardown exit 0.
func TestWindowsManagedProcessGracefulStopReachesConsolelessChild(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestWindowsLauncherCooperativeHelper$")
	cmd.Env = append(os.Environ(), launcherCooperativeHelperEnv+"=1")
	configureManagedProcess(cmd)
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	exited := make(chan struct{})
	var waitErr error
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
		}
	})
	ready := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if strings.TrimSpace(scanner.Text()) == "helper-ready" {
				close(ready)
				break
			}
		}
		_, _ = io.Copy(io.Discard, stdout)
		waitErr = cmd.Wait()
		close(exited)
	}()
	select {
	case <-ready:
	case <-time.After(30 * time.Second):
		t.Fatal("helper did not arm its shutdown handler")
	}

	pid := cmd.Process.Pid
	if err := terminateManagedProcess(pid, pid, true); err != nil {
		t.Fatalf("graceful stop request for console-less child failed: %v", err)
	}
	select {
	case <-exited:
		if waitErr != nil {
			t.Fatalf("helper exit = %v, want clean exit from its own teardown", waitErr)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("helper did not exit after the graceful stop request")
	}
}

func TestWindowsLauncherCooperativeHelper(t *testing.T) {
	if os.Getenv(launcherCooperativeHelperEnv) != "1" {
		return
	}
	newSupervisor().attachSignals()
	fmt.Println("helper-ready")
	time.Sleep(60 * time.Second)
	os.Exit(3)
}
