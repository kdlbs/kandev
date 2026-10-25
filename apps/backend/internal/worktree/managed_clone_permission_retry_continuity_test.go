//go:build unix

package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPermissionRetryRejectsOriginalContentDriftAfterProof(t *testing.T) {
	fixture := newPermissionRetryFixture(t, "proof-content-drift")
	proof, err := provePermissionOnlySnapshot(context.Background(), fixture.original, fixture.oldSnapshot)
	if err != nil {
		t.Fatalf("prove permission-only snapshot: %v", err)
	}
	manifest, err := checkoutManifest(fixture.original)
	if err != nil {
		t.Fatalf("manifest original after proof: %v", err)
	}
	identityManifest, err := recoveryRequiredIdentityManifest(fixture.original)
	if err != nil {
		t.Fatalf("manifest original identity after proof: %v", err)
	}
	if proof.manifest != manifest || proof.identityManifest != identityManifest {
		t.Fatalf("proof manifests = %s/%s, original manifests = %s/%s", proof.manifest, proof.identityManifest, manifest, identityManifest)
	}

	file := filepath.Join(fixture.original, "group-writable.txt")
	if err := os.WriteFile(file, []byte("changed after permission proof\n"), 0o600); err != nil {
		t.Fatalf("change original after proof: %v", err)
	}
	record := fixture.recoveryRecord
	record.Snapshot = fixture.newSnapshot
	record.State = RecoveryStateSnapshotting
	record.ModeRetry = &recoveryModeRetry{
		Version: 1, PreviousSnapshot: fixture.oldSnapshot,
		PreviousError: fixture.recoveryRecord.Error, PreviousUpdatedAt: fixture.recoveryRecord.UpdatedAt,
		SourceManifest: proof.manifest, SourceIdentityManifest: proof.identityManifest,
	}
	copyCalled := false
	_, err = rebuildRecoverySnapshotWithCopier(
		fixture.original,
		fixture.newSnapshot,
		fixture.original+".kandev-recovery.json",
		record,
		func(source, destination string) error {
			copyCalled = true
			return snapshotCheckout(source, destination)
		},
	)
	if err == nil {
		t.Fatal("snapshot copied source content that changed after the permission proof")
	}
	if copyCalled {
		t.Fatal("snapshot copier ran after source content drift")
	}
}

func TestInterruptedPermissionRetryRejectsOriginalContentDrift(t *testing.T) {
	for _, state := range []RecoveryState{RecoveryStateSnapshotting, RecoveryStateRematerializing} {
		t.Run(string(state), func(t *testing.T) {
			fixture := newPermissionRetryFixture(t, "restart-content-drift-"+string(state))
			record := seedInterruptedPermissionRetry(t, fixture, state)
			if err := writeRecoveryRecord(fixture.original+".kandev-recovery.json", record); err != nil {
				t.Fatalf("seed interrupted retry record: %v", err)
			}
			if err := os.WriteFile(
				filepath.Join(fixture.original, "group-writable.txt"),
				[]byte("changed while the backend was stopped\n"),
				0o600,
			); err != nil {
				t.Fatalf("change original during restart: %v", err)
			}

			admission, err := fixture.manager.AdmitRecovery(fixture.context, fixture.request)
			if admission != nil {
				_ = admission.Release(context.Background())
			}
			if err == nil {
				t.Fatal("interrupted retry adopted an original checkout changed while stopped")
			}
			if fixture.store.claim != nil {
				t.Fatal("rejected interrupted retry retained a durable claim")
			}
		})
	}
}

func TestInterruptedPermissionRetryRejectsOriginalIdentityDriftAfterSnapshot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("set-ID identity drift regression requires ownership changes")
	}
	for _, entry := range []struct {
		name string
		path func(permissionRetryFixture) string
		uid  int
		gid  int
		mode uint32
	}{
		{
			name: "setuid file UID",
			path: func(f permissionRetryFixture) string { return filepath.Join(f.original, "setuid-file") },
			uid:  65534, gid: -1, mode: 0o4755,
		},
		{
			name: "setgid directory GID",
			path: func(f permissionRetryFixture) string { return filepath.Join(f.original, "setgid-directory") },
			uid:  -1, gid: 65534, mode: 0o2750,
		},
	} {
		t.Run(entry.name, func(t *testing.T) {
			fixture := newPermissionRetryFixture(t, "restart-source-identity-drift-"+strings.ReplaceAll(entry.name, " ", "-"))
			record := seedInterruptedPermissionRetry(t, fixture, RecoveryStateRematerializing)
			if err := writeRecoveryRecord(fixture.original+".kandev-recovery.json", record); err != nil {
				t.Fatalf("seed rematerializing retry: %v", err)
			}
			originalPath := entry.path(fixture)
			snapshotPath := filepath.Join(fixture.newSnapshot, filepath.Base(originalPath))
			for _, path := range []string{originalPath, snapshotPath} {
				if err := os.Chown(path, entry.uid, entry.gid); err != nil {
					t.Skipf("change set-ID identity in %s: %v", path, err)
				}
				if err := chmodRecoveryUnixMode(path, entry.mode); err != nil {
					t.Fatalf("restore set-ID mode in %s: %v", path, err)
				}
			}
			if before, err := checkoutManifest(fixture.newSnapshot); err != nil || before != record.Manifest {
				t.Fatalf("identity-only mutation changed v1 manifest: got=%q want=%q err=%v", before, record.Manifest, err)
			}

			admission, err := fixture.manager.AdmitRecovery(fixture.context, fixture.request)
			if admission != nil {
				_ = admission.Release(context.Background())
			}
			if err == nil {
				t.Fatal("rematerializing restart accepted source and snapshot identity drift after proof")
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

func TestPermissionRetryRejectsSetIDIdentityDriftAfterProof(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("set-ID identity proof regression requires ownership changes")
	}
	for _, entry := range []struct {
		name string
		path func(permissionRetryFixture) string
		mode uint32
		uid  int
		gid  int
	}{
		{
			name: "setuid file UID",
			path: func(f permissionRetryFixture) string { return filepath.Join(f.original, "setuid-file") },
			mode: 0o4755, uid: 65534, gid: -1,
		},
		{
			name: "setgid directory GID",
			path: func(f permissionRetryFixture) string { return filepath.Join(f.original, "setgid-directory") },
			mode: 0o2750, uid: -1, gid: 65534,
		},
	} {
		t.Run(entry.name, func(t *testing.T) {
			fixture := newPermissionRetryFixture(t, "proof-identity-drift-"+strings.ReplaceAll(entry.name, " ", "-"))
			proof, err := provePermissionOnlySnapshot(context.Background(), fixture.original, fixture.oldSnapshot)
			if err != nil {
				t.Fatalf("prove permission-only snapshot: %v", err)
			}
			path := entry.path(fixture)
			if err := os.Chown(path, entry.uid, entry.gid); err != nil {
				t.Skipf("host cannot change source identity: %v", err)
			}
			if err := chmodRecoveryUnixMode(path, entry.mode); err != nil {
				t.Fatalf("restore source set-ID mode: %v", err)
			}
			record := fixture.recoveryRecord
			record.Snapshot = fixture.newSnapshot
			record.State = RecoveryStateSnapshotting
			record.ModeRetry = &recoveryModeRetry{
				Version: 1, PreviousSnapshot: fixture.oldSnapshot,
				PreviousError: fixture.recoveryRecord.Error, PreviousUpdatedAt: fixture.recoveryRecord.UpdatedAt,
				SourceManifest: proof.manifest, SourceIdentityManifest: proof.identityManifest,
			}
			copyCalled := false
			_, err = rebuildRecoverySnapshotWithCopier(
				fixture.original,
				fixture.newSnapshot,
				fixture.original+".kandev-recovery.json",
				record,
				func(source, destination string) error {
					copyCalled = true
					return snapshotCheckout(source, destination)
				},
			)
			if err == nil {
				t.Fatal("snapshot copied source with set-ID identity drift")
			}
			if copyCalled {
				t.Fatal("snapshot copier ran after set-ID identity drift")
			}
		})
	}
}
