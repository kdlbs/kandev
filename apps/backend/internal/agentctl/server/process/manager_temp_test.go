package process

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/shell"
)

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return strings.TrimPrefix(item, prefix)
		}
	}
	return ""
}

func hasEnvValue(env []string, key string) bool {
	prefix := key + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return true
		}
	}
	return false
}

func TestManager_BuildFinalCommandPreservesConfiguredTempEnvironment(t *testing.T) {
	serviceTemp := setServiceTempTestEnv(t)
	mgr := NewManager(&config.InstanceConfig{
		WorkDir:   t.TempDir(),
		AgentArgs: []string{"echo"},
		AgentEnv: []string{
			"PATH=/usr/bin",
			"TMPDIR=/configured/tmpdir",
			"TMP=/configured/tmp",
			"TEMP=/configured/temp",
		},
	}, newTestLogger(t))
	mgr.adapter = newStubAdapter()

	if err := mgr.buildFinalCommand(); err != nil {
		t.Fatalf("buildFinalCommand() error = %v", err)
	}

	want := map[string]string{
		"TMPDIR": "/configured/tmpdir",
		"TMP":    "/configured/tmp",
		"TEMP":   "/configured/temp",
	}
	for key, value := range want {
		if got := envValue(mgr.cmd.Env, key); got != value {
			t.Fatalf("%s = %q, want configured service value %q", key, got, value)
		}
	}
	assertNoAgentTempRoot(t, serviceTemp)
}

func TestManager_BuildFinalCommandLeavesUnsetTempEnvironmentUnset(t *testing.T) {
	serviceTemp := setServiceTempTestEnv(t)
	mgr := NewManager(&config.InstanceConfig{
		WorkDir:   t.TempDir(),
		AgentArgs: []string{"echo"},
		AgentEnv:  []string{"PATH=/usr/bin"},
	}, newTestLogger(t))
	mgr.adapter = newStubAdapter()

	if err := mgr.buildFinalCommand(); err != nil {
		t.Fatalf("buildFinalCommand() error = %v", err)
	}

	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		if hasEnvValue(mgr.cmd.Env, key) {
			t.Fatalf("%s unexpectedly added to child environment: %q", key, mgr.cmd.Env)
		}
	}
	assertNoAgentTempRoot(t, serviceTemp)
}

func TestManager_BeginStopWaitsForInFlightAdmission(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{WorkDir: t.TempDir()}, newTestLogger(t))
	release, err := mgr.admitStart()
	if err != nil {
		t.Fatalf("admitStart() error = %v", err)
	}
	stopAdmissionDone := make(chan struct{})
	go func() {
		mgr.BeginStop()
		close(stopAdmissionDone)
	}()
	select {
	case <-stopAdmissionDone:
		t.Fatal("BeginStop() returned before in-flight admission completed")
	default:
	}
	release()
	<-stopAdmissionDone

	if err := mgr.Start(context.Background()); !errors.Is(err, ErrManagerStopping) {
		t.Fatalf("Start() error = %v, want manager-stopping error", err)
	}
	if _, err := mgr.StartProcess(context.Background(), StartProcessRequest{}); !errors.Is(err, ErrManagerStopping) {
		t.Fatalf("StartProcess() error = %v, want manager-stopping error", err)
	}
	if err := mgr.StartShell(); !errors.Is(err, ErrManagerStopping) {
		t.Fatalf("StartShell() error = %v, want manager-stopping error", err)
	}
	if err := mgr.StartVscode(context.Background(), "dark"); !errors.Is(err, ErrManagerStopping) {
		t.Fatalf("StartVscode() error = %v, want manager-stopping error", err)
	}
	if _, err := mgr.ShellManager().Start("terminal", shell.DefaultConfig(t.TempDir())); err == nil {
		t.Fatal("terminal shell Start() succeeded after BeginStop()")
	}
}

func TestManager_TeardownAdmissionDrainHonorsContextAndRetries(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{WorkDir: t.TempDir()}, newTestLogger(t))
	release, err := mgr.admitStart()
	if err != nil {
		t.Fatalf("admitStart() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = mgr.StopForTeardown(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("StopForTeardown() error = %v, want %v", err, context.Canceled)
	}

	release()
	if err := mgr.StopForTeardown(context.Background()); err != nil {
		t.Fatalf("StopForTeardown() retry error = %v", err)
	}
}

func TestManager_TeardownWaitsForWorkspaceProcessReap(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{WorkDir: t.TempDir()}, newTestLogger(t))
	proc := &commandProcess{
		info:       ProcessInfo{ID: "workspace-blocked-reap"},
		stopSignal: make(chan struct{}),
		done:       make(chan struct{}),
	}
	mgr.processRunner.processes[proc.info.ID] = proc

	stopDone := make(chan error, 1)
	go func() { stopDone <- mgr.StopForTeardown(context.Background()) }()
	select {
	case <-proc.stopSignal:
	case err := <-stopDone:
		t.Fatalf("StopForTeardown() returned before workspace process stop/reap: %v", err)
	}
	select {
	case err := <-stopDone:
		t.Fatalf("StopForTeardown() returned before workspace process reap: %v", err)
	default:
	}

	close(proc.done)
	if err := <-stopDone; err != nil {
		t.Fatalf("StopForTeardown() error = %v", err)
	}
}

func TestManager_TeardownReportsMainProcessGroupReapFailure(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{WorkDir: t.TempDir()}, newTestLogger(t))
	mgr.cmd = &exec.Cmd{Process: &os.Process{Pid: 424242}}
	mgr.status.Store(StatusRunning)
	mgr.groupAliveFn = func(int) bool { return true }
	mgr.terminateGroupFn = func(int) error { return nil }
	mgr.killGroupFn = func(int) error { return nil }
	mgr.waitGroupExitFn = func(context.Context, int) bool { return false }
	t.Cleanup(func() {
		mgr.groupAliveFn = func(int) bool { return false }
		mgr.waitGroupExitFn = nil
		_ = mgr.StopForTeardown(context.Background())
	})

	err := mgr.StopForTeardown(context.Background())
	if err == nil || !strings.Contains(err.Error(), "remains alive") {
		t.Fatalf("StopForTeardown() error = %v, want process-group reap failure", err)
	}
}

func TestManager_TeardownReportsMainGoroutineReapFailure(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{WorkDir: t.TempDir()}, newTestLogger(t))
	mgr.cmd = &exec.Cmd{Process: &os.Process{Pid: 424243}}
	mgr.status.Store(StatusRunning)
	mgr.wg.Add(1)
	t.Cleanup(func() {
		mgr.wg.Done()
		mgr.managerWaitFn = nil
		_ = mgr.StopForTeardown(context.Background())
	})
	mgr.groupAliveFn = func(int) bool { return false }
	mgr.terminateGroupFn = func(int) error { return nil }
	mgr.killGroupFn = func(int) error { return nil }
	mgr.managerWaitFn = func(context.Context, <-chan struct{}, time.Duration) bool { return false }

	err := mgr.StopForTeardown(context.Background())
	if err == nil || !strings.Contains(err.Error(), "goroutines were not reaped") {
		t.Fatalf("StopForTeardown() error = %v, want manager reap failure", err)
	}
}

func setServiceTempTestEnv(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, root)
	}
	return root
}

func assertNoAgentTempRoot(t *testing.T, serviceTemp string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(serviceTemp, "kandev-agent")); !os.IsNotExist(err) {
		t.Fatalf("unexpected kandev-agent root, err = %v", err)
	}
}
