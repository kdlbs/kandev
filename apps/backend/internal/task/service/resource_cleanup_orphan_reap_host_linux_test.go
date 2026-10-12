//go:build linux

package service

import (
	"context"
	"io"
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
	writeProcStatFixture(t, procRoot, 102, "102 (unrelated) S 1 1 1 0\n")

	got, err := snapshotLinuxProc(context.Background(), procRoot, func(path string) ([]byte, error) {
		if path == filepath.Join(procRoot, "101", "stat") {
			if err := os.RemoveAll(filepath.Join(procRoot, "102")); err != nil {
				t.Fatalf("Remove unrelated process after enumeration: %v", err)
			}
		}
		return os.ReadFile(path)
	}, readProcCwdAt)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(got) != 1 || got[0].PID != 101 {
		t.Fatalf("snapshot = %+v, want only target process", got)
	}
}

func TestLinuxOrphanReapHostSnapshotPreservesUnresolvedAncestry(t *testing.T) {
	tests := []struct {
		name      string
		stat      string
		readError error
	}{
		{name: "partial stat", stat: "101 (target) S 1 1 1 0\n", readError: io.ErrUnexpectedEOF},
		{name: "permission denied", stat: "101 (target) S 1 1 1 0\n", readError: &fs.PathError{Op: "open", Err: fs.ErrPermission}},
		{name: "malformed stat", stat: "101 target S 1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			procRoot := t.TempDir()
			writeProcStatFixture(t, procRoot, 101, tt.stat)
			writeProcStatFixture(t, procRoot, 102, "102 (worker) S 101 101 101 0\n")

			workspaceRoot := filepath.Join(procRoot, "workspace")
			got, err := snapshotLinuxProc(context.Background(), procRoot, func(path string) ([]byte, error) {
				if path == filepath.Join(procRoot, "101", "stat") && tt.readError != nil {
					return nil, tt.readError
				}
				return os.ReadFile(path)
			}, func(_ string, pid int) (string, error) {
				if pid == 102 {
					return workspaceRoot, nil
				}
				return "", fs.ErrPermission
			})
			if err != nil {
				t.Fatalf("Snapshot: %v", err)
			}
			if len(got) != 2 {
				t.Fatalf("process count = %d, want 2: %+v", len(got), got)
			}
			if got[0].PID != 101 || got[0].PPID != orphanReapUnresolvedPPID || got[0].Cwd != "" {
				t.Fatalf("unresolved process = %+v", got[0])
			}
			if got[1].PID != 102 || got[1].PPID != 101 {
				t.Fatalf("descendant = %+v, want parent 101 retained", got[1])
			}
			candidates := attributeOrphanReapCandidates(got, []string{workspaceRoot})[workspaceRoot]
			if len(candidates) != 1 || candidates[0].PID != 102 {
				t.Fatalf("workspace candidates = %+v, want descendant 102", candidates)
			}
			ppidByPID := map[int]int{101: got[0].PPID, 102: got[1].PPID}
			_, blocked, inconclusive := orphanReapAncestorOwner(102, ppidByPID, nil)
			if blocked || !inconclusive {
				t.Fatalf("descendant ownership = blocked %v, inconclusive %v; want fail-closed ancestry", blocked, inconclusive)
			}
		})
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
