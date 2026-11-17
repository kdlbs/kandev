//go:build linux || darwin

package launcher

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"
)

const maintenanceSIGPIPEHelperEnv = "KANDEV_MAINTENANCE_SIGPIPE_TEST_MODE"

// @covers AC-PLATFORM-MAINTENANCE-JSON-001.4
func TestRunMaintenanceJSONBrokenPipe(t *testing.T) {
	if mode := os.Getenv(maintenanceSIGPIPEHelperEnv); mode != "" {
		// Notify clears any inherited ignored state in Go; Reset restores
		// ordinary SIGPIPE behavior. This changes only the subprocess.
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGPIPE)
		signal.Reset(syscall.SIGPIPE)
		if mode == "control" {
			_, _ = os.Stdout.Write([]byte("control"))
			os.Exit(70)
		}
		os.Exit(runMaintenance([]string{"database", "--json"}, BuildInfo{}))
	}

	t.Setenv(maintenanceSIGPIPEHelperEnv, "")
	maintenanceJSONFixture(t)
	// A control child must actually die from SIGPIPE. This makes an inherited
	// ignored disposition or a miswired fd fail instead of masking the bug.
	controlStderr, controlErr := maintenanceClosedStdoutChild(t, "control")
	var controlExit *exec.ExitError
	if !errors.As(controlErr, &controlExit) {
		t.Fatalf("SIGPIPE control error = %v, stderr = %q", controlErr, controlStderr)
	}
	status, ok := controlExit.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGPIPE {
		t.Fatalf("SIGPIPE control status = %v, stderr = %q", controlExit, controlStderr)
	}

	stderr, err := maintenanceClosedStdoutChild(t, "maintenance")
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("maintenance error = %v, stderr = %q; want exit 1", err, stderr)
	}
	if !strings.Contains(stderr, "failed to write maintenance output") || !strings.Contains(stderr, "broken pipe") {
		t.Fatalf("maintenance stderr = %q; want write-error diagnostic", stderr)
	}
}

func maintenanceClosedStdoutChild(t *testing.T, mode string) (string, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		_ = writer.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRunMaintenanceJSONBrokenPipe$")
	cmd.Env = append(os.Environ(), maintenanceSIGPIPEHelperEnv+"="+mode)
	cmd.Stdout = writer
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("child failed to finish: %v; stderr = %q", ctx.Err(), stderr.String())
	}
	return stderr.String(), err
}
