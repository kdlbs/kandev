package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryartifact"
)

func TestManagedCloneRecoveryLegacyJournalAndModeRetryContinuity(t *testing.T) {
	config := newTestConfig(t)
	original := filepath.Join(config.TasksBasePath, "task-legacy-artifacts", "widget")
	if err := os.MkdirAll(filepath.Dir(original), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(original, 0o700); err != nil {
		t.Fatal(err)
	}
	const operationID = "323e4567-e89b-12d3-a456-426614174000"
	worktreeID := uuid.NewString()
	replacementID := uuid.NewString()
	replacement := original + ".relocated-" + operationID[:8]
	wt := &Worktree{
		ID: worktreeID, TaskID: "task-legacy-artifacts", TaskEnvironmentID: "env-legacy-artifacts",
		RepositoryID: "repo-legacy-artifacts", Path: original, Status: StatusActive,
	}
	claim := &models.TaskEnvironmentRecoveryClaim{
		TaskEnvironmentID: wt.TaskEnvironmentID, OwnerTaskID: wt.TaskID, OwnershipGeneration: 1,
		SessionID: "session-legacy-artifacts", OperationID: operationID,
		ExecutorType: string(models.ExecutorTypeWorktree),
	}
	// The missing layout_version is the persisted v1 shape.
	legacyJSON := `{"operation_id":"` + operationID + `","task_id":"` + wt.TaskID +
		`","environment_id":"` + wt.TaskEnvironmentID + `","worktree_id":"` + wt.ID +
		`","original":"` + original + `","original_workspace_path":"` + original +
		`","replacement":"` + replacement + `","replacement_id":"` + replacementID +
		`","source_path":"/source","source_common_dir":"/source/.git","destination_path":"/destination","destination_common_dir":"/destination/.git","branch":"feature/legacy","head":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","state":"materialized"}`
	if err := os.WriteFile(original+".kandev-clone-relocation.json", []byte(legacyJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	previousSnapshot := original + ".kandev-recovery-" + operationID
	if err := os.Mkdir(previousSnapshot, 0o700); err != nil {
		t.Fatal(err)
	}
	retrySnapshot := permissionRetrySnapshotPath(original, operationID)
	recovery := recoveryRecord{
		OperationID: operationID, TaskID: wt.TaskID, WorktreeID: wt.ID,
		Original: original, Snapshot: retrySnapshot, Manifest: "", State: RecoveryStateBlocked,
		Error: permissionOnlyRecoveryFailure,
		ModeRetry: &recoveryModeRetry{
			Version: 1, PreviousSnapshot: previousSnapshot, PreviousError: permissionOnlyRecoveryFailure,
			PreviousUpdatedAt: time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC),
			SourceManifest:    strings.Repeat("a", 64), SourceIdentityManifest: strings.Repeat("b", 64),
		},
	}
	if err := writeRecoveryRecord(original+".kandev-recovery.json", recovery); err != nil {
		t.Fatal(err)
	}
	if !validRecoveryModeRetry(recovery, original) {
		t.Fatal("legacy mode-retry journal no longer validates at its adjacent snapshot paths")
	}
	store := &managedCloneRelocationStore{
		recoveryCASStore: &recoveryCASStore{mockStore: newMockStore()}, claim: claim,
	}
	store.worktrees[wt.ID] = wt
	manager, err := NewManager(config, store, newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	paths, err := manager.prepareManagedCloneArtifactLayout(context.Background(), wt, claim, true)
	if err != nil {
		t.Fatalf("prepare legacy artifact layout: %v", err)
	}
	if paths.LayoutVersion != 1 || paths.RelocationRecord != original+".kandev-clone-relocation.json" ||
		paths.RecoveryRecord != original+".kandev-recovery.json" || paths.Snapshot != retrySnapshot {
		t.Fatalf("legacy artifact paths changed: %+v", paths)
	}
	if len(store.artifacts) != 0 {
		t.Fatalf("legacy paths were registered before authoritative relocation proof: %+v", store.artifacts)
	}
}

func TestManagedCloneRecoveryArtifactRegistryRejectsSubstitution(t *testing.T) {
	const operationID = "423e4567-e89b-12d3-a456-426614174000"
	bucket := t.TempDir()
	recordPath := filepath.Join(bucket, "records", "relocation.json")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o700); err != nil {
		t.Fatal(err)
	}
	record := managedCloneRelocationRecord{
		LayoutVersion: 2, OperationID: operationID, TaskID: "task-artifact-proof",
		EnvironmentID: "env-artifact-proof", WorktreeID: "worktree-artifact-proof",
		Original: "/tasks/task/widget", OriginalWorkspacePath: "/tasks/task/widget",
		Replacement: "/tasks/task/widget.relocated-423e4567", ReplacementID: "replacement-artifact-proof",
		State: managedCloneRelocationStatePrepared,
	}
	if err := writeManagedCloneRelocationRecord(recordPath, record, true); err != nil {
		t.Fatal(err)
	}
	item := recoveryartifact.Registered{Registration: recoveryartifact.Registration{
		TaskEnvironmentID: record.EnvironmentID, OwnerTaskID: record.TaskID,
		OwnershipGeneration: 4, OperationID: operationID, WorktreeID: record.WorktreeID,
		RepositoryID: "repository-artifact-proof", OriginalPath: record.OriginalWorkspacePath,
		ReplacementID: record.ReplacementID, ReplacementPath: record.Replacement, LayoutVersion: 2,
		Provenance:    recoveryartifact.ProvenanceV2Operation,
		ArtifactPaths: []string{recordPath},
	}}
	if got, err := registeredRelocationOperationID(item); err != nil || got != operationID {
		t.Fatalf("registered relocation operation = %q, %v", got, err)
	}
	item.ReplacementPath = filepath.Join(bucket, "substituted")
	if _, err := registeredRelocationOperationID(item); err == nil {
		t.Fatal("registry proof accepted a substituted replacement path")
	}
}

func TestManagedCloneRecoveryArtifactRegistryRejectsSymlinkedRecordDirectory(t *testing.T) {
	const operationID = "623e4567-e89b-12d3-a456-426614174000"
	root := t.TempDir()
	bucket := filepath.Join(root, ".kandev-recovery", strings.Repeat("a", 64))
	recordDirectory := filepath.Join(bucket, "records")
	if err := os.MkdirAll(bucket, 0o700); err != nil {
		t.Fatal(err)
	}
	external := t.TempDir()
	record := managedCloneRelocationRecord{
		LayoutVersion: 2, OperationID: operationID, TaskID: "task-symlink-proof",
		EnvironmentID: "env-symlink-proof", WorktreeID: "worktree-symlink-proof",
		Original: "/tasks/task/widget", OriginalWorkspacePath: "/tasks/task/widget",
		Replacement: "/tasks/task/widget.relocated-623e4567", ReplacementID: "replacement-symlink-proof",
		State: managedCloneRelocationStateMaterialized,
	}
	if err := writeManagedCloneRelocationRecord(filepath.Join(external, "relocation.json"), record, true); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, recordDirectory); err != nil {
		t.Skipf("directory symlinks are unavailable: %v", err)
	}
	item := recoveryartifact.Registered{Registration: recoveryartifact.Registration{
		TaskEnvironmentID: record.EnvironmentID, OwnerTaskID: record.TaskID,
		OwnershipGeneration: 1, SessionID: "session-symlink-proof", OperationID: operationID,
		ExecutorType: "worktree", WorktreeID: record.WorktreeID, RepositoryID: "repo-symlink-proof",
		OriginalPath: record.OriginalWorkspacePath, ReplacementID: record.ReplacementID,
		ReplacementPath: record.Replacement, LayoutVersion: 2, Provenance: recoveryartifact.ProvenanceV2Operation,
		ArtifactPaths: []string{filepath.Join(recordDirectory, "relocation.json")},
	}}
	if _, err := registeredRelocationOperationID(item); err == nil {
		t.Fatal("registry proof followed a substituted private records directory")
	}
}

func TestRegisterVerifiedLegacyRelocationDoesNotTrustUnboundSnapshotPaths(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "task", "widget")
	replacement := original + ".relocated-723e4567"
	userSnapshot := filepath.Join(root, "user-data", "important-project")
	userPreviousSnapshot := filepath.Join(root, "other-user-data", "keep-visible")
	for _, path := range []string{filepath.Dir(original), replacement, userSnapshot, userPreviousSnapshot} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	const operationID = "723e4567-e89b-12d3-a456-426614174000"
	worktreeID := "worktree-legacy-unbound"
	replacementID := "replacement-legacy-unbound"
	destination := filepath.Join(root, "managed", "workspace", "widget")
	if err := os.MkdirAll(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	wt := &Worktree{
		ID: replacementID, TaskID: "task-legacy-unbound", TaskEnvironmentID: "env-legacy-unbound",
		RepositoryID: "repo-legacy-unbound", Path: replacement, RepositoryPath: destination,
	}
	claim := &models.TaskEnvironmentRecoveryClaim{
		TaskEnvironmentID: wt.TaskEnvironmentID, OwnerTaskID: wt.TaskID,
		OwnershipGeneration: 3, SessionID: "session-legacy-unbound", OperationID: operationID,
		ExecutorType: string(models.ExecutorTypeWorktree),
	}
	record := managedCloneRelocationRecord{
		LayoutVersion: 1, OperationID: operationID, TaskID: wt.TaskID,
		EnvironmentID: wt.TaskEnvironmentID, WorktreeID: worktreeID,
		Original: original, OriginalWorkspacePath: original, Replacement: replacement,
		ReplacementID: replacementID, State: managedCloneRelocationStateMaterialized,
		SourcePath:   filepath.Join(root, "providers", "widget"),
		SourceCommon: filepath.Join(root, "providers", "widget", ".git"),
		DestPath:     destination, DestCommon: filepath.Join(destination, ".git"),
		Branch: "feature/legacy", Head: strings.Repeat("a", 40),
	}
	if err := writeManagedCloneRelocationRecord(replacement+".kandev-clone-relocation.json", record, true); err != nil {
		t.Fatal(err)
	}
	if err := writeRecoveryRecord(original+".kandev-recovery.json", recoveryRecord{
		LayoutVersion: 1, OperationID: operationID, TaskID: wt.TaskID,
		WorktreeID: worktreeID, Original: original, Snapshot: userSnapshot,
		Replacement: replacement, Manifest: strings.Repeat("a", 64), State: RecoveryStateRematerializing,
		ModeRetry: &recoveryModeRetry{
			Version: 1, PreviousSnapshot: userPreviousSnapshot, PreviousError: permissionOnlyRecoveryFailure,
			PreviousUpdatedAt: time.Now().UTC(), SourceManifest: strings.Repeat("a", 64),
			SourceIdentityManifest: strings.Repeat("b", 64),
		},
	}); err != nil {
		t.Fatal(err)
	}
	store := &managedCloneRelocationStore{
		recoveryCASStore: &recoveryCASStore{mockStore: newMockStore()},
		claim:            claim,
	}
	store.worktrees[wt.ID] = wt
	manager, err := NewManager(newTestConfig(t), store, newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	req := &RecoveryAdmissionRequest{
		TaskID: wt.TaskID, TaskEnvironmentID: wt.TaskEnvironmentID, OwnerTaskID: wt.TaskID,
		OwnershipGeneration: claim.OwnershipGeneration, SessionID: claim.SessionID, ExecutorType: claim.ExecutorType,
	}
	if err := manager.registerVerifiedLegacyRelocation(context.Background(), req, wt, claim, record); err != nil {
		t.Fatalf("register verified legacy relocation: %v", err)
	}
	for _, registered := range store.artifacts {
		for _, path := range registered.ArtifactPaths {
			if path == userSnapshot || path == userPreviousSnapshot {
				t.Fatalf("unbound user directory was registered as an artifact: %q", path)
			}
		}
	}
}

type recoveryArtifactUnavailableStore struct {
	Store
	recoveryClaimStore
	recoveryClaimReader
	CompareAndSwapWorktreeWithRecoveryClaimStore
}

func TestManagedCloneRecoveryFailsBeforeTransferWhenRegistryIsUnavailable(t *testing.T) {
	config := newTestConfig(t)
	original := filepath.Join(config.TasksBasePath, "task-no-artifact-registry", "widget")
	if err := os.MkdirAll(filepath.Dir(original), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(original, 0o700); err != nil {
		t.Fatal(err)
	}
	wt := &Worktree{
		ID: "worktree-no-artifact-registry", TaskID: "task-no-artifact-registry",
		TaskEnvironmentID: "env-no-artifact-registry", RepositoryID: "repo-no-artifact-registry",
		Path: original, Status: StatusActive,
	}
	claim := &models.TaskEnvironmentRecoveryClaim{
		TaskEnvironmentID: wt.TaskEnvironmentID, OwnerTaskID: wt.TaskID, OwnershipGeneration: 1,
		SessionID: "session-no-artifact-registry", OperationID: "523e4567-e89b-12d3-a456-426614174000",
		ExecutorType: string(models.ExecutorTypeWorktree),
	}
	base := &managedCloneRelocationStore{
		recoveryCASStore: &recoveryCASStore{mockStore: newMockStore()}, claim: claim,
	}
	base.worktrees[wt.ID] = wt
	store := recoveryArtifactUnavailableStore{
		Store: base, recoveryClaimStore: base, recoveryClaimReader: base,
		CompareAndSwapWorktreeWithRecoveryClaimStore: base,
	}
	manager, err := NewManager(config, store, newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.prepareManagedCloneArtifactLayout(context.Background(), wt, claim, true); err == nil {
		t.Fatal("managed-clone recovery started without an artifact registry")
	}
	record := managedCloneRelocationRecord{
		OperationID: claim.OperationID, TaskID: wt.TaskID, EnvironmentID: wt.TaskEnvironmentID,
		WorktreeID: wt.ID, OriginalWorkspacePath: wt.Path,
	}
	archive, err := manager.managedCloneRelocationArchivePath(&record)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Dir(archive)); !os.IsNotExist(err) {
		t.Fatalf("private recovery bucket exists after registry refusal: %v", err)
	}
	for _, suffix := range []string{".kandev-clone-relocation.json", ".kandev-clone-relocation.claim"} {
		if _, err := os.Lstat(original + suffix); !os.IsNotExist(err) {
			t.Fatalf("legacy sidecar exists after registry refusal: %s: %v", suffix, err)
		}
	}
}
