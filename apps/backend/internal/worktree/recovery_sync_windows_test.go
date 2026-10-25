//go:build windows

package worktree

import (
	"path/filepath"
	"testing"
)

func TestRecoveryRecordSyncIsWindowsCompatible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery.json")
	record := recoveryRecord{
		OperationID: "11111111-1111-1111-1111-111111111111",
		TaskID:      "task-1", WorktreeID: "wt-1", Original: "C:\\tasks\\task-1",
		Snapshot: "C:\\tasks\\task-1.kandev-recovery-snapshot", State: RecoveryStateSnapshotting,
	}
	if err := createRecoveryRecord(path, record); err != nil {
		t.Fatalf("createRecoveryRecord: %v", err)
	}
	record.State = RecoveryStateRematerializing
	if err := writeRecoveryRecord(path, record); err != nil {
		t.Fatalf("writeRecoveryRecord: %v", err)
	}
}
