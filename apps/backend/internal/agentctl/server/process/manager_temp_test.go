package process

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/shell"
	"github.com/kandev/kandev/internal/agentctl/types"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type stubAgentAdapter struct{}

type notifyingCloser struct {
	closed chan struct{}
	once   sync.Once
}

func (c *notifyingCloser) Write(p []byte) (int, error) { return len(p), nil }

func (c *notifyingCloser) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func (stubAgentAdapter) PrepareEnvironment() (map[string]string, error) { return nil, nil }
func (stubAgentAdapter) PrepareCommandArgs() []string                   { return nil }
func (stubAgentAdapter) Connect(io.Writer, io.Reader) error             { return nil }
func (stubAgentAdapter) Initialize(context.Context) error               { return nil }
func (stubAgentAdapter) GetAgentInfo() *adapter.AgentInfo               { return nil }
func (stubAgentAdapter) NewSession(context.Context, []types.McpServer) (string, error) {
	return "", nil
}
func (stubAgentAdapter) LoadSession(context.Context, string, []types.McpServer) error {
	return nil
}
func (stubAgentAdapter) Prompt(context.Context, string, []v1.MessageAttachment, uint64) error {
	return nil
}
func (stubAgentAdapter) Cancel(context.Context) error                   { return nil }
func (stubAgentAdapter) Updates() <-chan adapter.AgentEvent             { return nil }
func (stubAgentAdapter) GetSessionID() string                           { return "" }
func (stubAgentAdapter) GetOperationID() string                         { return "" }
func (stubAgentAdapter) SetPermissionHandler(adapter.PermissionHandler) {}
func (stubAgentAdapter) Close() error                                   { return nil }
func (stubAgentAdapter) RequiresProcessKill() bool                      { return false }

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, item := range env {
		if len(item) > len(prefix) && item[:len(prefix)] == prefix {
			return item[len(prefix):]
		}
	}
	return ""
}

func TestManager_BuildFinalCommandInjectsIsolatedTempDir(t *testing.T) {
	setAgentTempTestEnv(t)
	mgr := NewManager(&config.InstanceConfig{
		WorkDir:   t.TempDir(),
		SessionID: "session-123",
		AgentArgs: []string{"echo"},
		AgentEnv:  []string{"PATH=/usr/bin"},
	}, newTestLogger(t))
	mgr.adapter = stubAgentAdapter{}
	t.Cleanup(func() { _ = mgr.StopForTeardown(context.Background()) })

	if err := mgr.buildFinalCommand(); err != nil {
		t.Fatalf("buildFinalCommand() error = %v", err)
	}

	want := filepath.Join(os.TempDir(), "kandev-agent", agentTempDirName("session-123", "", 0))
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		if got := envValue(mgr.cmd.Env, key); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("expected temp dir %q to exist, stat=%v err=%v", want, info, err)
	}
}

func TestManager_OwnedProcessEnvOverridesRequestedTemp(t *testing.T) {
	setAgentTempTestEnv(t)
	mgr := NewManager(&config.InstanceConfig{
		WorkDir:   t.TempDir(),
		SessionID: "session-child-env",
	}, newTestLogger(t))
	if err := mgr.ensureAgentTempEnv(); err != nil {
		t.Fatalf("ensureAgentTempEnv() error = %v", err)
	}

	env := mgr.ownedProcessEnv(map[string]string{
		"TMPDIR": "unmanaged",
		"CUSTOM": "preserved",
	})
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		if got := env[key]; got != mgr.agentTempDir {
			t.Fatalf("%s = %q, want %q", key, got, mgr.agentTempDir)
		}
	}
	if got := env["CUSTOM"]; got != "preserved" {
		t.Fatalf("CUSTOM = %q, want preserved", got)
	}
	if err := mgr.StopForTeardown(context.Background()); err != nil {
		t.Fatalf("StopForTeardown() error = %v", err)
	}
}

func TestManager_StopRemovesAlreadyStoppedTempDirAndPreservesSibling(t *testing.T) {
	setAgentTempTestEnv(t)
	mgr := NewManager(&config.InstanceConfig{
		WorkDir:   t.TempDir(),
		SessionID: "session-owned",
	}, newTestLogger(t))

	if err := mgr.ensureAgentTempEnv(); err != nil {
		t.Fatalf("ensureAgentTempEnv() error = %v", err)
	}
	owned := envValue(mgr.cfg.AgentEnv, "TMPDIR")
	sibling := filepath.Join(filepath.Dir(owned), "session-sibling")
	if err := os.MkdirAll(sibling, 0o700); err != nil {
		t.Fatalf("create sibling temp dir: %v", err)
	}

	if err := mgr.StopForTeardown(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if _, err := os.Stat(owned); !os.IsNotExist(err) {
		t.Fatalf("owned temp dir still exists after Stop(), err = %v", err)
	}
	if info, err := os.Stat(sibling); err != nil || !info.IsDir() {
		t.Fatalf("sibling temp dir was changed, stat = %v, err = %v", info, err)
	}
	if info, err := os.Stat(filepath.Dir(owned)); err != nil || !info.IsDir() {
		t.Fatalf("shared temp root was changed, stat = %v, err = %v", info, err)
	}
}

func TestManager_StopRemovesTempOnlyAfterProcessTeardown(t *testing.T) {
	setAgentTempTestEnv(t)
	mgr := NewManager(&config.InstanceConfig{
		WorkDir:   t.TempDir(),
		SessionID: "session-running",
	}, newTestLogger(t))
	if err := mgr.ensureAgentTempEnv(); err != nil {
		t.Fatalf("ensureAgentTempEnv() error = %v", err)
	}
	owned := envValue(mgr.cfg.AgentEnv, "TMPDIR")

	stdin := &notifyingCloser{closed: make(chan struct{})}
	mgr.stdin = stdin
	mgr.status.Store(StatusRunning)
	reap := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(reap) }) }
	mgr.wg.Add(1)
	go func() {
		defer mgr.wg.Done()
		<-reap
	}()
	t.Cleanup(release)

	stopDone := make(chan error, 1)
	go func() { stopDone <- mgr.StopForTeardown(context.Background()) }()
	<-stdin.closed
	if info, err := os.Stat(owned); err != nil || !info.IsDir() {
		t.Fatalf("temp dir removed before process reap, stat = %v, err = %v", info, err)
	}

	release()
	if err := <-stopDone; err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if _, err := os.Stat(owned); !os.IsNotExist(err) {
		t.Fatalf("owned temp dir still exists after process reap, err = %v", err)
	}
}

func TestManager_StopUsesStoredTempOwnershipInsteadOfMutableEnvironment(t *testing.T) {
	setAgentTempTestEnv(t)
	mgr := NewManager(&config.InstanceConfig{
		WorkDir:   t.TempDir(),
		SessionID: "session-stored",
	}, newTestLogger(t))
	if err := mgr.ensureAgentTempEnv(); err != nil {
		t.Fatalf("ensureAgentTempEnv() error = %v", err)
	}
	owned := envValue(mgr.cfg.AgentEnv, "TMPDIR")
	outside := filepath.Join(t.TempDir(), "must-remain")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatalf("create outside dir: %v", err)
	}
	mgr.cfg.AgentEnv = upsertEnvValue(mgr.cfg.AgentEnv, "TMPDIR", outside)

	if err := mgr.StopForTeardown(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if _, err := os.Stat(owned); !os.IsNotExist(err) {
		t.Fatalf("stored owned temp dir still exists, err = %v", err)
	}
	if info, err := os.Stat(outside); err != nil || !info.IsDir() {
		t.Fatalf("environment-selected outside dir was changed, stat = %v, err = %v", info, err)
	}
}

func TestManager_StopRejectsInvalidTempCleanupTargets(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, agentTempDirRoot)
	outside := filepath.Join(base, "outside")
	for _, path := range []string{root, outside} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatalf("create fixture dir %q: %v", path, err)
		}
	}

	tests := []struct {
		name   string
		root   string
		target string
	}{
		{name: "empty target", root: root},
		{name: "shared root", root: root, target: root},
		{name: "outside root", root: root, target: outside},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewManager(&config.InstanceConfig{WorkDir: t.TempDir()}, newTestLogger(t))
			mgr.agentTempRoot = tt.root
			mgr.agentTempDir = tt.target

			err := mgr.StopForTeardown(context.Background())
			if err == nil || !strings.Contains(err.Error(), "refusing to clean") {
				t.Fatalf("Stop() error = %v, want fail-closed cleanup error", err)
			}
		})
	}

	for _, path := range []string{root, outside} {
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			t.Fatalf("invalid cleanup changed %q, stat = %v, err = %v", path, info, err)
		}
	}
}

func TestManager_StopRejectsTempCleanupWithoutOwnershipHandle(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, agentTempDirRoot)
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatalf("create root dir: %v", err)
	}
	mgr := NewManager(&config.InstanceConfig{WorkDir: t.TempDir()}, newTestLogger(t))
	mgr.agentTempRoot = root
	mgr.agentTempDir = filepath.Join(root, "session\x00invalid")

	err := mgr.StopForTeardown(context.Background())
	if err == nil || !strings.Contains(err.Error(), "refusing to clean unowned") {
		t.Fatalf("Stop() error = %v, want fail-closed ownership error", err)
	}
	if mgr.agentTempDir == "" {
		t.Fatal("cleanup ownership cleared after failed removal")
	}
}

func TestManager_EnsureTempRejectsSymlinkedSharedRoot(t *testing.T) {
	tempBase := setAgentTempTestEnv(t)
	outside := t.TempDir()
	root := filepath.Join(tempBase, agentTempDirRoot)
	if err := os.Symlink(outside, root); err != nil {
		t.Skipf("create symlink fixture: %v", err)
	}
	mgr := NewManager(&config.InstanceConfig{
		WorkDir:   t.TempDir(),
		SessionID: "session-symlink",
	}, newTestLogger(t))

	err := mgr.ensureAgentTempEnv()
	if err == nil || !strings.Contains(err.Error(), "agent temp root") {
		t.Fatalf("ensureAgentTempEnv() error = %v, want unsafe-root error", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "session-symlink")); !os.IsNotExist(err) {
		t.Fatalf("ensureAgentTempEnv() traversed symlinked root, err = %v", err)
	}
}

func TestManager_StopUsesOpenedRootAfterPathIsReplaced(t *testing.T) {
	tempBase := setAgentTempTestEnv(t)
	mgr := NewManager(&config.InstanceConfig{
		WorkDir:   t.TempDir(),
		SessionID: "session-replaced-root",
	}, newTestLogger(t))
	if err := mgr.ensureAgentTempEnv(); err != nil {
		t.Fatalf("ensureAgentTempEnv() error = %v", err)
	}

	root := filepath.Join(tempBase, agentTempDirRoot)
	parkedRoot := filepath.Join(tempBase, "original-kandev-agent")
	if err := os.Rename(root, parkedRoot); err != nil {
		if runtime.GOOS == "windows" {
			if stopErr := mgr.StopForTeardown(context.Background()); stopErr != nil {
				t.Fatalf("StopForTeardown() after protected-root rename = %v", stopErr)
			}
			return
		}
		t.Fatalf("rename original temp root: %v", err)
	}
	outside := t.TempDir()
	outsideSession := filepath.Join(outside, "session-replaced-root")
	if err := os.MkdirAll(outsideSession, 0o700); err != nil {
		t.Fatalf("create outside session dir: %v", err)
	}
	if err := os.Symlink(outside, root); err != nil {
		t.Skipf("create replacement symlink fixture: %v", err)
	}

	ownedChild := mgr.agentTempChild
	err := mgr.StopForTeardown(context.Background())
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if info, err := os.Stat(outsideSession); err != nil || !info.IsDir() {
		t.Fatalf("cleanup traversed replaced root, stat = %v, err = %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(parkedRoot, ownedChild)); !os.IsNotExist(err) {
		t.Fatalf("owned child under original root remains after cleanup, err = %v", err)
	}
}

func TestManager_TempNamesDoNotCollideAfterSanitization(t *testing.T) {
	setAgentTempTestEnv(t)
	first := NewManager(&config.InstanceConfig{
		InstanceID: "instance-first",
		WorkDir:    t.TempDir(),
		SessionID:  "a/b",
		Port:       41001,
	}, newTestLogger(t))
	second := NewManager(&config.InstanceConfig{
		InstanceID: "instance-second",
		WorkDir:    t.TempDir(),
		SessionID:  "a_b",
		Port:       41002,
	}, newTestLogger(t))
	for _, mgr := range []*Manager{first, second} {
		if err := mgr.ensureAgentTempEnv(); err != nil {
			t.Fatalf("ensureAgentTempEnv() error = %v", err)
		}
	}
	if first.agentTempDir == second.agentTempDir {
		t.Fatalf("distinct instance identities share temp dir %q", first.agentTempDir)
	}
	firstDir := first.agentTempDir
	secondSentinel := filepath.Join(second.agentTempDir, "must-remain")
	if err := os.WriteFile(secondSentinel, []byte("owned by second"), 0o600); err != nil {
		t.Fatalf("create second sentinel: %v", err)
	}

	if err := first.StopForTeardown(context.Background()); err != nil {
		t.Fatalf("first Stop() error = %v", err)
	}
	if _, err := os.Stat(firstDir); !os.IsNotExist(err) {
		t.Fatalf("first temp dir still exists after Stop(), err = %v", err)
	}
	if data, err := os.ReadFile(secondSentinel); err != nil || string(data) != "owned by second" {
		t.Fatalf("first cleanup changed second temp dir, data = %q, err = %v", data, err)
	}
	if err := second.StopForTeardown(context.Background()); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}
}

func TestManager_StoppedAgentKeepsTempUntilWorkspaceProcessReaped(t *testing.T) {
	setAgentTempTestEnv(t)
	mgr := NewManager(&config.InstanceConfig{
		InstanceID: "instance-natural-exit",
		WorkDir:    t.TempDir(),
		SessionID:  "session-natural-exit",
	}, newTestLogger(t))
	if err := mgr.ensureAgentTempEnv(); err != nil {
		t.Fatalf("ensureAgentTempEnv() error = %v", err)
	}
	owned := mgr.agentTempDir
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
		t.Fatalf("Stop() returned before workspace process stop/reap: %v", err)
	}
	if info, err := os.Stat(owned); err != nil || !info.IsDir() {
		t.Fatalf("temp dir removed before workspace process reap, stat = %v, err = %v", info, err)
	}

	close(proc.done)
	if err := <-stopDone; err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if _, err := os.Stat(owned); !os.IsNotExist(err) {
		t.Fatalf("temp dir still exists after workspace process reap, err = %v", err)
	}
}

func TestManager_BeginStopWaitsForInFlightTempOwnerAdmission(t *testing.T) {
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
		t.Fatal("BeginStop() returned before in-flight owner admission completed")
	default:
	}
	release()
	<-stopAdmissionDone

	if err := mgr.Start(context.Background()); !errors.Is(err, errManagerStopping) {
		t.Fatalf("Start() error = %v, want manager-stopping error", err)
	}
	if _, err := mgr.StartProcess(context.Background(), StartProcessRequest{}); !errors.Is(err, errManagerStopping) {
		t.Fatalf("StartProcess() error = %v, want manager-stopping error", err)
	}
	if err := mgr.StartShell(); !errors.Is(err, errManagerStopping) {
		t.Fatalf("StartShell() error = %v, want manager-stopping error", err)
	}
	if err := mgr.StartVscode(context.Background(), "dark"); !errors.Is(err, errManagerStopping) {
		t.Fatalf("StartVscode() error = %v, want manager-stopping error", err)
	}
	if _, err := mgr.ShellManager().Start("terminal", shell.DefaultConfig(t.TempDir())); err == nil {
		t.Fatal("terminal shell Start() succeeded after BeginStop()")
	}
}

func TestManager_OrdinaryStopCannotCleanTempDuringTerminalAdmissionDrain(t *testing.T) {
	setAgentTempTestEnv(t)
	mgr := NewManager(&config.InstanceConfig{
		WorkDir:   t.TempDir(),
		SessionID: "session-concurrent-stop",
	}, newTestLogger(t))
	if err := mgr.ensureAgentTempEnv(); err != nil {
		t.Fatalf("ensureAgentTempEnv() error = %v", err)
	}
	owned := mgr.agentTempDir
	release, err := mgr.admitStart()
	if err != nil {
		t.Fatalf("admitStart() error = %v", err)
	}
	mgr.CloseAdmission()

	if err := mgr.Stop(context.Background()); err != nil {
		t.Fatalf("ordinary Stop() error = %v", err)
	}
	if info, err := os.Stat(owned); err != nil || !info.IsDir() {
		t.Fatalf("ordinary Stop() removed terminal-owned temp dir, stat = %v, err = %v", info, err)
	}

	terminalDone := make(chan error, 1)
	go func() { terminalDone <- mgr.StopForTeardown(context.Background()) }()
	select {
	case err := <-terminalDone:
		t.Fatalf("terminal stop returned before admission drained: %v", err)
	default:
	}
	release()
	if err := <-terminalDone; err != nil {
		t.Fatalf("StopForTeardown() error = %v", err)
	}
	if _, err := os.Stat(owned); !os.IsNotExist(err) {
		t.Fatalf("terminal stop did not remove temp dir after drain, err = %v", err)
	}
}

func TestManager_TerminalAdmissionDrainHonorsContextAndRetries(t *testing.T) {
	setAgentTempTestEnv(t)
	mgr := NewManager(&config.InstanceConfig{
		WorkDir:   t.TempDir(),
		SessionID: "session-drain-timeout",
	}, newTestLogger(t))
	if err := mgr.ensureAgentTempEnv(); err != nil {
		t.Fatalf("ensureAgentTempEnv() error = %v", err)
	}
	owned := mgr.agentTempDir
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
	if info, statErr := os.Stat(owned); statErr != nil || !info.IsDir() {
		t.Fatalf("canceled drain removed temp ownership, stat = %v, err = %v", info, statErr)
	}

	release()
	if err := mgr.StopForTeardown(context.Background()); err != nil {
		t.Fatalf("StopForTeardown() retry error = %v", err)
	}
	if _, err := os.Stat(owned); !os.IsNotExist(err) {
		t.Fatalf("retry did not remove temp dir, err = %v", err)
	}
}

func TestManager_TempCleanupWaitsForMainProcessGroupReap(t *testing.T) {
	setAgentTempTestEnv(t)
	mgr := NewManager(&config.InstanceConfig{
		InstanceID: "instance-main-reap-failure",
		WorkDir:    t.TempDir(),
		SessionID:  "session-main-reap-failure",
	}, newTestLogger(t))
	if err := mgr.ensureAgentTempEnv(); err != nil {
		t.Fatalf("ensureAgentTempEnv() error = %v", err)
	}
	owned := mgr.agentTempDir
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
		t.Fatalf("Stop() error = %v, want process-group reap failure", err)
	}
	if info, statErr := os.Stat(owned); statErr != nil || !info.IsDir() {
		t.Fatalf("temp dir removed after failed main reap, stat = %v, err = %v", info, statErr)
	}
}

func TestManager_TempCleanupWaitsForMainGoroutineReap(t *testing.T) {
	setAgentTempTestEnv(t)
	mgr := NewManager(&config.InstanceConfig{
		InstanceID: "instance-main-wait-failure",
		WorkDir:    t.TempDir(),
		SessionID:  "session-main-wait-failure",
	}, newTestLogger(t))
	if err := mgr.ensureAgentTempEnv(); err != nil {
		t.Fatalf("ensureAgentTempEnv() error = %v", err)
	}
	owned := mgr.agentTempDir
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
		t.Fatalf("Stop() error = %v, want manager reap failure", err)
	}
	if info, statErr := os.Stat(owned); statErr != nil || !info.IsDir() {
		t.Fatalf("temp dir removed after failed manager reap, stat = %v, err = %v", info, statErr)
	}
}

func setAgentTempTestEnv(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, root)
	}
	return root
}
