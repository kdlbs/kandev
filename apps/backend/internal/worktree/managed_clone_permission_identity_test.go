//go:build unix

package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPermissionOnlyBlockedRelocationUsesOriginalIdentityAfterHistoricalBitLoss(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("historical ownership regression requires the backend to own copied evidence")
	}
	fixture := newPermissionRetryFixture(t, "historical-owner-loss")
	setuidOriginal := filepath.Join(fixture.original, "setuid-file")
	setuidSnapshot := filepath.Join(fixture.oldSnapshot, "setuid-file")
	setgidOriginal := filepath.Join(fixture.original, "setgid-directory")
	setgidSnapshot := filepath.Join(fixture.oldSnapshot, "setgid-directory")
	if err := os.Chown(setuidOriginal, 65534, -1); err != nil {
		t.Skipf("set original setuid owner: %v", err)
	}
	if err := chmodRecoveryUnixMode(setuidOriginal, 0o4755); err != nil {
		t.Fatalf("restore original setuid mode: %v", err)
	}
	if err := os.Chown(setgidOriginal, -1, 65534); err != nil {
		t.Skipf("set original setgid group: %v", err)
	}
	if err := chmodRecoveryUnixMode(setgidOriginal, 0o2750); err != nil {
		t.Fatalf("restore original setgid mode: %v", err)
	}
	if err := os.Chown(setuidSnapshot, 0, 0); err != nil {
		t.Fatalf("seed backend-owned historical setuid file: %v", err)
	}
	if err := os.Chown(setgidSnapshot, 0, 0); err != nil {
		t.Fatalf("seed backend-owned historical setgid directory: %v", err)
	}
	if err := chmodRecoveryUnixMode(setuidSnapshot, 0o755); err != nil {
		t.Fatalf("strip historical setuid bit: %v", err)
	}
	if err := chmodRecoveryUnixMode(setgidSnapshot, 0o750); err != nil {
		t.Fatalf("strip historical setgid bit: %v", err)
	}
	oldFileData, err := os.ReadFile(setuidSnapshot)
	if err != nil {
		t.Fatalf("read historical setuid evidence: %v", err)
	}
	oldFileUID, oldFileGID := recoveryTestIdentity(t, statRecoveryEntry(t, setuidSnapshot))
	oldDirUID, oldDirGID := recoveryTestIdentity(t, statRecoveryEntry(t, setgidSnapshot))
	if oldFileUID != 0 || oldFileGID != 0 || oldDirUID != 0 || oldDirGID != 0 {
		t.Fatalf("historical owner identity = file %d/%d, directory %d/%d, want backend root 0/0", oldFileUID, oldFileGID, oldDirUID, oldDirGID)
	}
	if oldFileUID == 65534 || oldDirGID == 65534 {
		t.Fatal("test source identity must differ from backend-owned historical evidence")
	}

	admission, err := fixture.manager.AdmitRecovery(fixture.context, fixture.request)
	if err != nil {
		t.Fatalf("AdmitRecovery from old-copier evidence: %v", err)
	}
	if admission == nil {
		t.Fatal("explicit permission retry returned no admission")
	}
	defer func() { _ = admission.Release(context.Background()) }()
	record, err := readRecoveryRecord(fixture.original + ".kandev-recovery.json")
	if err != nil || record.OperationID != permissionRetryTestOperationID || record.State != RecoveryStateComplete {
		t.Fatalf("recovery operation/state = %+v, %v", record, err)
	}
	for _, path := range []string{fixture.newSnapshot, fixture.replacement} {
		assertRecoveryModeAndIdentity(t, filepath.Join(path, "setuid-file"), 0o4755, 65534, 0, true, false)
		assertRecoveryModeAndIdentity(t, filepath.Join(path, "setgid-directory"), 0o2750, 0, 65534, false, true)
	}
	assertRecoveryModeAndIdentity(t, setuidSnapshot, 0o755, oldFileUID, oldFileGID, true, true)
	assertRecoveryModeAndIdentity(t, setgidSnapshot, 0o750, oldDirUID, oldDirGID, true, true)
	if data, err := os.ReadFile(setuidSnapshot); err != nil || string(data) != string(oldFileData) {
		t.Fatalf("historical snapshot bytes changed: %v", err)
	}
}

func TestPermissionOnlyBlockedRelocationRejectsRematerializingIdentityDrift(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("snapshot owner drift regression requires ownership changes")
	}
	for _, entry := range []struct {
		name string
		path func(permissionRetryFixture) string
		uid  int
		gid  int
		mode uint32
	}{
		{name: "setuid file UID", path: func(f permissionRetryFixture) string { return filepath.Join(f.newSnapshot, "setuid-file") }, uid: 65534, gid: -1, mode: 0o4755},
		{name: "setgid directory GID", path: func(f permissionRetryFixture) string { return filepath.Join(f.newSnapshot, "setgid-directory") }, uid: -1, gid: 65534, mode: 0o2750},
	} {
		t.Run(entry.name, func(t *testing.T) {
			fixture := newPermissionRetryFixture(t, "restart-identity-"+strings.ReplaceAll(entry.name, " ", "-"))
			record := seedInterruptedPermissionRetry(t, fixture, RecoveryStateRematerializing)
			if err := writeRecoveryRecord(fixture.original+".kandev-recovery.json", record); err != nil {
				t.Fatalf("seed rematerializing record: %v", err)
			}
			path := entry.path(fixture)
			if err := os.Chown(path, entry.uid, entry.gid); err != nil {
				t.Skipf("change snapshot owner: %v", err)
			}
			if err := chmodRecoveryUnixMode(path, entry.mode); err != nil {
				t.Fatalf("restore set-ID snapshot mode: %v", err)
			}
			admission, err := fixture.manager.AdmitRecovery(fixture.context, fixture.request)
			if admission != nil {
				_ = admission.Release(context.Background())
			}
			if err == nil {
				t.Fatal("rematerializing restart accepted snapshot identity drift")
			}
			if persisted := fixture.store.worktrees[fixture.worktree.ID]; persisted == nil || persisted.Path != fixture.original {
				t.Fatalf("identity refusal published a canonical replacement: %+v", persisted)
			}
			if fixture.store.claim != nil {
				t.Fatal("identity refusal retained the durable claim")
			}
		})
	}
}
