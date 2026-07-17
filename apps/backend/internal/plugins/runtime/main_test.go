package runtime

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"testing"

	"go.uber.org/goleak"
)

// fixtureBinPath is the path to the real SDK-based plugin binary built by
// TestMain, used by the end-to-end tests in manager_test.go that exercise a
// real subprocess spawn/handshake/DeliverEvent/InvokeTool/crash-restart
// cycle. Empty when tests run with -short (no real subprocess tests run in
// that mode).
var fixtureBinPath string

// TestMain builds testdata/fixtureplugin (a real pkg/pluginsdk-based plugin
// backend) once for the whole package's test run, then asserts no
// goroutines from this package (the supervision loops process.go spawns)
// outlive the test process. Building is skipped under -short, per
// apps/backend/AGENTS.md's guidance to reserve real subprocess execution
// for integration tests.
func TestMain(m *testing.M) {
	flag.Parse()
	if !testing.Short() {
		binPath, err := buildFixturePlugin()
		if err != nil {
			fmt.Fprintln(os.Stderr, "runtime: building fixtureplugin:", err)
			os.Exit(1)
		}
		fixtureBinPath = binPath
	}
	goleak.VerifyTestMain(m)
}

// buildFixturePlugin compiles testdata/fixtureplugin into a temp binary and
// returns its path. The temp dir is intentionally not cleaned up here:
// goleak.VerifyTestMain calls os.Exit internally, which would skip a
// deferred cleanup anyway; the OS/CI temp-dir lifecycle handles it.
func buildFixturePlugin() (string, error) {
	dir, err := os.MkdirTemp("", "plugin-runtime-fixture-")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	binName := "fixtureplugin"
	if goruntime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(dir, binName)

	pkgDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}

	cmd := exec.Command("go", "build", "-o", binPath, "./testdata/fixtureplugin")
	cmd.Dir = pkgDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build fixtureplugin: %w: %s", err, out)
	}
	return binPath, nil
}
