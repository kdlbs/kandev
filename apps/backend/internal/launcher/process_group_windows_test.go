//go:build windows

package launcher

import (
	"os"
	"os/exec"
	"testing"
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
