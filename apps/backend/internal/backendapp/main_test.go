package backendapp

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/backendapp/ownershiplock"
)

func TestBackendStartupConflictStopsBeforeSharedStateInitialization(t *testing.T) {
	home := t.TempDir()
	targets, err := ownershiplock.Targets(home, "sqlite", "")
	if err != nil {
		t.Fatalf("Targets: %v", err)
	}
	owner, err := ownershiplock.Acquire(targets)
	if err != nil {
		t.Fatalf("Acquire primary owner: %v", err)
	}
	t.Cleanup(func() { _ = owner.Close() })

	var stderr bytes.Buffer
	cmd := exec.Command(os.Args[0], "-test.run", "^TestBackendStartupConflictHelper$")
	cmd.Env = append(os.Environ(),
		"KANDEV_BACKEND_OWNERSHIP_HELPER=1",
		"KANDEV_HOME_DIR="+home,
	)
	cmd.Stderr = &stderr
	err = cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("backend helper error = %v, want exit code 1; stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stderr.String(), home) || !strings.Contains(stderr.String(), "separate KANDEV_HOME_DIR") {
		t.Fatalf("conflict stderr = %q, want target and isolation guidance", stderr.String())
	}
	for _, path := range []string{
		filepath.Join(home, "logs"),
		filepath.Join(home, "data", "kandev.db"),
	} {
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("shared-state path %q exists or failed unexpectedly: %v", path, statErr)
		}
	}
}

func TestBackendStartupConflictHelper(t *testing.T) {
	if os.Getenv("KANDEV_BACKEND_OWNERSHIP_HELPER") != "1" {
		return
	}
	os.Exit(Run(nil, BuildInfo{Version: "test"}))
}
