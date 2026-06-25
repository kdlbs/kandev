package launcher

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestRunStartUsesSelfExecutableAndBackendCWD(t *testing.T) {
	oldExecutablePath := executablePath
	oldLaunchManaged := launchManaged
	t.Cleanup(func() {
		executablePath = oldExecutablePath
		launchManaged = oldLaunchManaged
	})

	exe := filepath.Join(t.TempDir(), "bin", "kandev")
	executablePath = func() (string, error) {
		return exe, nil
	}

	var got managedAppConfig
	launchManaged = func(cfg managedAppConfig) int {
		got = cfg
		return 42
	}
	t.Setenv("KANDEV_HOME_DIR", t.TempDir())

	code := runStart(Options{Command: CommandStart, BackendPort: 48123, Headless: true})
	if code != 42 {
		t.Fatalf("runStart() = %d, want 42", code)
	}
	if got.Backend != exe {
		t.Fatalf("Backend = %q, want %q", got.Backend, exe)
	}
	if got.BackendCWD != filepath.Dir(exe) {
		t.Fatalf("BackendCWD = %q, want %q", got.BackendCWD, filepath.Dir(exe))
	}
	if got.Mode != "start" {
		t.Fatalf("Mode = %q, want start", got.Mode)
	}
	if got.Ports.BackendPort != 48123 {
		t.Fatalf("BackendPort = %d, want 48123", got.Ports.BackendPort)
	}
	if !got.Opts.Headless {
		t.Fatal("expected Headless option to be preserved")
	}
}

func TestRunManagedAppAttachesSignalsBeforeBackendLaunch(t *testing.T) {
	oldNewSupervisor := newSupervisorFn
	oldLaunchBackend := launchBackendFn
	oldAttachSignals := attachSignalsFn
	oldWaitForHealth := waitForHealthFn
	t.Cleanup(func() {
		newSupervisorFn = oldNewSupervisor
		launchBackendFn = oldLaunchBackend
		attachSignalsFn = oldAttachSignals
		waitForHealthFn = oldWaitForHealth
	})

	var events []string
	newSupervisorFn = func() *processSupervisor {
		events = append(events, "new-supervisor")
		return newSupervisor()
	}
	launchBackendFn = func(
		_ string,
		_ []string,
		_ string,
		_ []string,
		_ bool,
		_ portConfig,
		_ string,
		_ *processSupervisor,
	) (*restartableBackend, func(), error) {
		events = append(events, "launch-backend")
		exitCh := make(chan int, 1)
		exitCh <- 0
		return &restartableBackend{exitCh: exitCh}, func() {}, nil
	}
	attachSignalsFn = func(_ *processSupervisor) {
		events = append(events, "attach-signals")
	}
	waitForHealthFn = func(_ string, _ childState, _ time.Duration, _ func()) error {
		events = append(events, "wait-health")
		return nil
	}
	t.Setenv("KANDEV_HOME_DIR", t.TempDir())

	code := runManagedApp(managedAppConfig{
		Header:     "test",
		Mode:       "start",
		Backend:    "kandev",
		BackendCWD: t.TempDir(),
		Ports: portConfig{
			BackendPort: 48123,
			BackendURL:  "http://localhost:48123",
		},
		Opts: Options{Headless: true},
	})
	if code != 0 {
		t.Fatalf("runManagedApp() = %d, want 0", code)
	}
	want := []string{"new-supervisor", "attach-signals", "launch-backend", "wait-health"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}
