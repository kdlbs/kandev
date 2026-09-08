package launcher

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogStartupPrintsVersion(t *testing.T) {
	t.Setenv("KANDEV_SERVER_HOST", "127.0.0.1")
	output := captureLauncherStdout(t, func() {
		logStartup("test", "1.2.3", portConfig{
			BackendPort: 38429,
			BackendURL:  "http://localhost:38429",
		}, "", "")
	})

	if !strings.Contains(output, "[kandev] version: 1.2.3\n") {
		t.Fatalf("startup output = %q, missing version", output)
	}
	if strings.Index(output, "[kandev] version:") > strings.Index(output, "[kandev] url:") {
		t.Fatalf("startup output = %q, version must precede URL", output)
	}
}

func TestLogStartupDefaultsEmptyVersionToDev(t *testing.T) {
	t.Setenv("KANDEV_SERVER_HOST", "127.0.0.1")
	output := captureLauncherStdout(t, func() {
		logStartup("test", "", portConfig{
			BackendPort: 38429,
			BackendURL:  "http://localhost:38429",
		}, "", "")
	})

	if got := strings.Count(output, "[kandev] version:"); got != 1 {
		t.Fatalf("startup output = %q, version line count = %d, want 1", output, got)
	}
	if !strings.Contains(output, "[kandev] version: dev\n") {
		t.Fatalf("startup output = %q, want dev fallback", output)
	}
}

func TestRunUsesDevVersionForEmptyBuild(t *testing.T) {
	output := captureLauncherStdout(t, func() {
		if code := Run([]string{"--version"}, BuildInfo{}); code != 0 {
			t.Fatalf("Run(--version) = %d, want 0", code)
		}
	})

	if output != "dev\n" {
		t.Fatalf("--version output = %q, want %q", output, "dev\n")
	}
}

func TestRunStartBuildVersion(t *testing.T) {
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
	t.Setenv("KANDEV_HOME_DIR", canonicalTempDir(t))

	var got managedAppConfig
	launchManaged = func(_ context.Context, cfg managedAppConfig) int {
		got = cfg
		return 42
	}

	if code := runStart(context.Background(), Options{Command: CommandStart, Headless: true}, BuildInfo{Version: "1.2.3"}); code != 42 {
		t.Fatalf("runStart() = %d, want 42", code)
	}
	if got.Version != "1.2.3" {
		t.Fatalf("managed startup version = %q, want 1.2.3", got.Version)
	}
}

func TestRunInstalledBuildVersion(t *testing.T) {
	oldLaunchManaged := launchManaged
	t.Cleanup(func() { launchManaged = oldLaunchManaged })

	bundle := t.TempDir()
	writeFile(t, filepath.Join(bundle, "bin", executableName("kandev")))
	writeFile(t, filepath.Join(bundle, "bin", executableName("agentctl")))
	writeRemoteAgentctlHelpers(t, bundle)
	t.Setenv("KANDEV_BUNDLE_DIR", bundle)
	clearLauncherConfigurationEnvironment(t)
	t.Chdir(t.TempDir())
	homeDir := canonicalTempDir(t)
	t.Setenv("KANDEV_HOME_DIR", homeDir)

	var got managedAppConfig
	launchManaged = func(_ context.Context, cfg managedAppConfig) int {
		got = cfg
		return 42
	}

	if code := runInstalled(context.Background(), Options{Command: CommandRun, Headless: true}, BuildInfo{Version: "1.2.3"}); code != 42 {
		t.Fatalf("runInstalled() = %d, want 42", code)
	}
	if got.Version != "1.2.3" {
		t.Fatalf("managed startup version = %q, want 1.2.3", got.Version)
	}
}
