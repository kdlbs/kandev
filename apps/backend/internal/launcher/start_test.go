package launcher

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestResolveLogThresholdsByMode(t *testing.T) {
	t.Setenv("KANDEV_LOG_LEVEL", "")
	tests := []struct {
		name        string
		options     Options
		wantFile    string
		wantConsole string
	}{
		{name: "normal", wantFile: "info", wantConsole: "warn"},
		{name: "debug", options: Options{Debug: true}, wantFile: "debug", wantConsole: "warn"},
		{name: "verbose", options: Options{Verbose: true}, wantFile: "info", wantConsole: "info"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolveLogLevel(test.options); got != test.wantFile {
				t.Fatalf("file level = %q, want %q", got, test.wantFile)
			}
			if got := resolveConsoleLogLevel(test.options); got != test.wantConsole {
				t.Fatalf("console level = %q, want %q", got, test.wantConsole)
			}
		})
	}
	t.Setenv("KANDEV_LOG_LEVEL", "error")
	if got := resolveLogLevel(Options{Debug: true}); got != "error" {
		t.Fatalf("explicit file override = %q", got)
	}
	if os.Getenv("KANDEV_LOG_LEVEL") != "error" {
		t.Fatal("test environment changed unexpectedly")
	}
}

func TestBackendEnvCarriesIndependentThresholds(t *testing.T) {
	env := backendEnv(portConfig{BackendPort: 1234, AgentctlPort: 5678}, "debug", "warn", true)
	joined := strings.Join(env, "\n")
	for _, expected := range []string{
		"KANDEV_LOG_LEVEL=debug",
		"KANDEV_CONSOLE_LOG_LEVEL=warn",
		"KANDEV_DEBUG_AGENT_MESSAGES=true",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("backend environment missing %q", expected)
		}
	}
}

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
	launchManaged = func(_ context.Context, cfg managedAppConfig) int {
		got = cfg
		return 42
	}
	t.Setenv("KANDEV_HOME_DIR", t.TempDir())

	code := runStart(context.Background(), Options{Command: CommandStart, BackendPort: 48123, Headless: true})
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
	var launchedQuiet bool
	newSupervisorFn = func() *processSupervisor {
		events = append(events, "new-supervisor")
		return newSupervisor()
	}
	launchBackendFn = func(
		_ string,
		_ []string,
		_ string,
		_ []string,
		quiet bool,
		_ portConfig,
		_ string,
		_ *processSupervisor,
	) (*restartableBackend, func(), error) {
		launchedQuiet = quiet
		events = append(events, "launch-backend")
		exitCh := make(chan int, 1)
		exitCh <- 0
		return &restartableBackend{exitCh: exitCh}, func() {}, nil
	}
	attachSignalsFn = func(_ *processSupervisor) {
		events = append(events, "attach-signals")
	}
	waitForHealthFn = func(_ context.Context, _ string, _ childState, _ time.Duration, _ func()) error {
		events = append(events, "wait-health")
		return nil
	}
	t.Setenv("KANDEV_HOME_DIR", t.TempDir())

	code := runManagedApp(context.Background(), managedAppConfig{
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
	if launchedQuiet {
		t.Fatal("backend stdout was not forwarded")
	}
}
