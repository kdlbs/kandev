//go:build unix

package worktree

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	permissionRetryTestOperationID = "123e4567-e89b-12d3-a456-426614174000"
	permissionRetryTestFailure     = "recovery snapshot does not match original checkout"
)

type permissionRetryFixture struct {
	manager        *Manager
	store          *managedCloneRelocationStore
	request        RecoveryAdmissionRequest
	context        context.Context
	worktree       *Worktree
	original       string
	replacement    string
	sourceClone    string
	destination    string
	oldSnapshot    string
	newSnapshot    string
	recoveryRecord recoveryRecord
	relocation     managedCloneRelocationRecord
}

func TestPermissionOnlyBlockedRelocationRetry(t *testing.T) {
	fixture := newPermissionRetryFixture(t, "success")
	admission, err := fixture.manager.AdmitRecovery(fixture.context, fixture.request)
	if err != nil {
		t.Fatalf("AdmitRecovery: %v", err)
	}
	if admission == nil {
		t.Fatal("explicit blocked-snapshot retry returned no admission")
	}
	defer func() {
		if err := admission.Release(context.Background()); err != nil {
			t.Errorf("release admission: %v", err)
		}
	}()

	published := fixture.store.worktrees[fixture.relocation.ReplacementID]
	if published == nil || published.Path != fixture.replacement {
		t.Fatalf("published worktree = %+v, want existing replacement %q", published, fixture.replacement)
	}
	recovery, err := readRecoveryRecord(fixture.original + ".kandev-recovery.json")
	if err != nil {
		t.Fatalf("read completed recovery record: %v", err)
	}
	if recovery.OperationID != permissionRetryTestOperationID || recovery.State != RecoveryStateComplete {
		t.Fatalf("completed recovery identity/state = %s/%s", recovery.OperationID, recovery.State)
	}
	if recovery.Snapshot != fixture.newSnapshot {
		t.Fatalf("recovery snapshot = %q, want %q", recovery.Snapshot, fixture.newSnapshot)
	}
	recoveryData, err := os.ReadFile(fixture.original + ".kandev-recovery.json")
	if err != nil {
		t.Fatalf("read retry provenance: %v", err)
	}
	var persisted map[string]json.RawMessage
	if err := json.Unmarshal(recoveryData, &persisted); err != nil {
		t.Fatalf("decode retry provenance: %v", err)
	}
	var retryProvenance struct {
		Version                int       `json:"version"`
		PreviousSnapshot       string    `json:"previous_snapshot"`
		PreviousError          string    `json:"previous_error"`
		PreviousUpdatedAt      time.Time `json:"previous_updated_at"`
		SourceManifest         string    `json:"source_manifest"`
		SourceIdentityManifest string    `json:"source_identity_manifest"`
	}
	if err := json.Unmarshal(persisted["mode_retry"], &retryProvenance); err != nil || retryProvenance.PreviousSnapshot != fixture.oldSnapshot {
		t.Fatalf("mode retry provenance = %s, err=%v", persisted["mode_retry"], err)
	}
	if retryProvenance.Version != 1 || retryProvenance.PreviousError != permissionRetryTestFailure ||
		retryProvenance.PreviousUpdatedAt != fixture.recoveryRecord.UpdatedAt ||
		!validRecoveryDigest(retryProvenance.SourceManifest) ||
		!validRecoveryDigest(retryProvenance.SourceIdentityManifest) {
		t.Fatalf("mode retry provenance fields = %+v", retryProvenance)
	}
	if _, err := os.Stat(fixture.oldSnapshot); err != nil {
		t.Fatalf("historical failed snapshot was not retained: %v", err)
	}
	assertRecoveryMode(t, filepath.Join(fixture.replacement, "group-writable.txt"), 0o775)
	fileUID, _ := recoveryTestIdentity(t, statRecoveryEntry(t, filepath.Join(recovery.Original, "setuid-file")))
	_, directoryGID := recoveryTestIdentity(t, statRecoveryEntry(t, filepath.Join(recovery.Original, "setgid-directory")))
	assertRecoveryModeAndIdentity(t, filepath.Join(fixture.replacement, "setuid-file"), 0o4755, fileUID, 0, true, false)
	assertRecoveryModeAndIdentity(t, filepath.Join(fixture.replacement, "setgid-directory"), 0o2750, 0, directoryGID, false, true)
	assertRecoveryModeAndIdentity(t, filepath.Join(fixture.replacement, "sticky-directory"), 0o1777, 0, 0, false, false)
	if target, err := os.Readlink(filepath.Join(fixture.replacement, "dirty-link")); err != nil || target != "group-writable.txt" {
		t.Fatalf("restored symbolic link target = %q, %v", target, err)
	}
	if got, err := os.ReadFile(filepath.Join(recovery.Original, "group-writable.txt")); err != nil || string(got) != "permission retry content\n" {
		t.Fatalf("retained original file = %q, %v", got, err)
	}
	if mode := statRecoveryEntry(t, filepath.Join(recovery.Original, "group-writable.txt")).Mode().Perm(); mode != 0o775 {
		t.Fatalf("retained original mode = %04o, want 0775", mode)
	}
}

func TestPermissionOnlyBlockedRelocationPreservesUnaffectedSibling(t *testing.T) {
	fixture := newPermissionRetryFixture(t, "unaffected-sibling")
	sibling := addUnaffectedPermissionRetrySibling(t, &fixture)
	marker := filepath.Join(sibling.Path, "uncommitted-note.txt")
	if err := os.WriteFile(marker, []byte("keep sibling state\n"), 0o600); err != nil {
		t.Fatalf("write sibling marker: %v", err)
	}
	beforeStatus := runGit(t, sibling.Path, "status", "--porcelain=v1", "--untracked-files=all")
	beforeHead := runGit(t, sibling.Path, "rev-parse", "HEAD")
	fixture.request.Slots = append(fixture.request.Slots, RecoverySlot{
		WorktreeID: sibling.ID, RepositoryID: sibling.RepositoryID, BranchSlug: sibling.BranchSlug,
		RepositoryPath: fixture.destination,
	})
	admission, err := fixture.manager.AdmitRecovery(fixture.context, fixture.request)
	if err != nil {
		t.Fatalf("AdmitRecovery with healthy sibling: %v", err)
	}
	defer func() { _ = admission.Release(context.Background()) }()
	assertUnaffectedPermissionRetrySibling(t, fixture, sibling, marker, beforeHead, beforeStatus)
}

func addUnaffectedPermissionRetrySibling(t *testing.T, fixture *permissionRetryFixture) *Worktree {
	t.Helper()
	branch := "feature/permission-retry-unaffected"
	runGit(t, fixture.destination, "branch", branch, fixture.relocation.Head)
	path := filepath.Join(filepath.Dir(fixture.original), "unaffected-sibling")
	runGit(t, fixture.destination, "worktree", "add", path, branch)
	sibling := &Worktree{
		ID: "wt-permission-retry-unaffected", TaskID: fixture.worktree.TaskID,
		TaskEnvironmentID: fixture.worktree.TaskEnvironmentID, RepositoryID: "repo-permission-retry-unaffected",
		Path: path, RepositoryPath: fixture.destination, Branch: branch,
		BranchSlug: "branch-permission-retry-unaffected", Status: StatusActive,
	}
	fixture.store.worktrees[sibling.ID] = sibling
	return sibling
}

func assertUnaffectedPermissionRetrySibling(
	t *testing.T,
	fixture permissionRetryFixture,
	sibling *Worktree,
	marker, beforeHead, beforeStatus string,
) {
	t.Helper()
	if persisted := fixture.store.worktrees[sibling.ID]; persisted == nil || persisted.Path != sibling.Path {
		t.Fatalf("unaffected sibling inventory changed: %+v", persisted)
	}
	if got := runGit(t, sibling.Path, "rev-parse", "HEAD"); got != beforeHead {
		t.Fatalf("unaffected sibling HEAD = %q, want %q", got, beforeHead)
	}
	if got := runGit(t, sibling.Path, "status", "--porcelain=v1", "--untracked-files=all"); got != beforeStatus {
		t.Fatalf("unaffected sibling status = %q, want %q", got, beforeStatus)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep sibling state\n" {
		t.Fatalf("unaffected sibling marker = %q, %v", data, err)
	}
	if _, err := os.Lstat(sibling.Path + ".kandev-recovery.json"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unaffected sibling gained a recovery record: %v", err)
	}
}

func TestValidRecoveryModeRetryRequiresRetainedSnapshot(t *testing.T) {
	original := filepath.Join(t.TempDir(), "checkout")
	oldSnapshot := original + ".kandev-recovery-" + permissionRetryTestOperationID
	if err := os.Mkdir(original, 0o700); err != nil {
		t.Fatalf("create original checkout: %v", err)
	}
	if err := os.Mkdir(oldSnapshot, 0o700); err != nil {
		t.Fatalf("create retained snapshot: %v", err)
	}
	record := recoveryRecord{
		OperationID: permissionRetryTestOperationID,
		Snapshot:    permissionRetrySnapshotPath(original, permissionRetryTestOperationID),
		ModeRetry: &recoveryModeRetry{
			Version: 1, PreviousSnapshot: oldSnapshot,
			PreviousError: permissionOnlyRecoveryFailure, PreviousUpdatedAt: time.Now().UTC(),
			SourceManifest: strings.Repeat("a", 64), SourceIdentityManifest: strings.Repeat("b", 64),
		},
	}
	if !validRecoveryModeRetry(record, original) {
		t.Fatal("valid retained snapshot provenance was rejected")
	}
	if err := os.Remove(oldSnapshot); err != nil {
		t.Fatalf("remove retained snapshot: %v", err)
	}
	if validRecoveryModeRetry(record, original) {
		t.Fatal("missing retained snapshot provenance was accepted")
	}
	if err := os.Mkdir(oldSnapshot, 0o700); err != nil {
		t.Fatalf("recreate retained snapshot: %v", err)
	}
	if err := os.Rename(oldSnapshot, oldSnapshot+".real"); err != nil {
		t.Fatalf("move retained snapshot: %v", err)
	}
	if err := os.Symlink(oldSnapshot+".real", oldSnapshot); err != nil {
		t.Fatalf("replace retained snapshot with symlink: %v", err)
	}
	if validRecoveryModeRetry(record, original) {
		t.Fatal("symlinked retained snapshot provenance was accepted")
	}
}

func TestPermissionOnlyBlockedRelocationRestart(t *testing.T) {
	for _, state := range []RecoveryState{RecoveryStateSnapshotting, RecoveryStateRematerializing} {
		t.Run(string(state), func(t *testing.T) {
			fixture := newPermissionRetryFixture(t, "restart-"+string(state))
			record := seedInterruptedPermissionRetry(t, fixture, state)
			if err := writeRecoveryRecord(fixture.original+".kandev-recovery.json", record); err != nil {
				t.Fatalf("seed interrupted retry record: %v", err)
			}
			admission, err := fixture.manager.AdmitRecovery(fixture.context, fixture.request)
			if err != nil {
				t.Fatalf("AdmitRecovery after %s: %v", state, err)
			}
			if admission == nil {
				t.Fatal("interrupted retry returned no admission")
			}
			defer func() { _ = admission.Release(context.Background()) }()
			completed, err := readRecoveryRecord(fixture.original + ".kandev-recovery.json")
			if err != nil {
				t.Fatalf("read resumed recovery: %v", err)
			}
			if completed.OperationID != record.OperationID || completed.Snapshot != fixture.newSnapshot || completed.State != RecoveryStateComplete {
				t.Fatalf("resumed recovery = %+v", completed)
			}
			if err := verifyRecoveryRequiredIdentityTrees(completed.Original, fixture.newSnapshot, fixture.replacement); err != nil {
				t.Fatalf("resumed recovery set-ID identity: %v", err)
			}
			if _, err := os.Stat(fixture.oldSnapshot); err != nil {
				t.Fatalf("historical snapshot was not retained: %v", err)
			}
			assertRecoveryMode(t, filepath.Join(fixture.replacement, "group-writable.txt"), 0o775)
		})
	}
}

func seedInterruptedPermissionRetry(t *testing.T, fixture permissionRetryFixture, state RecoveryState) recoveryRecord {
	t.Helper()
	sourceProof, err := provePermissionOnlySnapshot(context.Background(), fixture.original, fixture.oldSnapshot)
	if err != nil {
		t.Fatalf("prove interrupted retry source: %v", err)
	}
	record := fixture.recoveryRecord
	record.Snapshot = fixture.newSnapshot
	record.ModeRetry = &recoveryModeRetry{
		Version: 1, PreviousSnapshot: fixture.oldSnapshot,
		PreviousError: fixture.recoveryRecord.Error, PreviousUpdatedAt: fixture.recoveryRecord.UpdatedAt,
		SourceManifest: sourceProof.manifest, SourceIdentityManifest: sourceProof.identityManifest,
	}
	record.State = state
	switch state {
	case RecoveryStateSnapshotting:
		if err := os.Mkdir(fixture.newSnapshot, 0o700); err != nil {
			t.Fatalf("create partial retry snapshot: %v", err)
		}
		if err := os.WriteFile(filepath.Join(fixture.newSnapshot, "partial"), []byte("incomplete"), 0o600); err != nil {
			t.Fatalf("write partial retry snapshot: %v", err)
		}
	case RecoveryStateRematerializing:
		if err := snapshotCheckout(fixture.original, fixture.newSnapshot); err != nil {
			t.Fatalf("create complete retry snapshot: %v", err)
		}
		manifest, err := checkoutManifest(fixture.newSnapshot)
		if err != nil {
			t.Fatalf("manifest retry snapshot: %v", err)
		}
		record.Manifest = manifest
	}
	return record
}

func TestPermissionOnlyBlockedRelocationMixedInventory(t *testing.T) {
	fixture := newPermissionRetryFixture(t, "mixed")
	secondWorktree, secondSnapshot := addPermissionRetrySibling(t, &fixture)
	if err := os.WriteFile(filepath.Join(secondSnapshot, "group-writable.txt"), []byte("sibling content drift\n"), 0o600); err != nil {
		t.Fatalf("mutate failed sibling snapshot: %v", err)
	}
	proof := *fixture.request.Slots[0].CloneRelocation
	secondSlot := RecoverySlot{
		WorktreeID: secondWorktree.ID, RepositoryID: secondWorktree.RepositoryID,
		BranchSlug: secondWorktree.BranchSlug, RepositoryPath: fixture.destination,
		CloneRelocation: &proof,
	}
	fixture.request.Slots = append(fixture.request.Slots, secondSlot)
	if admission, err := fixture.manager.AdmitRecovery(fixture.context, fixture.request); err == nil {
		if admission != nil {
			_ = admission.Release(context.Background())
		}
		t.Fatal("mixed inventory accepted a failed permission-only sibling")
	}
	if fixture.store.claim != nil {
		t.Fatal("mixed-inventory refusal retained the durable claim")
	}
	for _, item := range []struct {
		worktree *Worktree
		original string
		snapshot string
	}{
		{fixture.worktree, fixture.original, fixture.oldSnapshot},
		{secondWorktree, secondWorktree.Path, secondWorktree.Path + ".kandev-recovery-" + permissionRetryTestOperationID},
	} {
		if persisted := fixture.store.worktrees[item.worktree.ID]; persisted == nil || persisted.Path != item.original {
			t.Fatalf("mixed inventory changed worktree %s: %+v", item.worktree.ID, persisted)
		}
		recovery, err := readRecoveryRecord(item.original + ".kandev-recovery.json")
		if err != nil || recovery.State != RecoveryStateBlocked || recovery.Snapshot != item.snapshot {
			t.Fatalf("mixed inventory changed blocked recovery: %+v, %v", recovery, err)
		}
		if _, err := os.Lstat(permissionRetrySnapshotPath(item.original, permissionRetryTestOperationID)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("mixed inventory created retry snapshot for %s: %v", item.worktree.ID, err)
		}
	}
}

func addPermissionRetrySibling(t *testing.T, fixture *permissionRetryFixture) (*Worktree, string) {
	t.Helper()
	branch := "feature/permission-retry-sibling"
	runGit(t, fixture.sourceClone, "branch", branch, fixture.relocation.Head)
	original := filepath.Join(filepath.Dir(fixture.original), "sibling")
	runGit(t, fixture.sourceClone, "worktree", "add", original, branch)
	writeRecoveryUnixModeFixture(t, filepath.Join(original, "group-writable.txt"), 0o775)
	if err := os.WriteFile(filepath.Join(original, "group-writable.txt"), []byte("sibling permission retry\n"), 0o600); err != nil {
		t.Fatalf("write sibling dirty content: %v", err)
	}
	if err := chmodRecoveryUnixMode(filepath.Join(original, "group-writable.txt"), 0o775); err != nil {
		t.Fatalf("set sibling dirty mode: %v", err)
	}
	runGit(t, fixture.destination, "fetch", "--no-tags", fixture.sourceClone, fixture.relocation.Head)
	runGit(t, fixture.destination, "update-ref", "refs/heads/"+branch, fixture.relocation.Head)
	replacement := original + ".relocated-" + permissionRetryTestOperationID[:8]
	runGit(t, fixture.destination, "worktree", "add", replacement, branch)
	worktree := &Worktree{
		ID: "wt-permission-retry-sibling", TaskID: fixture.worktree.TaskID,
		TaskEnvironmentID: fixture.worktree.TaskEnvironmentID, RepositoryID: "repo-permission-retry-sibling",
		Path: original, RepositoryPath: fixture.destination, Branch: branch,
		BranchSlug: "branch-permission-retry-sibling", Status: StatusActive,
	}
	fixture.store.worktrees[worktree.ID] = worktree
	snapshot := original + ".kandev-recovery-" + permissionRetryTestOperationID
	if err := snapshotCheckout(original, snapshot); err != nil {
		t.Fatalf("snapshot sibling checkout: %v", err)
	}
	if err := chmodRecoveryUnixMode(filepath.Join(snapshot, "group-writable.txt"), 0o755); err != nil {
		t.Fatalf("simulate sibling permission loss: %v", err)
	}
	recovery := recoveryRecord{
		OperationID: permissionRetryTestOperationID, TaskID: worktree.TaskID, WorktreeID: worktree.ID,
		Original: original, Snapshot: snapshot, State: RecoveryStateBlocked,
		Error: permissionRetryTestFailure, UpdatedAt: fixture.recoveryRecord.UpdatedAt,
	}
	if err := createRecoveryRecord(original+".kandev-recovery.json", recovery); err != nil {
		t.Fatalf("seed sibling recovery record: %v", err)
	}
	relocation := managedCloneRelocationRecord{
		OperationID: permissionRetryTestOperationID, TaskID: worktree.TaskID,
		EnvironmentID: worktree.TaskEnvironmentID, WorktreeID: worktree.ID,
		Original: original, OriginalWorkspacePath: original, Replacement: replacement,
		ReplacementID: "wt-permission-retry-sibling-replacement", SourcePath: fixture.sourceClone,
		SourceCommon: filepath.Join(fixture.sourceClone, ".git"), DestPath: fixture.destination,
		DestCommon: filepath.Join(fixture.destination, ".git"), Branch: branch, Head: fixture.relocation.Head,
		State: managedCloneRelocationStateMaterialized,
	}
	if err := writeManagedCloneRelocationRecord(original+".kandev-clone-relocation.json", relocation, true); err != nil {
		t.Fatalf("seed sibling relocation record: %v", err)
	}
	return worktree, snapshot
}

func newPermissionRetryFixture(t *testing.T, suffix string) permissionRetryFixture {
	t.Helper()
	managedRoot := filepath.Join(t.TempDir(), "repos")
	sourceClone := filepath.Join(managedRoot, "_providers", "github", "github.com", "acme", "widget")
	destination := filepath.Join(managedRoot, "workspaces", "workspace-1", "github", "acme", "widget")
	for _, path := range []string{sourceClone, destination} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create clone parent: %v", err)
		}
	}
	seed := initGitRepoForWorktreeTest(t)
	runGit(t, seed, "clone", "--no-hardlinks", seed, sourceClone)
	runGit(t, seed, "clone", "--no-hardlinks", seed, destination)
	configureManagedCloneRelocationGitIdentity(t, sourceClone)
	for _, clone := range []string{sourceClone, destination} {
		runGit(t, clone, "remote", "set-url", "origin", "https://github.com/acme/widget.git")
	}
	branch := "feature/permission-retry-" + suffix
	runGit(t, sourceClone, "checkout", "-b", branch)
	if err := os.WriteFile(filepath.Join(sourceClone, "commit.txt"), []byte("unpublished commit\n"), 0o644); err != nil {
		t.Fatalf("write branch commit: %v", err)
	}
	runGit(t, sourceClone, "add", "commit.txt")
	runGit(t, sourceClone, "commit", "-m", "permission retry fixture")
	head := strings.TrimSpace(runGit(t, sourceClone, "rev-parse", "HEAD"))
	runGit(t, sourceClone, "checkout", "main")
	config := newTestConfig(t)
	original := filepath.Join(config.TasksBasePath, "task-permission-retry-"+suffix, "widget")
	if err := os.MkdirAll(filepath.Dir(original), 0o755); err != nil {
		t.Fatalf("create original parent: %v", err)
	}
	runGit(t, sourceClone, "worktree", "add", original, branch)
	writeRecoveryUnixModeFixture(t, filepath.Join(original, "group-writable.txt"), 0o775)
	if err := os.WriteFile(filepath.Join(original, "group-writable.txt"), []byte("permission retry content\n"), 0o775); err != nil {
		t.Fatalf("write dirty content: %v", err)
	}
	if err := chmodRecoveryUnixMode(filepath.Join(original, "group-writable.txt"), 0o775); err != nil {
		t.Fatalf("set dirty file mode: %v", err)
	}
	writeRecoveryUnixModeFixture(t, filepath.Join(original, "setuid-file"), 0o4755)
	if err := os.Symlink("group-writable.txt", filepath.Join(original, "dirty-link")); err != nil {
		t.Fatalf("create dirty symbolic link: %v", err)
	}
	for name, mode := range map[string]uint32{"setgid-directory": 0o2750, "sticky-directory": 0o1777} {
		path := filepath.Join(original, name)
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if err := chmodRecoveryUnixMode(path, mode); err != nil {
			t.Fatalf("set %s mode: %v", name, err)
		}
	}
	if statRecoveryEntry(t, filepath.Join(original, "setuid-file")).Mode()&os.ModeSetuid == 0 {
		t.Skip("host filesystem does not support setuid test entries")
	}
	if statRecoveryEntry(t, filepath.Join(original, "setgid-directory")).Mode()&os.ModeSetgid == 0 {
		t.Skip("host filesystem does not support setgid test entries")
	}
	if statRecoveryEntry(t, filepath.Join(original, "sticky-directory")).Mode()&os.ModeSticky == 0 {
		t.Skip("host filesystem does not support sticky test entries")
	}
	runGit(t, destination, "fetch", "--no-tags", sourceClone, head)
	runGit(t, destination, "update-ref", "refs/heads/"+branch, head)
	replacement := original + ".relocated-" + permissionRetryTestOperationID[:8]
	runGit(t, destination, "worktree", "add", replacement, branch)
	worktree := &Worktree{
		ID: "wt-permission-retry-" + suffix, TaskID: "task-permission-retry-" + suffix,
		TaskEnvironmentID: "env-permission-retry-" + suffix, RepositoryID: "repo-permission-retry-" + suffix,
		Path: original, RepositoryPath: destination, Branch: branch, BranchSlug: "branch-permission-retry-" + suffix,
		Status: StatusActive,
	}
	store := &managedCloneRelocationStore{recoveryCASStore: &recoveryCASStore{mockStore: newMockStore()}}
	store.worktrees[worktree.ID] = worktree
	manager, err := NewManager(config, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	oldSnapshot := original + ".kandev-recovery-" + permissionRetryTestOperationID
	if err := snapshotCheckout(original, oldSnapshot); err != nil {
		t.Fatalf("create historical snapshot: %v", err)
	}
	for path, mode := range map[string]uint32{
		filepath.Join(oldSnapshot, "group-writable.txt"): 0o755,
		filepath.Join(oldSnapshot, "setuid-file"):        0o755,
		filepath.Join(oldSnapshot, "setgid-directory"):   0o750,
		filepath.Join(oldSnapshot, "sticky-directory"):   0o777,
	} {
		if err := chmodRecoveryUnixMode(path, mode); err != nil {
			t.Fatalf("simulate historical permission loss: %v", err)
		}
	}
	now := time.Date(2026, 10, 4, 15, 0, 0, 0, time.UTC)
	recovery := recoveryRecord{
		OperationID: permissionRetryTestOperationID, TaskID: worktree.TaskID, WorktreeID: worktree.ID,
		Original: original, Snapshot: oldSnapshot, State: RecoveryStateBlocked,
		Error: permissionRetryTestFailure, UpdatedAt: now,
	}
	if err := createRecoveryRecord(original+".kandev-recovery.json", recovery); err != nil {
		t.Fatalf("seed historical recovery record: %v", err)
	}
	relocation := managedCloneRelocationRecord{
		OperationID: permissionRetryTestOperationID, TaskID: worktree.TaskID, EnvironmentID: worktree.TaskEnvironmentID,
		WorktreeID: worktree.ID, Original: original, OriginalWorkspacePath: original,
		Replacement: replacement, ReplacementID: "wt-permission-retry-replacement-" + suffix,
		SourcePath: sourceClone, SourceCommon: filepath.Join(sourceClone, ".git"),
		DestPath: destination, DestCommon: filepath.Join(destination, ".git"), Branch: branch, Head: head,
		State: managedCloneRelocationStateMaterialized,
	}
	if err := writeManagedCloneRelocationRecord(original+".kandev-clone-relocation.json", relocation, true); err != nil {
		t.Fatalf("seed materialized relocation record: %v", err)
	}
	request := RecoveryAdmissionRequest{
		TaskID: worktree.TaskID, SessionID: "session-permission-retry-" + suffix,
		TaskEnvironmentID: worktree.TaskEnvironmentID, OwnerTaskID: worktree.TaskID,
		OwnershipGeneration: 1, ExecutorType: string(models.ExecutorTypeWorktree),
		Slots: []RecoverySlot{{
			WorktreeID: worktree.ID, RepositoryID: worktree.RepositoryID, BranchSlug: worktree.BranchSlug,
			RepositoryPath: destination, CloneRelocation: &ManagedCloneRelocationProof{
				ManagedRoot: managedRoot, ExpectedSourcePath: sourceClone, ExpectedDestinationPath: destination,
				Identity: ManagedRepositoryIdentity{Provider: "github", Host: "github.com", Owner: "acme", Name: "widget"},
			},
		}},
	}
	ctx := WithDirtyCloneRelocation(context.Background())
	ctx = WithManagedCloneRelocationAuthorization(ctx, func(context.Context) error { return nil })
	return permissionRetryFixture{
		manager: manager, store: store, request: request, context: ctx, worktree: worktree,
		original: original, replacement: replacement, sourceClone: sourceClone, destination: destination,
		oldSnapshot: oldSnapshot, newSnapshot: original + ".kandev-recovery-" + permissionRetryTestOperationID + "-modes-v1",
		recoveryRecord: recovery, relocation: relocation,
	}
}
