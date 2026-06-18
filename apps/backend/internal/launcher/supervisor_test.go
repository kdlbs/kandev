package launcher

import (
	"path/filepath"
	"testing"
	"time"
)

func TestBuildManifestUsesSameBinaryBackendMode(t *testing.T) {
	env := []string{
		"KANDEV_SERVER_PORT=1234",
		"UNRELATED=value",
	}
	manifest := buildManifest("/opt/kandev/bin/kandev", []string{"__backend"}, "/opt/kandev/bin", env, "/tmp/home", 1234, "run")

	if manifest.BackendExecutable != "/opt/kandev/bin/kandev" {
		t.Fatalf("BackendExecutable = %q", manifest.BackendExecutable)
	}
	if len(manifest.Argv) != 1 || manifest.Argv[0] != "__backend" {
		t.Fatalf("Argv = %v, want [__backend]", manifest.Argv)
	}
	if _, ok := manifest.Env["UNRELATED"]; ok {
		t.Fatalf("manifest contains unrelated env: %+v", manifest.Env)
	}
	if manifest.Env["KANDEV_SERVER_PORT"] != "1234" {
		t.Fatalf("KANDEV_SERVER_PORT = %q", manifest.Env["KANDEV_SERVER_PORT"])
	}
}

func TestPrepareSupervisorEnvWritesExpectedPaths(t *testing.T) {
	home := t.TempDir()
	env, socket, manifest, err := prepareSupervisorEnv(nil, home)
	if err != nil {
		t.Fatal(err)
	}
	if socket != filepath.Join(home, "supervisor", "control.sock") {
		t.Fatalf("socket = %q", socket)
	}
	if manifest != filepath.Join(home, "supervisor", "launch.json") {
		t.Fatalf("manifest = %q", manifest)
	}
	got := allowedSupervisorEnv(env)
	if got["KANDEV_RESTART_ADAPTER"] != "supervisor" {
		t.Fatalf("restart adapter env = %+v", got)
	}
}

func TestRestartFailureNotifiesLauncherExit(t *testing.T) {
	backend := &restartableBackend{
		command:    filepath.Join(t.TempDir(), "missing-kandev"),
		supervisor: newSupervisor(),
		exitCh:     make(chan int, 1),
	}

	backend.restart()

	select {
	case code := <-backend.exitCh:
		if code == 0 {
			t.Fatal("restart failure reported successful exit")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("restart failure did not notify launcher exit")
	}
}
