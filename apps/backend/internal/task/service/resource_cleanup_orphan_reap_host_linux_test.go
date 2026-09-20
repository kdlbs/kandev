//go:build linux

package service

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestLinuxOrphanReapHostSnapshotKeepsSettledProcessTree(t *testing.T) {
	procRoot := t.TempDir()
	writeProcStatFixture(t, procRoot, 101, "101 (root) S 1 1 1 0\n")
	writeProcStatFixture(t, procRoot, 102, "102 (worker) S 101 101 101 0\n")

	got, err := snapshotLinuxProc(context.Background(), procRoot, os.ReadFile, readProcCwdAt)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("process count = %d, want 2: %+v", len(got), got)
	}
	if got[0].PID != 101 || got[0].PPID != 1 || got[0].Command != "root" {
		t.Fatalf("root = %+v", got[0])
	}
	if got[1].PID != 102 || got[1].PPID != 101 || got[1].Command != "worker" {
		t.Fatalf("descendant = %+v", got[1])
	}
}

func TestLinuxOrphanReapHostSnapshotSkipsProcessGoneAfterEnumeration(t *testing.T) {
	procRoot := t.TempDir()
	writeProcStatFixture(t, procRoot, 101, "101 (target) S 1 1 1 0\n")
	if err := os.Mkdir(filepath.Join(procRoot, "102"), 0o755); err != nil {
		t.Fatalf("Mkdir unrelated process: %v", err)
	}

	got, err := snapshotLinuxProc(context.Background(), procRoot, os.ReadFile, readProcCwdAt)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(got) != 1 || got[0].PID != 101 {
		t.Fatalf("snapshot = %+v, want only target process", got)
	}
}

func TestLinuxOrphanReapHostSnapshotRejectsStatPermissionFailure(t *testing.T) {
	procRoot := t.TempDir()
	writeProcStatFixture(t, procRoot, 101, "101 (target) S 1 1 1 0\n")

	_, err := snapshotLinuxProc(context.Background(), procRoot, func(path string) ([]byte, error) {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrPermission}
	}, readProcCwdAt)
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("Snapshot error = %v, want permission denied", err)
	}
}

func TestLinuxOrphanReapHostSnapshotRejectsMalformedStat(t *testing.T) {
	procRoot := t.TempDir()
	writeProcStatFixture(t, procRoot, 101, "101 target S 1\n")

	if _, err := snapshotLinuxProc(context.Background(), procRoot, os.ReadFile, readProcCwdAt); err == nil {
		t.Fatal("Snapshot succeeded for malformed stat")
	}
}

func writeProcStatFixture(t *testing.T, procRoot string, pid int, stat string) {
	t.Helper()
	dir := filepath.Join(procRoot, strconv.Itoa(pid))
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("Mkdir proc entry: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0o600); err != nil {
		t.Fatalf("WriteFile stat: %v", err)
	}
}
