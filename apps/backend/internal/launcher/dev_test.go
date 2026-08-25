package launcher

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func itoa(n int) string { return strconv.Itoa(n) }

var errTestHealth = errors.New("test health failure")

func TestRunDevLaunchesBackendThenWebAndOpensBrowser(t *testing.T) {
	repo := makeRepoTree(t)
	t.Chdir(repo)
	t.Setenv("KANDEV_TASK_ID", "")
	t.Setenv("KANDEV_DATABASE_PATH", "")
	t.Setenv("KANDEV_HOME_DIR", "")
	t.Setenv("KANDEV_SERVER_HOST", "0.0.0.0")

	oldNewSupervisor := newSupervisorFn
	oldAttachSignals := attachSignalsFn
	oldLaunchBackend := launchBackendFn
	oldWaitForHealth := waitForHealthFn
	oldWaitForReady := waitForReadyFn
	oldWaitForURL := waitForURLFn
	oldOpenBrowser := openBrowserFn
	oldStartProcess := startProcessFn
	t.Cleanup(func() {
		newSupervisorFn = oldNewSupervisor
		attachSignalsFn = oldAttachSignals
		launchBackendFn = oldLaunchBackend
		waitForHealthFn = oldWaitForHealth
		waitForReadyFn = oldWaitForReady
		waitForURLFn = oldWaitForURL
		openBrowserFn = oldOpenBrowser
		startProcessFn = oldStartProcess
	})

	var events []string
	var backendCfg backendLaunchConfig
	var readyURL string
	var webCmd, webArgs []string
	var webCWD, webEnvJoined string
	var webLabel string
	newSupervisorFn = func() *processSupervisor {
		events = append(events, "new-supervisor")
		return newSupervisor()
	}
	attachSignalsFn = func(_ *processSupervisor) {
		events = append(events, "attach-signals")
	}
	launchBackendFn = func(cfg backendLaunchConfig) (*restartableBackend, func(), error) {
		backendCfg = cfg
		events = append(events, "launch-backend")
		exitCh := make(chan int, 1)
		exitCh <- 0
		return &restartableBackend{exitCh: exitCh}, func() {}, nil
	}
	waitForHealthFn = func(_ context.Context, endpoints backendEndpointSet, _ childState, _ time.Duration, _ string, _ func()) (string, error) {
		events = append(events, "wait-health:"+endpoints.accessURL)
		readyURL = "http://127.0.0.2:" + itoa(backendCfg.Ports.BackendPort)
		return readyURL, nil
	}
	waitForReadyFn = func(_ context.Context, url string, _ childState) error {
		events = append(events, "wait-ready:"+url)
		return nil
	}
	waitForURLFn = func(_ context.Context, url string, _ childState, _ time.Duration) error {
		events = append(events, "wait-url:"+url)
		return nil
	}
	openBrowserFn = func(url string) {
		events = append(events, "open:"+url)
	}
	startProcessFn = func(cmd string, args []string, cwd string, env []string, quiet bool, label string, _ *processSupervisor) (*managedProcess, func(), error) {
		webCmd = append([]string{cmd}, args...)
		webArgs = args
		webLabel = label
		webCWD = cwd
		webEnvJoined = strings.Join(env, "\n")
		events = append(events, "start-web:"+label)
		return &managedProcess{}, func() {}, nil
	}

	code := runDev(context.Background(), Options{Command: CommandDev})
	if code != 0 {
		t.Fatalf("runDev() = %d, want 0", code)
	}
	want := []string{
		"new-supervisor",
		"attach-signals",
		"launch-backend",
		"wait-health:http://localhost:" + itoa(backendCfg.Ports.BackendPort),
		"wait-ready:" + readyURL,
		"start-web:web",
		"wait-url:http://localhost:" + itoa(backendCfg.Ports.WebPort),
		"open:" + readyURL,
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	if backendCfg.Command != "make" || !reflect.DeepEqual(backendCfg.Args, []string{"-C", "apps/backend", "dev"}) {
		t.Fatalf("backend child = %q %v, want make -C apps/backend dev", backendCfg.Command, backendCfg.Args)
	}
	if backendCfg.CWD != repo {
		t.Fatalf("backend cwd = %q, want %q", backendCfg.CWD, repo)
	}
	if backendCfg.HomeDir != filepath.Join(repo, ".kandev-dev") {
		t.Fatalf("backend supervisor home = %q, want %q", backendCfg.HomeDir, filepath.Join(repo, ".kandev-dev"))
	}
	if backendCfg.Mode != "dev" {
		t.Fatalf("backend mode = %q, want dev", backendCfg.Mode)
	}

	backendEnv := strings.Join(backendCfg.Env, "\n")
	for _, expected := range []string{
		"KANDEV_WEB_INTERNAL_URL=http://localhost:" + itoa(backendCfg.Ports.WebPort),
		"KANDEV_DEBUG_DEV_MODE=true",
		"KANDEV_HOME_DIR=" + filepath.Join(repo, ".kandev-dev"),
		"KANDEV_DATABASE_PATH=",
		"KANDEV_BACKEND_PID_FILE=" + filepath.Join(repo, ".kandev-dev", "supervisor", "backend.pid"),
	} {
		if !strings.Contains(backendEnv, expected) {
			t.Fatalf("backend environment missing %q:\n%s", expected, backendEnv)
		}
	}

	if webCWD != repo {
		t.Fatalf("web cwd = %q, want %q", webCWD, repo)
	}
	if webLabel != "web" {
		t.Fatalf("web label = %q, want web", webLabel)
	}
	if !reflect.DeepEqual(webArgs, []string{"-C", "apps", "--filter", "@kandev/web", "dev"}) {
		t.Fatalf("web args = %v, want the Vite dev command", webArgs)
	}
	for _, expected := range []string{
		"KANDEV_API_BASE_URL=" + readyURL,
		"PORT=" + itoa(backendCfg.Ports.WebPort),
		"HOSTNAME=127.0.0.1",
		"KANDEV_DEBUG=true",
		"VITE_KANDEV_DEBUG=true",
	} {
		if !strings.Contains(webEnvJoined, expected) {
			t.Fatalf("web environment missing %q:\n%s", expected, webEnvJoined)
		}
	}
	if strings.Contains(webEnvJoined, "VITE_KANDEV_API_PORT") {
		t.Fatalf("web environment leaked VITE_KANDEV_API_PORT:\n%s", webEnvJoined)
	}
	if webCmd[0] == "cmd.exe" {
		t.Fatalf("web command = %v, want a plain pnpm invocation on this platform", webCmd)
	}
}

func TestRunDevUsesConfiguredHealthTimeoutForBackendAndWeb(t *testing.T) {
	clearLauncherConfigurationEnvironment(t)
	repo := makeRepoTree(t)
	t.Chdir(repo)
	writeLauncherConfig(t, filepath.Join(repo, "config.yaml"), "launcher:\n  healthTimeoutMs: 12345\n")
	t.Setenv("KANDEV_TASK_ID", "")
	t.Setenv("KANDEV_DATABASE_PATH", "")
	t.Setenv("KANDEV_HOME_DIR", "")

	oldNewSupervisor := newSupervisorFn
	oldAttachSignals := attachSignalsFn
	oldLaunchBackend := launchBackendFn
	oldWaitForHealth := waitForHealthFn
	oldWaitForReady := waitForReadyFn
	oldWaitForURL := waitForURLFn
	oldOpenBrowser := openBrowserFn
	oldStartProcess := startProcessFn
	t.Cleanup(func() {
		newSupervisorFn = oldNewSupervisor
		attachSignalsFn = oldAttachSignals
		launchBackendFn = oldLaunchBackend
		waitForHealthFn = oldWaitForHealth
		waitForReadyFn = oldWaitForReady
		waitForURLFn = oldWaitForURL
		openBrowserFn = oldOpenBrowser
		startProcessFn = oldStartProcess
	})

	newSupervisorFn = newSupervisor
	attachSignalsFn = func(_ *processSupervisor) {}
	launchBackendFn = func(_ backendLaunchConfig) (*restartableBackend, func(), error) {
		exitCh := make(chan int, 1)
		exitCh <- 0
		return &restartableBackend{exitCh: exitCh}, func() {}, nil
	}
	var backendTimeout, webTimeout time.Duration
	waitForHealthFn = func(_ context.Context, _ backendEndpointSet, _ childState, timeout time.Duration, _ string, _ func()) (string, error) {
		backendTimeout = timeout
		return "", nil
	}
	waitForReadyFn = func(_ context.Context, _ string, _ childState) error {
		return nil
	}
	waitForURLFn = func(_ context.Context, _ string, _ childState, timeout time.Duration) error {
		webTimeout = timeout
		return nil
	}
	openBrowserFn = func(string) {}
	startProcessFn = func(_ string, _ []string, _ string, _ []string, _ bool, _ string, _ *processSupervisor) (*managedProcess, func(), error) {
		return &managedProcess{}, func() {}, nil
	}

	if code := runDev(context.Background(), Options{Command: CommandDev}); code != 0 {
		t.Fatalf("runDev() = %d, want 0", code)
	}
	want := 12345 * time.Millisecond
	if backendTimeout != want || webTimeout != want {
		t.Fatalf("health timeouts = backend %s, web %s; want %s for both", backendTimeout, webTimeout, want)
	}
}

func TestRunDevHeadlessSkipsBrowser(t *testing.T) {
	repo := makeRepoTree(t)
	t.Chdir(repo)
	t.Setenv("KANDEV_TASK_ID", "")
	t.Setenv("KANDEV_DATABASE_PATH", "")
	t.Setenv("KANDEV_HOME_DIR", "")

	oldLaunchBackend := launchBackendFn
	oldWaitForHealth := waitForHealthFn
	oldWaitForReady := waitForReadyFn
	oldWaitForURL := waitForURLFn
	oldOpenBrowser := openBrowserFn
	oldStartProcess := startProcessFn
	oldAttachSignals := attachSignalsFn
	t.Cleanup(func() {
		launchBackendFn = oldLaunchBackend
		waitForHealthFn = oldWaitForHealth
		waitForReadyFn = oldWaitForReady
		waitForURLFn = oldWaitForURL
		openBrowserFn = oldOpenBrowser
		startProcessFn = oldStartProcess
		attachSignalsFn = oldAttachSignals
	})

	browserOpened := false
	attachSignalsFn = func(_ *processSupervisor) {}
	launchBackendFn = func(cfg backendLaunchConfig) (*restartableBackend, func(), error) {
		exitCh := make(chan int, 1)
		exitCh <- 0
		return &restartableBackend{exitCh: exitCh}, func() {}, nil
	}
	waitForHealthFn = func(_ context.Context, _ backendEndpointSet, _ childState, _ time.Duration, _ string, _ func()) (string, error) {
		return "", nil
	}
	waitForReadyFn = func(_ context.Context, _ string, _ childState) error {
		return nil
	}
	waitForURLFn = func(_ context.Context, _ string, _ childState, _ time.Duration) error {
		return nil
	}
	openBrowserFn = func(_ string) { browserOpened = true }
	startProcessFn = func(_ string, _ []string, _ string, _ []string, _ bool, _ string, _ *processSupervisor) (*managedProcess, func(), error) {
		return &managedProcess{}, func() {}, nil
	}

	code := runDev(context.Background(), Options{Command: CommandDev, Headless: true})
	if code != 0 {
		t.Fatalf("runDev(headless) = %d, want 0", code)
	}
	if browserOpened {
		t.Fatal("runDev(headless) opened the browser")
	}
}

func TestRunDevAbortsOnBackupFailureBeforeLaunch(t *testing.T) {
	repo := makeRepoTree(t)
	t.Chdir(repo)
	t.Setenv("KANDEV_TASK_ID", "")
	// A path without a .kandev-dev segment is treated as production; making it
	// a directory forces the copy to fail so the abort path runs.
	t.Setenv("HOME", t.TempDir())
	dbDir := t.TempDir()
	t.Setenv("KANDEV_DATABASE_PATH", filepath.Join(dbDir, "kandev.db"))
	if err := os.Mkdir(filepath.Join(dbDir, "kandev.db"), 0o700); err != nil {
		t.Fatal(err)
	}

	oldLaunchBackend := launchBackendFn
	t.Cleanup(func() { launchBackendFn = oldLaunchBackend })
	launched := false
	launchBackendFn = func(_ backendLaunchConfig) (*restartableBackend, func(), error) {
		launched = true
		return nil, nil, nil
	}

	code := runDev(context.Background(), Options{Command: CommandDev})
	if code != 1 {
		t.Fatalf("runDev() = %d, want 1", code)
	}
	if launched {
		t.Fatal("backend launched despite a failed production-db backup")
	}
}

func TestRunDevShutsDownTreeOnBackendHealthFailure(t *testing.T) {
	repo := makeRepoTree(t)
	t.Chdir(repo)
	t.Setenv("KANDEV_TASK_ID", "")
	t.Setenv("KANDEV_DATABASE_PATH", "")
	t.Setenv("KANDEV_HOME_DIR", "")

	oldLaunchBackend := launchBackendFn
	oldWaitForHealth := waitForHealthFn
	oldWaitForURL := waitForURLFn
	oldOpenBrowser := openBrowserFn
	oldStartProcess := startProcessFn
	oldAttachSignals := attachSignalsFn
	oldStatusOutput := launcherStatusOutput
	t.Cleanup(func() {
		launchBackendFn = oldLaunchBackend
		waitForHealthFn = oldWaitForHealth
		waitForURLFn = oldWaitForURL
		openBrowserFn = oldOpenBrowser
		startProcessFn = oldStartProcess
		attachSignalsFn = oldAttachSignals
		launcherStatusOutput = oldStatusOutput
	})

	var output strings.Builder
	launcherStatusOutput = &output

	browserOpened := false
	attachSignalsFn = func(_ *processSupervisor) {}
	launchBackendFn = func(_ backendLaunchConfig) (*restartableBackend, func(), error) {
		return &restartableBackend{exitCh: make(chan int, 1)}, func() {}, nil
	}
	waitForHealthFn = func(_ context.Context, _ backendEndpointSet, _ childState, _ time.Duration, _ string, _ func()) (string, error) {
		return "", errTestHealth
	}
	waitForURLFn = func(_ context.Context, _ string, _ childState, _ time.Duration) error {
		t.Fatal("waitForURL called after backend health failure")
		return nil
	}
	openBrowserFn = func(_ string) { browserOpened = true }
	startProcessFn = func(_ string, _ []string, _ string, _ []string, _ bool, _ string, _ *processSupervisor) (*managedProcess, func(), error) {
		t.Fatal("web child launched after backend health failure")
		return nil, nil, nil
	}

	code := runDev(context.Background(), Options{Command: CommandDev})
	if code != 1 {
		t.Fatalf("runDev() = %d, want 1", code)
	}
	if !strings.Contains(output.String(), "graceful shutdown started") {
		t.Fatalf("supervisor shutdown not triggered on health failure:\n%s", output.String())
	}
	if browserOpened {
		t.Fatal("browser opened despite health failure")
	}
}

// TestRunDevShutsDownTreeOnBackendReadinessFailure is the R2-1 regression
// test for dev mode: it must not launch the web child or open the browser
// onto a backend that answered /health but never became ready.
func TestRunDevShutsDownTreeOnBackendReadinessFailure(t *testing.T) {
	repo := makeRepoTree(t)
	t.Chdir(repo)
	t.Setenv("KANDEV_TASK_ID", "")
	t.Setenv("KANDEV_DATABASE_PATH", "")
	t.Setenv("KANDEV_HOME_DIR", "")

	oldLaunchBackend := launchBackendFn
	oldWaitForHealth := waitForHealthFn
	oldWaitForReady := waitForReadyFn
	oldWaitForURL := waitForURLFn
	oldOpenBrowser := openBrowserFn
	oldStartProcess := startProcessFn
	oldAttachSignals := attachSignalsFn
	oldStatusOutput := launcherStatusOutput
	t.Cleanup(func() {
		launchBackendFn = oldLaunchBackend
		waitForHealthFn = oldWaitForHealth
		waitForReadyFn = oldWaitForReady
		waitForURLFn = oldWaitForURL
		openBrowserFn = oldOpenBrowser
		startProcessFn = oldStartProcess
		attachSignalsFn = oldAttachSignals
		launcherStatusOutput = oldStatusOutput
	})

	var output strings.Builder
	launcherStatusOutput = &output

	browserOpened := false
	attachSignalsFn = func(_ *processSupervisor) {}
	launchBackendFn = func(_ backendLaunchConfig) (*restartableBackend, func(), error) {
		return &restartableBackend{exitCh: make(chan int, 1)}, func() {}, nil
	}
	waitForHealthFn = func(_ context.Context, _ backendEndpointSet, _ childState, _ time.Duration, _ string, _ func()) (string, error) {
		return "", nil
	}
	waitForReadyFn = func(_ context.Context, _ string, _ childState) error {
		return errTestHealth
	}
	waitForURLFn = func(_ context.Context, _ string, _ childState, _ time.Duration) error {
		t.Fatal("waitForURL called after backend readiness failure")
		return nil
	}
	openBrowserFn = func(_ string) { browserOpened = true }
	startProcessFn = func(_ string, _ []string, _ string, _ []string, _ bool, _ string, _ *processSupervisor) (*managedProcess, func(), error) {
		t.Fatal("web child launched after backend readiness failure")
		return nil, nil, nil
	}

	code := runDev(context.Background(), Options{Command: CommandDev})
	if code != 1 {
		t.Fatalf("runDev() = %d, want 1", code)
	}
	if !strings.Contains(output.String(), "graceful shutdown started") {
		t.Fatalf("supervisor shutdown not triggered on readiness failure:\n%s", output.String())
	}
	if browserOpened {
		t.Fatal("browser opened despite readiness failure")
	}
}

func TestRunDevShutsDownTreeWhenWebChildExits(t *testing.T) {
	repo := makeRepoTree(t)
	t.Chdir(repo)
	t.Setenv("KANDEV_TASK_ID", "")
	t.Setenv("KANDEV_DATABASE_PATH", "")
	t.Setenv("KANDEV_HOME_DIR", "")

	oldLaunchBackend := launchBackendFn
	oldWaitForHealth := waitForHealthFn
	oldWaitForReady := waitForReadyFn
	oldWaitForURL := waitForURLFn
	oldOpenBrowser := openBrowserFn
	oldStartProcess := startProcessFn
	oldAttachSignals := attachSignalsFn
	oldStatusOutput := launcherStatusOutput
	t.Cleanup(func() {
		launchBackendFn = oldLaunchBackend
		waitForHealthFn = oldWaitForHealth
		waitForReadyFn = oldWaitForReady
		waitForURLFn = oldWaitForURL
		openBrowserFn = oldOpenBrowser
		startProcessFn = oldStartProcess
		attachSignalsFn = oldAttachSignals
		launcherStatusOutput = oldStatusOutput
	})

	var output strings.Builder
	launcherStatusOutput = &output

	attachSignalsFn = func(_ *processSupervisor) {}
	// The backend stays alive; only the web child exits. waitForAppExit must
	// select the web child and take the launcher down with its exit code.
	launchBackendFn = func(_ backendLaunchConfig) (*restartableBackend, func(), error) {
		return &restartableBackend{exitCh: make(chan int, 1)}, func() {}, nil
	}
	waitForHealthFn = func(_ context.Context, _ backendEndpointSet, _ childState, _ time.Duration, _ string, _ func()) (string, error) {
		return "", nil
	}
	waitForReadyFn = func(_ context.Context, _ string, _ childState) error {
		return nil
	}
	waitForURLFn = func(_ context.Context, _ string, _ childState, _ time.Duration) error {
		return nil
	}
	openBrowserFn = func(_ string) {}
	webDone := make(chan struct{})
	startProcessFn = func(_ string, _ []string, _ string, _ []string, _ bool, label string, _ *processSupervisor) (*managedProcess, func(), error) {
		return &managedProcess{label: label, exited: true, exitCode: 7, done: webDone}, func() {}, nil
	}
	// The web child has already exited: a pre-closed done channel is the
	// signal waitForAppExit selects on.
	close(webDone)

	code := runDev(context.Background(), Options{Command: CommandDev, Headless: true})
	if code != 7 {
		t.Fatalf("runDev() = %d, want the web child's exit code 7", code)
	}
	if !strings.Contains(output.String(), "reason=web exit") {
		t.Fatalf("supervisor not shut down for the web child exit:\n%s", output.String())
	}
}

func TestRunDevFailsWhenRepoRootNotFound(t *testing.T) {
	t.Chdir(t.TempDir())

	code := runDev(context.Background(), Options{Command: CommandDev})
	if code != 2 {
		t.Fatalf("runDev() = %d, want 2", code)
	}
}
