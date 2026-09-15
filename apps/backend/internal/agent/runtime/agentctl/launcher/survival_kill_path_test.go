//go:build !windows

package launcher

import (
	"context"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	commonconfig "github.com/kandev/kandev/internal/common/config"
)

// TestBuildAndStartProcessSkipsParentLivenessPipeWhenSurvivalEnabled pins
// kill-path #1 from design 01's kill-paths list ("parent-liveness pipe"):
// when the agent-survival capability is engaged for this launch, agentctl
// must not depend on any backend shutdown step to keep running
// (AC-EXECUTORS-SURVIVAL-001.2), so the launcher must not arm the pipe that
// would otherwise make agentctl self-terminate the instant the write-end
// closes -- including on a backend SIGKILL, which never gives Stop() a
// chance to run.
func TestBuildAndStartProcessSkipsParentLivenessPipeWhenSurvivalEnabled(t *testing.T) {
	l := newSurvivalKillPathTestLauncher(t, true)
	if err := l.buildAndStartProcess("test-nonce"); err != nil {
		t.Fatalf("buildAndStartProcess: %v", err)
	}
	t.Cleanup(func() { _ = l.cmd.Process.Kill() })
	<-l.exited // "true" exits immediately; wait so ExtraFiles/env are stable to inspect

	if len(l.cmd.ExtraFiles) != 0 {
		t.Fatalf("ExtraFiles = %v, want none: the parent-liveness pipe must not be armed", l.cmd.ExtraFiles)
	}
	if l.parentPipe != nil {
		t.Fatal("parentPipe was set even though the capability is engaged for this launch")
	}
	for _, env := range l.cmd.Env {
		if strings.HasPrefix(env, "KANDEV_PARENT_PIPE_FD=") {
			t.Fatalf("KANDEV_PARENT_PIPE_FD was set in the child environment: %s", env)
		}
	}
}

// TestBuildAndStartProcessStripsInheritedLivenessPipeEnvWhenSurvivalEnabled
// covers a real hazard this session's own sandbox surfaced: the launcher
// process can itself be running under an ambient KANDEV_PARENT_PIPE_FD (for
// example a survivable agentctl launch nested inside another one). Without
// stripping it, that stale value would leak into the child's environment
// even though ExtraFiles was never set up to back FD 3, and
// monitorParentLiveness would misread it as a live pipe rather than absent.
func TestBuildAndStartProcessStripsInheritedLivenessPipeEnvWhenSurvivalEnabled(t *testing.T) {
	t.Setenv("KANDEV_PARENT_PIPE_FD", "3")

	l := newSurvivalKillPathTestLauncher(t, true)
	if err := l.buildAndStartProcess("test-nonce"); err != nil {
		t.Fatalf("buildAndStartProcess: %v", err)
	}
	t.Cleanup(func() { _ = l.cmd.Process.Kill() })
	<-l.exited

	for _, env := range l.cmd.Env {
		if strings.HasPrefix(env, "KANDEV_PARENT_PIPE_FD=") {
			t.Fatalf("inherited KANDEV_PARENT_PIPE_FD leaked into the child environment: %s", env)
		}
	}
}

// TestBuildAndStartProcessArmsParentLivenessPipeWhenSurvivalDisabled is the
// regression companion: today's behavior (capability off, or an unmanaged
// launch) must be unchanged -- the pipe is armed exactly as before.
func TestBuildAndStartProcessArmsParentLivenessPipeWhenSurvivalDisabled(t *testing.T) {
	l := newSurvivalKillPathTestLauncher(t, false)
	if err := l.buildAndStartProcess("test-nonce"); err != nil {
		t.Fatalf("buildAndStartProcess: %v", err)
	}
	t.Cleanup(func() { _ = l.cmd.Process.Kill() })
	<-l.exited

	if len(l.cmd.ExtraFiles) != 1 {
		t.Fatalf("ExtraFiles = %v, want exactly 1 (the pipe read-end)", l.cmd.ExtraFiles)
	}
	if l.parentPipe == nil {
		t.Fatal("parentPipe was not set with the capability disabled")
	}
	_ = l.parentPipe.Close()
}

// TestLauncherStopIsANoOpWhenSurvivalEnabled pins kill-path #5 ("registered
// cleanup"): AC-EXECUTORS-SURVIVAL-001.1 requires that a graceful backend
// shutdown "shall not issue a stop" to the control server or its instances
// when the capability is enabled. Stop() is the only thing the backend's
// registered cleanup calls (see provider.go), so making Stop() itself a
// no-op closes that path without needing a second "should I actually stop"
// branch at the one call site.
func TestLauncherStopIsANoOpWhenSurvivalEnabled(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start long-lived test process: %v", err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })

	l := &Launcher{
		cmd:           cmd,
		exited:        make(chan struct{}), // never closed: nothing reaps this process in the test
		logger:        newLauncherTestLogger(t),
		startupConfig: commonconfig.AgentctlStartupConfig{Configured: true, AgentSurvivalEnabled: true},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := l.Stop(ctx); err != nil {
		t.Fatalf("Stop() returned an error for a no-op stop: %v", err)
	}

	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("process is no longer alive after a supposedly no-op Stop(): %v", err)
	}
}

// TestLauncherStopTerminatesTheServerWhenSurvivalDisabled is the closed side
// of the same kill path. With the capability off, the control server is the
// backend's to own for the length of the backend's life, so the registered
// cleanup's Stop() must actually terminate it rather than leave an orphan
// running with no backend attached to it.
func TestLauncherStopTerminatesTheServerWhenSurvivalDisabled(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start long-lived test process: %v", err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })

	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()

	l := &Launcher{
		cmd:           cmd,
		exited:        exited,
		logger:        newLauncherTestLogger(t),
		startupConfig: commonconfig.AgentctlStartupConfig{Configured: true, AgentSurvivalEnabled: false},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := l.Stop(ctx); err != nil {
		t.Fatalf("Stop() = %v, want nil", err)
	}

	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("process is still running after Stop() with the capability disabled")
	}
}

func newSurvivalKillPathTestLauncher(t *testing.T, survivalEnabled bool) *Launcher {
	t.Helper()
	binary, err := exec.LookPath("true")
	if err != nil {
		t.Skipf("no 'true' binary on PATH: %v", err)
	}
	return &Launcher{
		binaryPath: binary,
		host:       "localhost",
		port:       0,
		exited:     make(chan struct{}),
		logger:     newLauncherTestLogger(t),
		startupConfig: commonconfig.AgentctlStartupConfig{
			Configured:                true,
			IdleReaperInterval:        time.Minute,
			NotificationQueueCapacity: 4096,
			AgentSurvivalEnabled:      survivalEnabled,
		},
	}
}
