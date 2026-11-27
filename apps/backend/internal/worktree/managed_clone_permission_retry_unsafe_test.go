//go:build unix

package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPermissionOnlyBlockedRelocationRejectsUnsafeRetry(t *testing.T) {
	fixture := newPermissionRetryFixture(t, "unsafe")
	groupFile := filepath.Join(fixture.oldSnapshot, "group-writable.txt")
	setgidDir := filepath.Join(fixture.oldSnapshot, "setgid-directory")
	originalFile := filepath.Join(fixture.original, "group-writable.txt")
	link := filepath.Join(fixture.oldSnapshot, "dirty-link")
	originalRecord := fixture.recoveryRecord
	originalRelocation := fixture.relocation
	cases := []struct {
		name   string
		mutate func(*testing.T) func()
	}{
		{
			name: "source content drift",
			mutate: func(t *testing.T) func() {
				if err := os.WriteFile(originalFile, []byte("source changed after failure\n"), 0o775); err != nil {
					t.Fatalf("change original checkout content: %v", err)
				}
				return func() {
					if err := os.WriteFile(originalFile, []byte("permission retry content\n"), 0o775); err != nil {
						t.Errorf("restore original checkout content: %v", err)
					}
				}
			},
		},
		{
			name: "snapshot entry membership drift",
			mutate: func(t *testing.T) func() {
				path := filepath.Join(fixture.oldSnapshot, "foreign-entry")
				if err := os.WriteFile(path, []byte("foreign"), 0o600); err != nil {
					t.Fatalf("add historical snapshot entry: %v", err)
				}
				return func() { _ = os.Remove(path) }
			},
		},
		{
			name: "symbolic link target drift",
			mutate: func(t *testing.T) func() {
				if err := os.Remove(link); err != nil {
					t.Fatalf("remove historical symbolic link: %v", err)
				}
				if err := os.Symlink("setuid-file", link); err != nil {
					t.Fatalf("change historical symbolic link target: %v", err)
				}
				return func() {
					_ = os.Remove(link)
					if err := os.Symlink("group-writable.txt", link); err != nil {
						t.Errorf("restore historical symbolic link: %v", err)
					}
				}
			},
		},
		{
			name: "content drift",
			mutate: func(t *testing.T) func() {
				if err := os.WriteFile(groupFile, []byte("changed snapshot content\n"), 0o600); err != nil {
					t.Fatalf("change historical snapshot content: %v", err)
				}
				return func() {
					if err := os.WriteFile(groupFile, []byte("permission retry content\n"), 0o600); err != nil {
						t.Errorf("restore historical snapshot content: %v", err)
					}
					if err := chmodRecoveryUnixMode(groupFile, 0o755); err != nil {
						t.Errorf("restore historical snapshot mode: %v", err)
					}
				}
			},
		},
		{
			name: "snapshot permission gain",
			mutate: func(t *testing.T) func() {
				if err := chmodRecoveryUnixMode(groupFile, 0o777); err != nil {
					t.Fatalf("add historical snapshot permissions: %v", err)
				}
				return func() {
					if err := chmodRecoveryUnixMode(groupFile, 0o755); err != nil {
						t.Errorf("restore historical snapshot permissions: %v", err)
					}
				}
			},
		},
		{
			name: "setuid owner drift",
			mutate: func(t *testing.T) func() {
				path := filepath.Join(fixture.oldSnapshot, "setuid-file")
				sourceUID, _ := recoveryTestIdentity(t, statRecoveryEntry(t, filepath.Join(fixture.original, "setuid-file")))
				if os.Geteuid() != 0 {
					t.Skip("setuid owner test requires host ownership support")
				}
				targetUID := 1
				if sourceUID == targetUID {
					targetUID = 2
				}
				if err := os.Chown(path, targetUID, -1); err != nil {
					t.Skipf("host cannot change test snapshot owner: %v", err)
				}
				if err := chmodRecoveryUnixMode(path, 0o4755); err != nil {
					t.Fatalf("restore retained snapshot setuid mode: %v", err)
				}
				return func() {
					if err := os.Chown(path, sourceUID, -1); err != nil {
						t.Errorf("restore snapshot owner: %v", err)
					}
					if err := chmodRecoveryUnixMode(path, 0o755); err != nil {
						t.Errorf("restore snapshot mode: %v", err)
					}
				}
			},
		},
		{
			name: "directory permission drift",
			mutate: func(t *testing.T) func() {
				if err := chmodRecoveryUnixMode(setgidDir, 0o700); err != nil {
					t.Fatalf("change historical directory mode: %v", err)
				}
				return func() {
					if err := chmodRecoveryUnixMode(setgidDir, 0o750); err != nil {
						t.Errorf("restore historical directory mode: %v", err)
					}
				}
			},
		},
		{
			name: "unrelated blocked reason",
			mutate: func(t *testing.T) func() {
				fixture.recoveryRecord.Error = "replacement worktree was modified"
				if err := writeRecoveryRecord(fixture.original+".kandev-recovery.json", fixture.recoveryRecord); err != nil {
					t.Fatalf("change blocked reason: %v", err)
				}
				return func() {
					if err := writeRecoveryRecord(fixture.original+".kandev-recovery.json", originalRecord); err != nil {
						t.Errorf("restore blocked record: %v", err)
					}
					fixture.recoveryRecord = originalRecord
				}
			},
		},
		{
			name: "conflicting relocation operation ID",
			mutate: func(t *testing.T) func() {
				fixture.relocation.OperationID = "223e4567-e89b-12d3-a456-426614174000"
				if err := writeManagedCloneRelocationRecord(fixture.original+".kandev-clone-relocation.json", fixture.relocation, false); err != nil {
					t.Fatalf("change relocation operation ID: %v", err)
				}
				return func() {
					fixture.relocation = originalRelocation
					if err := writeManagedCloneRelocationRecord(fixture.original+".kandev-clone-relocation.json", originalRelocation, false); err != nil {
						t.Errorf("restore relocation record: %v", err)
					}
				}
			},
		},
		{
			name: "edited replacement",
			mutate: func(t *testing.T) func() {
				path := filepath.Join(fixture.replacement, "edited-after-materialization")
				if err := os.WriteFile(path, []byte("edited"), 0o600); err != nil {
					t.Fatalf("edit replacement worktree: %v", err)
				}
				return func() { _ = os.Remove(path) }
			},
		},
		{
			name: "replacement head drift",
			mutate: func(t *testing.T) func() {
				runGit(t, fixture.replacement, "-c", "user.name=Permission Retry", "-c", "user.email=retry@example.invalid", "commit", "--allow-empty", "-m", "changed replacement head")
				return func() { runGit(t, fixture.replacement, "reset", "--hard", fixture.relocation.Head) }
			},
		},
		{
			name: "repeated retry provenance",
			mutate: func(t *testing.T) func() {
				fixture.recoveryRecord.ModeRetry = &recoveryModeRetry{
					Version: 1, PreviousSnapshot: fixture.oldSnapshot,
					PreviousError: permissionOnlyRecoveryFailure, PreviousUpdatedAt: originalRecord.UpdatedAt,
				}
				if err := writeRecoveryRecord(fixture.original+".kandev-recovery.json", fixture.recoveryRecord); err != nil {
					t.Fatalf("write repeated retry provenance: %v", err)
				}
				return func() {
					fixture.recoveryRecord = originalRecord
					if err := writeRecoveryRecord(fixture.original+".kandev-recovery.json", originalRecord); err != nil {
						t.Errorf("restore blocked recovery record: %v", err)
					}
				}
			},
		},
		{
			name: "old snapshot replaced by symbolic link",
			mutate: func(t *testing.T) func() {
				backup := fixture.oldSnapshot + ".retained"
				if err := os.Rename(fixture.oldSnapshot, backup); err != nil {
					t.Fatalf("move retained historical snapshot: %v", err)
				}
				if err := os.Symlink(fixture.replacement, fixture.oldSnapshot); err != nil {
					t.Fatalf("replace historical snapshot with symbolic link: %v", err)
				}
				return func() {
					_ = os.Remove(fixture.oldSnapshot)
					if err := os.Rename(backup, fixture.oldSnapshot); err != nil {
						t.Errorf("restore retained historical snapshot: %v", err)
					}
				}
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			restore := testCase.mutate(t)
			defer restore()
			recoveryBefore, err := os.ReadFile(fixture.original + ".kandev-recovery.json")
			if err != nil {
				t.Fatalf("read recovery record before refusal: %v", err)
			}
			relocationBefore, err := os.ReadFile(fixture.original + ".kandev-clone-relocation.json")
			if err != nil {
				t.Fatalf("read relocation record before refusal: %v", err)
			}
			admission, err := fixture.manager.AdmitRecovery(fixture.context, fixture.request)
			if err == nil {
				if admission != nil {
					_ = admission.Release(context.Background())
				}
				t.Fatal("AdmitRecovery accepted an unsafe permission-only retry")
			}
			if admission != nil {
				_ = admission.Release(context.Background())
				t.Fatal("unsafe retry returned an active admission")
			}
			if fixture.store.claim != nil {
				t.Fatal("unsafe retry retained a durable claim")
			}
			if fixture.store.worktrees[fixture.worktree.ID] == nil || fixture.store.worktrees[fixture.worktree.ID].Path != fixture.original {
				t.Fatal("unsafe retry changed the worktree inventory")
			}
			if _, err := os.Lstat(fixture.newSnapshot); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unsafe retry created a new snapshot: %v", err)
			}
			recoveryAfter, err := os.ReadFile(fixture.original + ".kandev-recovery.json")
			if err != nil || string(recoveryAfter) != string(recoveryBefore) {
				t.Fatalf("unsafe retry changed recovery record: err=%v", err)
			}
			relocationAfter, err := os.ReadFile(fixture.original + ".kandev-clone-relocation.json")
			if err != nil || string(relocationAfter) != string(relocationBefore) {
				t.Fatalf("unsafe retry changed relocation record: err=%v", err)
			}
		})
	}
}
