package launcher

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// @covers AC-PLATFORM-GO-DEV-LAUNCHER-001.12
func TestRunDevProductionBackupAdmission(t *testing.T) {
	for _, name := range []string{"invalid_source", "canceled", "valid_source"} {
		t.Run(name, func(t *testing.T) {
			clearLauncherConfigurationEnvironment(t)
			t.Chdir(makeRepoTree(t))
			t.Setenv("KANDEV_TASK_ID", "")
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			source := filepath.Join(t.TempDir(), "production.db")
			t.Setenv("KANDEV_DATABASE_PATH", source)
			if name == "invalid_source" {
				if err := os.WriteFile(source, []byte("invalid SQLite"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else {
				createBackupFixture(t, source, false)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if name == "canceled" {
				cancel()
			}
			oldLaunch := launchBackendFn
			oldAttachSignals := attachSignalsFn
			t.Cleanup(func() {
				launchBackendFn = oldLaunch
				attachSignalsFn = oldAttachSignals
			})
			attachSignalsFn = func(_ *processSupervisor) {}
			launched := false
			launchBackendFn = func(_ backendLaunchConfig) (*restartableBackend, func(), error) {
				launched = true
				return nil, nil, errors.New("test backend boundary reached")
			}
			var code int
			output := captureLauncherStdout(t, func() {
				code = runDev(ctx, Options{Command: CommandDev}, BuildInfo{})
			})
			if code != 1 {
				t.Fatalf("runDev = %d, want 1", code)
			}
			wantLaunch := name == "valid_source"
			if launched != wantLaunch {
				t.Fatalf("backend boundary reached = %v, want %v", launched, wantLaunch)
			}
			if strings.Contains(output, "backed up production db") != wantLaunch {
				t.Fatalf("unexpected backup success output: %q", output)
			}
		})
	}
}
