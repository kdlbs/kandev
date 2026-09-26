package backendapp

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/backendapp/ownershiplock"
	"github.com/kandev/kandev/internal/common/config"
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
	marker := parseStartupConflictMarker(t, stderr.String())
	if marker.TargetKind != "home" || marker.TargetPath != home {
		t.Fatalf("conflict marker target = (%q, %q), want (home, %q)", marker.TargetKind, marker.TargetPath, home)
	}
	if marker.StorageKind != "sqlite_in_home" || marker.DatabasePath != filepath.Join(home, "data", "kandev.db") {
		t.Fatalf("conflict marker storage = (%q, %q), want in-home SQLite path", marker.StorageKind, marker.DatabasePath)
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

func TestBackendStartupExternalDatabaseConflictDiagnostic(t *testing.T) {
	primaryHome := t.TempDir()
	secondaryHome := t.TempDir()
	databasePath := filepath.Join(t.TempDir(), "shared.db")
	targets, err := ownershiplock.Targets(primaryHome, "sqlite", databasePath)
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
		"KANDEV_HOME_DIR="+secondaryHome,
		"KANDEV_DATABASE_DRIVER=sqlite",
		"KANDEV_DATABASE_PATH="+databasePath,
	)
	cmd.Stderr = &stderr
	err = cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("backend helper error = %v, want exit code 1; stderr=%s", err, stderr.String())
	}
	marker := parseStartupConflictMarker(t, stderr.String())
	if marker.TargetKind != "database" || marker.TargetPath != databasePath {
		t.Fatalf("conflict marker target = (%q, %q), want (database, %q)", marker.TargetKind, marker.TargetPath, databasePath)
	}
	if marker.StorageKind != "sqlite_external" || marker.DatabasePath != databasePath {
		t.Fatalf("conflict marker storage = (%q, %q), want external SQLite path", marker.StorageKind, marker.DatabasePath)
	}
}

func TestBackendStartupUnrelatedFailuresDoNotEmitConflictDiagnostic(t *testing.T) {
	t.Run("invalid configuration", func(t *testing.T) {
		var stderr bytes.Buffer
		cmd := exec.Command(os.Args[0], "-test.run", "^TestBackendStartupConflictHelper$")
		cmd.Env = append(os.Environ(),
			"KANDEV_BACKEND_OWNERSHIP_HELPER=1",
			"KANDEV_DATABASE_DRIVER=invalid",
		)
		cmd.Stderr = &stderr
		if err := cmd.Run(); err == nil {
			t.Fatal("backend helper succeeded with an invalid database driver")
		}
		assertNoStartupConflictMarker(t, stderr.String())
	})

	t.Run("ownership target resolution error", func(t *testing.T) {
		dir := t.TempDir()
		loop := filepath.Join(dir, "loop")
		if err := os.Symlink(loop, loop); err != nil {
			t.Skipf("create symlink loop: %v", err)
		}
		var stderr bytes.Buffer
		cmd := exec.Command(os.Args[0], "-test.run", "^TestBackendStartupConflictHelper$")
		cmd.Env = append(os.Environ(),
			"KANDEV_BACKEND_OWNERSHIP_HELPER=1",
			"KANDEV_HOME_DIR="+loop,
		)
		cmd.Stderr = &stderr
		if err := cmd.Run(); err == nil {
			t.Fatal("backend helper succeeded with an invalid home path")
		}
		assertNoStartupConflictMarker(t, stderr.String())
	})
}

func TestWriteDesktopStartupConflictMarkerRequiresConflictError(t *testing.T) {
	var output bytes.Buffer
	cfg := &config.Config{Database: config.DatabaseConfig{Driver: "sqlite"}}
	if wrote := writeDesktopStartupConflictMarker(&output, cfg, errors.New("permission denied")); wrote {
		t.Fatal("writeDesktopStartupConflictMarker wrote a marker for a non-conflict error")
	}
	if output.Len() != 0 {
		t.Fatalf("marker output = %q, want empty", output.String())
	}
}

func TestWriteDesktopStartupConflictMarkerBoundsOwnerDetails(t *testing.T) {
	home := t.TempDir()
	var output bytes.Buffer
	err := &ownershiplock.ConflictError{
		Target: ownershiplock.Target{Kind: ownershiplock.TargetHome, ResourcePath: home},
		Owner: &ownershiplock.OwnerRecord{
			PID:        42,
			Executable: "/tmp/" + strings.Repeat("x", 600) + "\nwith-control",
			StartedAt:  "2026-09-25T12:00:00Z",
		},
	}
	cfg := &config.Config{HomeDir: home, Database: config.DatabaseConfig{Driver: "sqlite"}}
	if wrote := writeDesktopStartupConflictMarker(&output, cfg, err); !wrote {
		t.Fatal("writeDesktopStartupConflictMarker did not write a typed conflict")
	}
	marker := parseStartupConflictMarker(t, output.String())
	if marker.Owner.PID != 42 || len(marker.Owner.Executable) > 256 || strings.ContainsAny(marker.Owner.Executable, "\r\n\x1b") {
		t.Fatalf("owner details were not safely bounded: %+v", marker.Owner)
	}
}

func TestBackendStartupConflictHelper(t *testing.T) {
	if os.Getenv("KANDEV_BACKEND_OWNERSHIP_HELPER") != "1" {
		return
	}
	os.Exit(Run(nil, BuildInfo{Version: "test"}))
}

type startupConflictMarker struct {
	Version      int    `json:"version"`
	TargetKind   string `json:"target_kind"`
	TargetPath   string `json:"target_path"`
	StorageKind  string `json:"storage_kind"`
	DatabasePath string `json:"database_path"`
	Owner        struct {
		PID        int64  `json:"pid"`
		Executable string `json:"executable"`
		StartedAt  string `json:"started_at"`
	} `json:"owner"`
}

func parseStartupConflictMarker(t *testing.T, output string) startupConflictMarker {
	t.Helper()
	const prefix = "KANDEV_DESKTOP_CONFLICT_V1 "
	var marker startupConflictMarker
	count := 0
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		count++
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, prefix)), &marker); err != nil {
			t.Fatalf("decode startup conflict marker: %v; line=%q", err, line)
		}
	}
	if count != 1 {
		t.Fatalf("startup conflict marker count = %d, want 1; stderr=%q", count, output)
	}
	if marker.Version != 1 {
		t.Fatalf("startup conflict marker version = %d, want 1", marker.Version)
	}
	return marker
}

func assertNoStartupConflictMarker(t *testing.T, output string) {
	t.Helper()
	if strings.Contains(output, "KANDEV_DESKTOP_CONFLICT_V1") {
		t.Fatalf("unexpected startup conflict marker in stderr: %q", output)
	}
}
