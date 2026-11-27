//go:build unix

package worktree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareRecoverySnapshotRejectsRequiredIdentityDriftAfterCopy(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("source identity drift regression requires ownership changes")
	}
	source := filepath.Join(t.TempDir(), "original")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatalf("create source checkout: %v", err)
	}
	setuidFile := filepath.Join(source, "setuid-file")
	writeRecoveryUnixModeFixture(t, setuidFile, 0o4755)
	if err := os.Chown(setuidFile, 65534, -1); err != nil {
		t.Skipf("set source setuid owner: %v", err)
	}
	if err := chmodRecoveryUnixMode(setuidFile, 0o4755); err != nil {
		t.Fatalf("restore source setuid mode: %v", err)
	}
	setgidDir := filepath.Join(source, "setgid-directory")
	if err := os.Mkdir(setgidDir, 0o700); err != nil {
		t.Fatalf("create source setgid directory: %v", err)
	}
	if err := os.Chown(setgidDir, -1, 65534); err != nil {
		t.Skipf("set source setgid group: %v", err)
	}
	if err := chmodRecoveryUnixMode(setgidDir, 0o2750); err != nil {
		t.Fatalf("set source setgid mode: %v", err)
	}
	if statRecoveryEntry(t, setuidFile).Mode()&os.ModeSetuid == 0 ||
		statRecoveryEntry(t, setgidDir).Mode()&os.ModeSetgid == 0 {
		t.Skip("host filesystem does not retain set-ID modes after ownership changes")
	}
	snapshot := source + ".kandev-recovery-123e4567-e89b-12d3-a456-426614174000"
	if err := snapshotCheckout(source, snapshot); err != nil {
		t.Fatalf("copy original checkout: %v", err)
	}
	manifest, err := checkoutManifest(snapshot)
	if err != nil {
		t.Fatalf("manifest completed snapshot: %v", err)
	}
	if err := os.Chown(setuidFile, 65533, -1); err != nil {
		t.Fatalf("change source setuid owner after copy: %v", err)
	}
	if err := chmodRecoveryUnixMode(setuidFile, 0o4755); err != nil {
		t.Fatalf("restore changed source setuid mode: %v", err)
	}
	if err := os.Chown(setgidDir, -1, 65533); err != nil {
		t.Fatalf("change source setgid group after copy: %v", err)
	}
	if err := chmodRecoveryUnixMode(setgidDir, 0o2750); err != nil {
		t.Fatalf("restore changed source setgid mode: %v", err)
	}
	if sourceManifest, err := checkoutManifest(source); err != nil || sourceManifest != manifest {
		t.Fatalf("ownership-only mutation changed v1 manifest: source=%q snapshot=%q err=%v", sourceManifest, manifest, err)
	}
	record := recoveryRecord{
		OperationID: "123e4567-e89b-12d3-a456-426614174000", Original: source,
		Snapshot: snapshot, Manifest: manifest, State: RecoveryStateRematerializing,
	}
	if _, err := prepareRecoverySnapshot(source, snapshot, source+".kandev-recovery.json", record); err == nil {
		t.Fatal("completed snapshot adoption accepted source set-ID identity drift")
	}
}

func TestRebuildRecoverySnapshotRejectsRequiredIdentityDriftAfterEntryCopy(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("source identity drift regression requires ownership changes")
	}
	for _, entry := range []struct {
		name     string
		path     func(string) string
		uid, gid int
		mode     uint32
	}{
		{name: "setuid file UID", path: func(root string) string { return filepath.Join(root, "setuid-file") }, uid: 65534, gid: -1, mode: 0o4755},
		{name: "setgid directory GID", path: func(root string) string { return filepath.Join(root, "setgid-directory") }, uid: -1, gid: 65534, mode: 0o2750},
	} {
		t.Run(entry.name, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "original")
			if err := os.Mkdir(source, 0o700); err != nil {
				t.Fatalf("create source checkout: %v", err)
			}
			path := entry.path(source)
			if strings.Contains(entry.name, "directory") {
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatalf("create setgid source directory: %v", err)
				}
			} else {
				writeRecoveryUnixModeFixture(t, path, entry.mode)
			}
			if err := os.Chown(path, entry.uid, entry.gid); err != nil {
				t.Skipf("set source identity: %v", err)
			}
			if err := chmodRecoveryUnixMode(path, entry.mode); err != nil {
				t.Fatalf("restore source special mode: %v", err)
			}
			if statRecoveryEntry(t, path).Mode()&recoverySpecialMode == 0 {
				t.Skip("host filesystem does not retain set-ID modes after ownership changes")
			}

			const operationID = "123e4567-e89b-12d3-a456-426614174000"
			snapshot := source + ".kandev-recovery-" + operationID
			recordPath := source + ".kandev-recovery.json"
			record := recoveryRecord{OperationID: operationID, Original: source, Snapshot: snapshot, State: RecoveryStateSnapshotting}
			changedUID, changedGID := -1, -1
			if entry.uid >= 0 {
				changedUID = entry.uid + 1
			}
			if entry.gid >= 0 {
				changedGID = entry.gid + 1
			}
			_, err := rebuildRecoverySnapshotWithCopier(source, snapshot, recordPath, record, func(from, to string) error {
				if err := snapshotCheckout(from, to); err != nil {
					return err
				}
				if err := os.Chown(path, changedUID, changedGID); err != nil {
					return err
				}
				return chmodRecoveryUnixMode(path, entry.mode)
			})
			if err == nil || !strings.Contains(err.Error(), "identity changed during snapshot") {
				t.Fatalf("snapshot rebuild error = %v, want required identity drift refusal", err)
			}
			if record, readErr := readRecoveryRecord(recordPath); readErr != nil || record.State != RecoveryStateBlocked {
				t.Fatalf("snapshot drift recovery record = %+v, %v", record, readErr)
			}
		})
	}
}
