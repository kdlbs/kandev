package launcher

import (
	"context"
	"errors"
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
	// The surrounding task environment may set dev-mode selectors. start/run
	// inherit the ambient env untouched (matching the TypeScript process.env
	// spread) but must never introduce their own selectors or a web URL.
	t.Setenv("KANDEV_DEBUG_DEV_MODE", "true")
	t.Setenv("KANDEV_WEB_INTERNAL_URL", "http://localhost:9999")
	env := backendEnv(portConfig{BackendPort: 1234, AgentctlPort: 5678}, "debug", "warn", true, "health-token", nil)
	joined := strings.Join(env, "\n")
	for _, expected := range []string{
		"KANDEV_LOG_LEVEL=debug",
		"KANDEV_CONSOLE_LOG_LEVEL=warn",
		"KANDEV_DEBUG_AGENT_MESSAGES=true",
		"KANDEV_DESKTOP_HEALTH_TOKEN=health-token",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("backend environment missing %q", expected)
		}
	}
	// Inherited selectors pass through unchanged, but no web port is present
	// so backendEnv must not synthesize a URL for it.
	if !strings.Contains(joined, "KANDEV_WEB_INTERNAL_URL=http://localhost:9999") {
		t.Fatalf("backend environment dropped an inherited selector:\n%s", joined)
	}
	if strings.Contains(joined, "KANDEV_WEB_INTERNAL_URL=http://localhost:5678") {
		t.Fatal("backend environment introduced a web internal URL without a web port")
	}
}

func TestBackendEnvEmitsWebInternalURLForDevPorts(t *testing.T) {
	env := backendEnv(portConfig{BackendPort: 1234, WebPort: 5678, AgentctlPort: 9876}, "info", "warn", false, "token", []string{"KANDEV_DEBUG_DEV_MODE=true", "KANDEV_HOME_DIR=/dev/home"})
	joined := strings.Join(env, "\n")
	for _, expected := range []string{
		"KANDEV_WEB_INTERNAL_URL=http://localhost:5678",
		"KANDEV_DEBUG_DEV_MODE=true",
		"KANDEV_HOME_DIR=/dev/home",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("backend environment missing %q", expected)
		}
	}
	if strings.Contains(joined, "KANDEV_WEB_INTERNAL_URL=http://localhost:0") {
		t.Fatalf("backend environment emitted an empty web internal URL:\n%s", joined)
	}
}

func TestBackendEnvDebugDefaultsToDebugTitle(t *testing.T) {
	unsetEnvForTest(t, "KANDEV_WEB_TITLE_PREFIX")

	env := backendEnv(portConfig{BackendPort: 1234, AgentctlPort: 5678}, "debug", "warn", true, "health-token", nil)
	if got := envValue(env, "KANDEV_WEB_TITLE_PREFIX"); got != "Debug" {
		t.Fatalf("KANDEV_WEB_TITLE_PREFIX = %q for debug launch; want %q", got, "Debug")
	}
}

func TestBackendEnvDebugPreservesExplicitTitle(t *testing.T) {
	t.Setenv("KANDEV_WEB_TITLE_PREFIX", "Custom")

	env := backendEnv(portConfig{BackendPort: 1234, AgentctlPort: 5678}, "debug", "warn", true, "health-token", nil)
	if got := envValue(env, "KANDEV_WEB_TITLE_PREFIX"); got != "Custom" {
		t.Fatalf("KANDEV_WEB_TITLE_PREFIX = %q for explicit debug launch; want %q", got, "Custom")
	}
}

func TestNewHealthTokenIsFreshAndOpaque(t *testing.T) {
	first, err := newHealthToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := newHealthToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != healthTokenBytes*2 || len(second) != healthTokenBytes*2 {
		t.Fatalf("health token lengths = %d and %d, want %d", len(first), len(second), healthTokenBytes*2)
	}
	if first == second {
		t.Fatal("two health token generations returned the same value")
	}
}

func TestRunStartUsesSelfExecutableAndBackendCWD(t *testing.T) {
	oldExecutablePath := executablePath
	oldLaunchManaged := launchManaged
	t.Cleanup(func() {
		executablePath = oldExecutablePath
		launchManaged = oldLaunchManaged
	})

	exe := filepath.Join(canonicalTempDir(t), "bin", "kandev")
	executablePath = func() (string, error) {
		return exe, nil
	}

	var got managedAppConfig
	launchManaged = func(_ context.Context, cfg managedAppConfig) int {
		got = cfg
		return 42
	}
	// ensureDataDir rejects a data dir with any symlinked ancestor, so the home
	// dir must be canonical (macOS temp dirs sit under the /var -> /private/var link).
	t.Setenv("KANDEV_HOME_DIR", canonicalTempDir(t))

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

func TestRunStartUsesConfiguredBindForBackendURL(t *testing.T) {
	clearLauncherConfigurationEnvironment(t)
	dir := t.TempDir()
	t.Chdir(dir)
	homeDir := canonicalTempDir(t)
	writeLauncherConfig(t, filepath.Join(dir, "config.yaml"), "homeDir: "+homeDir+"\nserver:\n  host: 192.0.2.10\n  port: 48123\n")

	oldExecutablePath := executablePath
	oldLaunchManaged := launchManaged
	t.Cleanup(func() {
		executablePath = oldExecutablePath
		launchManaged = oldLaunchManaged
	})
	executablePath = func() (string, error) {
		return filepath.Join(homeDir, "bin", "kandev"), nil
	}
	var got managedAppConfig
	launchManaged = func(_ context.Context, cfg managedAppConfig) int {
		got = cfg
		return 42
	}

	if code := runStart(context.Background(), Options{Command: CommandStart, Headless: true}); code != 42 {
		t.Fatalf("runStart() = %d, want 42", code)
	}
	if got.Ports.BackendURL != "http://192.0.2.10:48123" {
		t.Fatalf("BackendURL = %q, want the configured bind address", got.Ports.BackendURL)
	}
}

func TestRunManagedAppAttachesSignalsBeforeBackendLaunch(t *testing.T) {
	oldNewSupervisor := newSupervisorFn
	oldLaunchBackend := launchBackendFn
	oldAttachSignals := attachSignalsFn
	oldStartParentWatch := startParentWatchFn
	oldWaitForHealth := waitForHealthFn
	oldWaitForReady := waitForReadyFn
	t.Cleanup(func() {
		newSupervisorFn = oldNewSupervisor
		launchBackendFn = oldLaunchBackend
		attachSignalsFn = oldAttachSignals
		startParentWatchFn = oldStartParentWatch
		waitForHealthFn = oldWaitForHealth
		waitForReadyFn = oldWaitForReady
	})

	var events []string
	var launchedQuiet bool
	var launchedHealthToken string
	var launchedHomeDir string
	var waitedHealthToken string
	newSupervisorFn = func() *processSupervisor {
		events = append(events, "new-supervisor")
		return newSupervisor()
	}
	launchBackendFn = func(cfg backendLaunchConfig) (*restartableBackend, func(), error) {
		launchedQuiet = cfg.Quiet
		launchedHomeDir = cfg.HomeDir
		for _, item := range cfg.Env {
			if strings.HasPrefix(item, "KANDEV_DESKTOP_HEALTH_TOKEN=") {
				launchedHealthToken = strings.TrimPrefix(item, "KANDEV_DESKTOP_HEALTH_TOKEN=")
			}
		}
		events = append(events, "launch-backend")
		exitCh := make(chan int, 1)
		exitCh <- 0
		return &restartableBackend{exitCh: exitCh}, func() {}, nil
	}
	attachSignalsFn = func(_ *processSupervisor) {
		events = append(events, "attach-signals")
	}
	startParentWatchFn = func(_ *processSupervisor) *parentWatchdog {
		events = append(events, "start-parent-watch")
		return newParentWatchdog(0, nil, nil)
	}
	waitForHealthFn = func(_ context.Context, _ backendEndpointSet, _ childState, _ time.Duration, expectedToken string, _ func()) (string, error) {
		waitedHealthToken = expectedToken
		events = append(events, "wait-health")
		return "", nil
	}
	waitForReadyFn = func(_ context.Context, _ string, _ childState) error {
		events = append(events, "wait-ready")
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
	want := []string{"new-supervisor", "attach-signals", "start-parent-watch", "launch-backend", "wait-health", "wait-ready"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	if launchedQuiet {
		t.Fatal("backend stdout was not forwarded")
	}
	if launchedHomeDir != os.Getenv("KANDEV_HOME_DIR") {
		t.Fatalf("supervisor home dir = %q, want the resolved home %q", launchedHomeDir, os.Getenv("KANDEV_HOME_DIR"))
	}
	if launchedHealthToken == "" || launchedHealthToken != waitedHealthToken {
		t.Fatalf("health token launched=%q waited=%q, want one shared non-empty token", launchedHealthToken, waitedHealthToken)
	}
}

// TestRunManagedAppShutsDownOnReadinessFailure is the regression test for
// R2-1: a launcher that only waited on /health would report "backend ready"
// and open a browser onto the bootstrap handler's 503 the moment the socket
// bound, before startup recovery finished. waitForReadyFn must run after
// waitForHealthFn succeeds, and its failure must abort before either the
// ready print or a browser open, exactly like a health failure does.
func TestRunManagedAppShutsDownOnReadinessFailure(t *testing.T) {
	oldNewSupervisor := newSupervisorFn
	oldLaunchBackend := launchBackendFn
	oldAttachSignals := attachSignalsFn
	oldStartParentWatch := startParentWatchFn
	oldWaitForHealth := waitForHealthFn
	oldWaitForReady := waitForReadyFn
	oldStatusOutput := launcherStatusOutput
	t.Cleanup(func() {
		newSupervisorFn = oldNewSupervisor
		launchBackendFn = oldLaunchBackend
		attachSignalsFn = oldAttachSignals
		startParentWatchFn = oldStartParentWatch
		waitForHealthFn = oldWaitForHealth
		waitForReadyFn = oldWaitForReady
		launcherStatusOutput = oldStatusOutput
	})

	var output strings.Builder
	launcherStatusOutput = &output

	newSupervisorFn = newSupervisor
	attachSignalsFn = func(_ *processSupervisor) {}
	startParentWatchFn = func(_ *processSupervisor) *parentWatchdog {
		return newParentWatchdog(0, nil, nil)
	}
	launchBackendFn = func(_ backendLaunchConfig) (*restartableBackend, func(), error) {
		return &restartableBackend{exitCh: make(chan int, 1)}, func() {}, nil
	}
	waitForHealthFn = func(_ context.Context, _ backendEndpointSet, _ childState, _ time.Duration, _ string, _ func()) (string, error) {
		return "", nil
	}
	readyWaited := false
	waitForReadyFn = func(_ context.Context, _ string, _ childState) error {
		readyWaited = true
		return errors.New("test readiness failure")
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
	if code != 1 {
		t.Fatalf("runManagedApp() = %d, want 1", code)
	}
	if !readyWaited {
		t.Fatal("waitForReadyFn was never called")
	}
	if !strings.Contains(output.String(), "graceful shutdown started") {
		t.Fatalf("supervisor shutdown not triggered on readiness failure:\n%s", output.String())
	}
}

func TestRunManagedAppPreservesDesktopOwnedHealthToken(t *testing.T) {
	oldNewSupervisor := newSupervisorFn
	oldLaunchBackend := launchBackendFn
	oldAttachSignals := attachSignalsFn
	oldWaitForHealth := waitForHealthFn
	oldWaitForReady := waitForReadyFn
	t.Cleanup(func() {
		newSupervisorFn = oldNewSupervisor
		launchBackendFn = oldLaunchBackend
		attachSignalsFn = oldAttachSignals
		waitForHealthFn = oldWaitForHealth
		waitForReadyFn = oldWaitForReady
	})

	const desktopToken = "desktop-owned-token"
	var launchedHealthToken string
	var waitedHealthToken string
	newSupervisorFn = newSupervisor
	attachSignalsFn = func(_ *processSupervisor) {}
	launchBackendFn = func(cfg backendLaunchConfig) (*restartableBackend, func(), error) {
		launchedHealthToken = envValue(cfg.Env, "KANDEV_DESKTOP_HEALTH_TOKEN")
		exitCh := make(chan int, 1)
		exitCh <- 0
		return &restartableBackend{exitCh: exitCh}, func() {}, nil
	}
	waitForHealthFn = func(_ context.Context, _ backendEndpointSet, _ childState, _ time.Duration, expectedToken string, _ func()) (string, error) {
		waitedHealthToken = expectedToken
		return "", nil
	}
	waitForReadyFn = func(_ context.Context, _ string, _ childState) error {
		return nil
	}
	t.Setenv("KANDEV_HOME_DIR", t.TempDir())
	t.Setenv("KANDEV_DESKTOP_NATIVE_NOTIFICATIONS", "true")
	t.Setenv("KANDEV_DESKTOP_HEALTH_TOKEN", desktopToken)

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
	if launchedHealthToken != desktopToken || waitedHealthToken != desktopToken {
		t.Fatalf("health tokens launched=%q waited=%q, want %q", launchedHealthToken, waitedHealthToken, desktopToken)
	}
}

func TestLaunchHealthTokenOwnership(t *testing.T) {
	const staleToken = "stale-token"
	tests := []struct {
		name        string
		marker      string
		healthToken string
		want        string
		wantFresh   bool
	}{
		{name: "desktop-owned", marker: "true", healthToken: "desktop-token", want: "desktop-token"},
		{name: "desktop-owned-without-token", marker: "true", wantFresh: true},
		{name: "ordinary-launch-with-stale-token", healthToken: staleToken, wantFresh: true},
		{name: "non-exact-marker-with-stale-token", marker: "TRUE", healthToken: staleToken, wantFresh: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("KANDEV_DESKTOP_NATIVE_NOTIFICATIONS", test.marker)
			t.Setenv("KANDEV_DESKTOP_HEALTH_TOKEN", test.healthToken)

			got, err := launchHealthToken()
			if err != nil {
				t.Fatal(err)
			}
			if test.wantFresh {
				if got == "" || got == test.healthToken {
					t.Fatalf("health token = %q, want a fresh non-empty token", got)
				}
				return
			}
			if got != test.want {
				t.Fatalf("health token = %q, want %q", got, test.want)
			}
		})
	}
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return strings.TrimPrefix(item, prefix)
		}
	}
	return ""
}

func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()
	value, wasSet := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if wasSet {
			_ = os.Setenv(key, value)
			return
		}
		_ = os.Unsetenv(key)
	})
}
