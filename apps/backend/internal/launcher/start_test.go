package launcher

import (
	"path/filepath"
	"testing"
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
