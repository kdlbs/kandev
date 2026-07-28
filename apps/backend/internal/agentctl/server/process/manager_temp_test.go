package process

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/shell"
	"github.com/kandev/kandev/pkg/agent"
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

func TestManager_StartShellInheritsAgentEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY-backed shell sessions are unsupported on Windows")
	}
	mgr := NewManager(&config.InstanceConfig{
		WorkDir:      t.TempDir(),
		ShellEnabled: true,
		AgentEnv: []string{
			"KANDEV_GITHUB_CREDENTIAL_BROKER_URL=http://127.0.0.1:9876",
			"PATH=/tmp/kandev-shim:/usr/bin",
		},
	}, newTestLogger(t))
	if err := mgr.StartShell(); err != nil {
		t.Fatalf("StartShell() error = %v", err)
	}
	t.Cleanup(func() { _ = mgr.shell.Stop() })
	cfg := mgr.shell.Config()
	if got := cfg.Env["KANDEV_GITHUB_CREDENTIAL_BROKER_URL"]; got != "http://127.0.0.1:9876" {
		t.Fatalf("shell broker env = %q, want managed broker URL", got)
	}
	if got := cfg.Env["PATH"]; got != "/tmp/kandev-shim:/usr/bin" {
		t.Fatalf("shell PATH = %q, want shim-first PATH", got)
	}
}

func TestManager_StartProcessInheritsAgentEnvironment(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{
		AgentEnv: []string{
			"KANDEV_GITHUB_CREDENTIAL_BROKER_URL=http://127.0.0.1:9876",
			"PATH=/tmp/kandev-shim:/usr/bin",
		},
	}, newTestLogger(t))
	req, err := mgr.buildProcessRequest(StartProcessRequest{
		SessionID: "session-1",
		Command:   "echo ok",
		Env: map[string]string{
			"COMMAND_ONLY": "yes",
			"PATH":         "/request/path",
		},
	})
	if err != nil {
		t.Fatalf("buildProcessRequest() error = %v", err)
	}
	if got := req.Env["KANDEV_GITHUB_CREDENTIAL_BROKER_URL"]; got != "http://127.0.0.1:9876" {
		t.Fatalf("process broker env = %q, want managed broker URL", got)
	}
	if got := req.Env["PATH"]; got != "/request/path" {
		t.Fatalf("process PATH = %q, want explicit request value", got)
	}
	if got := req.Env["COMMAND_ONLY"]; got != "yes" {
		t.Fatalf("process explicit env = %q, want yes", got)
	}
}

func TestManager_ProcessEnvironmentMergesIndexedGitConfig(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{
		AgentEnv: []string{
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=credential.helper",
			"GIT_CONFIG_VALUE_0=!agentctl git-credential",
		},
	}, newTestLogger(t))
	req, err := mgr.buildProcessRequest(StartProcessRequest{
		SessionID: "session-1",
		Command:   "echo ok",
		Env: map[string]string{
			"GIT_CONFIG_COUNT":   "1",
			"GIT_CONFIG_KEY_0":   "core.hooksPath",
			"GIT_CONFIG_VALUE_0": "/tmp/hooks",
		},
	})
	if err != nil {
		t.Fatalf("buildProcessRequest() error = %v", err)
	}
	if got := req.Env["GIT_CONFIG_COUNT"]; got != "2" {
		t.Fatalf("GIT_CONFIG_COUNT = %q, want merged count 2", got)
	}
	if got := req.Env["GIT_CONFIG_KEY_1"]; got != "core.hooksPath" {
		t.Fatalf("GIT_CONFIG_KEY_1 = %q, want request entry appended", got)
	}
}

func TestManager_StartAutoShellDoesNotDeadlockOnEnvironmentSnapshot(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{
		AgentArgs:    []string{"echo"},
		ShellEnabled: true,
		Protocol:     agent.ProtocolACP,
	}, newTestLogger(t))
	mgr.adapter = newOneShotStubAdapter()
	t.Cleanup(func() { _ = mgr.Stop(context.Background()) })

	done := make(chan error, 1)
	go func() { done <- mgr.Start(context.Background()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start() deadlocked while creating the auto shell")
	}
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
