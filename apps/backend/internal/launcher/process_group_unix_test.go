//go:build linux || darwin

package launcher

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

const launcherSignalHelperEnv = "KANDEV_LAUNCHER_SIGNAL_HELPER"

func TestConfigureManagedProcessCreatesProcessGroup(t *testing.T) {
	cmd := exec.Command("kandev")
	configureManagedProcess(cmd)

	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatalf("configureManagedProcess should set Setpgid")
	}
}

func TestManagedProcessKillSendsGracefulSignalBeforeForceKill(t *testing.T) {
	tempDir := t.TempDir()
	readyFile := filepath.Join(tempDir, "ready")
	termFile := filepath.Join(tempDir, "term")
	cmd := exec.Command(os.Args[0], "-test.run=TestLauncherSignalHelper")
	cmd.Env = append(os.Environ(),
		launcherSignalHelperEnv+"=1",
		"KANDEV_LAUNCHER_SIGNAL_HELPER_READY_FILE="+readyFile,
		"KANDEV_LAUNCHER_SIGNAL_HELPER_FILE="+termFile,
	)
	configureManagedProcess(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start fixture: %v", err)
	}

	proc := &managedProcess{cmd: cmd, done: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		code := 0
		if err != nil {
			code = 1
			if exitErr, ok := err.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			}
		}
		proc.mu.Lock()
		proc.exitCode = code
		proc.exited = true
		proc.mu.Unlock()
		close(proc.done)
	}()
	t.Cleanup(func() {
		if exited, _ := proc.Exited(); !exited {
			_ = killManagedProcessGroup(cmd.Process.Pid)
			<-proc.done
		}
	})

	waitForFile(t, readyFile)
	proc.kill()

	raw, err := os.ReadFile(termFile)
	if err != nil {
		t.Fatalf("expected fixture to handle SIGTERM before force kill: %v", err)
	}
	if string(raw) != "term" {
		t.Fatalf("SIGTERM marker = %q, want term", string(raw))
	}
}

func TestLauncherSignalHelper(t *testing.T) {
	if os.Getenv(launcherSignalHelperEnv) != "1" {
		return
	}
	termFile := os.Getenv("KANDEV_LAUNCHER_SIGNAL_HELPER_FILE")
	if termFile == "" {
		os.Exit(2)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	readyFile := os.Getenv("KANDEV_LAUNCHER_SIGNAL_HELPER_READY_FILE")
	if readyFile == "" {
		os.Exit(4)
	}
	if err := os.WriteFile(readyFile, []byte("ready"), 0o600); err != nil {
		os.Exit(5)
	}
	<-signals
	if err := os.WriteFile(termFile, []byte("term"), 0o600); err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	for range 100 {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
