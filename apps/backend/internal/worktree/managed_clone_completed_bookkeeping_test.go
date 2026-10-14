package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-MANAGED-CLONE-RELOCATION-001.3, AC-TASKS-MANAGED-CLONE-RELOCATION-001.6, AC-TASKS-MANAGED-CLONE-RELOCATION-001.7
func TestCompletedRelocationRejectsInvalidInventory(t *testing.T) {
	t.Run("invalid completed sibling", func(t *testing.T) {
		fixture := newCompletedRelocationFixture(t, "github", "github.com")
		addCompletedRelocationSibling(t, &fixture, "widget-two")
		firstRecord := fixture.recordPath
		firstBefore, err := os.ReadFile(firstRecord)
		if err != nil {
			t.Fatalf("read first journal: %v", err)
		}
		second := &fixture.request.Slots[1]
		second.CloneRelocation.Identity.Name = "another-repository"
		manager, err := NewManager(fixture.config, fixture.store, newTestLogger())
		if err != nil {
			t.Fatalf("reconstruct manager: %v", err)
		}
		if admission, err := manager.AdmitRecovery(context.Background(), fixture.request); err == nil {
			if admission != nil {
				_ = admission.Release(context.Background())
			}
			t.Fatal("admission accepted an invalid completed sibling")
		}
		firstAfter, err := os.ReadFile(firstRecord)
		if err != nil || string(firstAfter) != string(firstBefore) {
			t.Fatalf("valid sibling journal changed before inventory refusal: read err %v", err)
		}
	})

	t.Run("materialized record keeps exact head proof", func(t *testing.T) {
		fixture := newCompletedRelocationFixture(t, "gitlab", "gitlab.com")
		record, err := readManagedCloneRelocationRecord(fixture.recordPath)
		if err != nil {
			t.Fatalf("read relocation journal: %v", err)
		}
		record.State = managedCloneRelocationStateMaterialized
		if err := writeManagedCloneRelocationRecord(fixture.recordPath, record, false); err != nil {
			t.Fatalf("write unfinished journal fixture: %v", err)
		}
		before, err := os.ReadFile(fixture.recordPath)
		if err != nil {
			t.Fatalf("read unfinished journal fixture: %v", err)
		}
		if err := os.WriteFile(filepath.Join(fixture.replacement.Path, "later.txt"), []byte("later commit\n"), 0o644); err != nil {
			t.Fatalf("write later file: %v", err)
		}
		runGit(t, fixture.replacement.Path, "add", "later.txt")
		runGit(t, fixture.replacement.Path, "commit", "-m", "later commit")
		manager, err := NewManager(fixture.config, fixture.store, newTestLogger())
		if err != nil {
			t.Fatalf("reconstruct manager: %v", err)
		}
		if admission, err := manager.AdmitRecovery(context.Background(), fixture.request); err == nil {
			if admission != nil {
				_ = admission.Release(context.Background())
			}
			t.Fatal("admission accepted a changed materialized replacement")
		}
		after, err := os.ReadFile(fixture.recordPath)
		if err != nil || string(after) != string(before) {
			t.Fatalf("failed unfinished reconciliation changed its journal: read err %v", err)
		}
	})
}

// @covers AC-TASKS-MANAGED-CLONE-RELOCATION-001.3, AC-TASKS-MANAGED-CLONE-RELOCATION-002.4
func TestCompletedRelocationClaimBookkeeping(t *testing.T) {
	t.Run("matching claim is released", func(t *testing.T) {
		fixture := newCompletedRelocationFixture(t, "github", "github.com")
		record, err := readManagedCloneRelocationRecord(fixture.recordPath)
		if err != nil {
			t.Fatalf("read relocation journal: %v", err)
		}
		fixture.store.claim = &models.TaskEnvironmentRecoveryClaim{
			TaskEnvironmentID: fixture.request.TaskEnvironmentID, OwnerTaskID: fixture.request.OwnerTaskID,
			OwnershipGeneration: fixture.request.OwnershipGeneration, SessionID: fixture.request.SessionID,
			OperationID: record.OperationID, ExecutorType: fixture.request.ExecutorType,
		}
		before, err := os.ReadFile(fixture.recordPath)
		if err != nil {
			t.Fatalf("read relocation journal: %v", err)
		}
		manager, err := NewManager(fixture.config, fixture.store, newTestLogger())
		if err != nil {
			t.Fatalf("reconstruct manager: %v", err)
		}
		if admission, err := manager.AdmitRecovery(context.Background(), fixture.request); err != nil || admission != nil {
			if admission != nil {
				_ = admission.Release(context.Background())
			}
			t.Fatalf("AdmitRecovery with matching leftover claim = (%v, %v), want (nil, nil)", admission, err)
		}
		if fixture.store.claim != nil {
			t.Fatal("matching completed relocation claim remains active")
		}
		after, err := os.ReadFile(fixture.recordPath)
		if err != nil || string(after) != string(before) {
			t.Fatalf("matching-claim reconciliation rewrote relocation history: read err %v", err)
		}
	})

	t.Run("unrelated claim is preserved", func(t *testing.T) {
		fixture := newCompletedRelocationFixture(t, "gitlab", "gitlab.com")
		fixture.store.claim = &models.TaskEnvironmentRecoveryClaim{
			TaskEnvironmentID: fixture.request.TaskEnvironmentID, OwnerTaskID: "another-task",
			OwnershipGeneration: fixture.request.OwnershipGeneration, SessionID: "another-session",
			OperationID: "223e4567-e89b-12d3-a456-426614174000", ExecutorType: fixture.request.ExecutorType,
		}
		before, err := os.ReadFile(fixture.recordPath)
		if err != nil {
			t.Fatalf("read relocation journal: %v", err)
		}
		manager, err := NewManager(fixture.config, fixture.store, newTestLogger())
		if err != nil {
			t.Fatalf("reconstruct manager: %v", err)
		}
		if admission, err := manager.AdmitRecovery(context.Background(), fixture.request); err == nil {
			if admission != nil {
				_ = admission.Release(context.Background())
			}
			t.Fatal("admission accepted an unrelated recovery claim")
		}
		if fixture.store.claim == nil || fixture.store.claim.OwnerTaskID != "another-task" {
			t.Fatal("admission released or changed the unrelated recovery claim")
		}
		after, err := os.ReadFile(fixture.recordPath)
		if err != nil || string(after) != string(before) {
			t.Fatalf("claim conflict rewrote relocation history: read err %v", err)
		}
	})

	t.Run("incomplete companion is reconciled", func(t *testing.T) {
		fixture := newCompletedRelocationFixture(t, "github", "github.com")
		record, err := readManagedCloneRelocationRecord(fixture.recordPath)
		if err != nil {
			t.Fatalf("read relocation journal: %v", err)
		}
		before, err := os.ReadFile(fixture.recordPath)
		if err != nil {
			t.Fatalf("read relocation journal: %v", err)
		}
		companion := recoveryRecord{
			OperationID: record.OperationID, TaskID: record.TaskID, WorktreeID: record.WorktreeID,
			Original: record.OriginalWorkspacePath, Snapshot: "snapshot", Replacement: record.Replacement,
			State: RecoveryStateRematerializing,
		}
		primaryPath := filepath.Join(filepath.Dir(fixture.recordPath), managedCloneRecoveryRecordFilename)
		if err := writeRecoveryRecord(primaryPath, companion); err != nil {
			t.Fatalf("write interrupted companion fixture: %v", err)
		}
		manager, err := NewManager(fixture.config, fixture.store, newTestLogger())
		if err != nil {
			t.Fatalf("reconstruct manager: %v", err)
		}
		if admission, err := manager.AdmitRecovery(context.Background(), fixture.request); err != nil || admission != nil {
			if admission != nil {
				_ = admission.Release(context.Background())
			}
			t.Fatalf("AdmitRecovery with incomplete companion = (%v, %v), want (nil, nil)", admission, err)
		}
		for _, path := range []string{primaryPath, record.Original + ".kandev-recovery.json"} {
			got, readErr := readRecoveryRecord(path)
			if readErr != nil || got.State != RecoveryStateComplete || got.Original != record.Original || got.Replacement != record.Replacement {
				t.Fatalf("reconciled companion at %q = %+v, %v", path, got, readErr)
			}
		}
		after, err := os.ReadFile(fixture.recordPath)
		if err != nil || string(after) != string(before) {
			t.Fatalf("companion reconciliation rewrote completed relocation history: read err %v", err)
		}
	})
}

func addCompletedRelocationSibling(t *testing.T, fixture *completedRelocationFixture, repositoryName string) {
	t.Helper()
	proof := fixture.request.Slots[0].CloneRelocation
	source := filepath.Join(fixture.managedRoot, "_providers", proof.Identity.Provider, proof.Identity.Host, proof.Identity.Owner, repositoryName)
	destination := filepath.Join(fixture.managedRoot, "workspaces", "workspace-1", proof.Identity.Provider, proof.Identity.Owner, repositoryName)
	for _, path := range []string{source, destination} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create sibling clone parent: %v", err)
		}
	}
	seed := initGitRepoForWorktreeTest(t)
	runGit(t, seed, "clone", "--no-hardlinks", seed, source)
	runGit(t, seed, "clone", "--no-hardlinks", seed, destination)
	origin := "https://" + proof.Identity.Host + "/" + proof.Identity.Owner + "/" + repositoryName + ".git"
	for _, clone := range []string{source, destination} {
		configureManagedCloneRelocationGitIdentity(t, clone)
		runGit(t, clone, "remote", "set-url", "origin", origin)
	}
	branch := "feature/" + strings.ReplaceAll(repositoryName, "_", "-")
	runGit(t, source, "checkout", "-b", branch)
	if err := os.WriteFile(filepath.Join(source, "task-change.txt"), []byte("sibling task commit\n"), 0o644); err != nil {
		t.Fatalf("write sibling task file: %v", err)
	}
	runGit(t, source, "add", "task-change.txt")
	runGit(t, source, "commit", "-m", "sibling task commit")
	runGit(t, source, "checkout", "main")
	original := filepath.Join(fixture.config.TasksBasePath, fixture.request.TaskID, repositoryName)
	if err := os.MkdirAll(filepath.Dir(original), 0o755); err != nil {
		t.Fatalf("create sibling task root: %v", err)
	}
	runGit(t, source, "worktree", "add", original, branch)
	worktree := &Worktree{
		ID: "wt-sibling-" + repositoryName, TaskID: fixture.request.OwnerTaskID,
		TaskEnvironmentID: fixture.request.TaskEnvironmentID, RepositoryID: "repo-sibling-" + repositoryName,
		Path: original, RepositoryPath: destination, Branch: branch,
		BranchSlug: "branch-sibling-" + repositoryName, Status: StatusActive,
	}
	fixture.store.worktrees[worktree.ID] = worktree
	manager, err := NewManager(fixture.config, fixture.store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager for sibling: %v", err)
	}
	request := RecoveryAdmissionRequest{
		TaskID: fixture.request.TaskID, SessionID: fixture.request.SessionID,
		TaskEnvironmentID: fixture.request.TaskEnvironmentID, OwnerTaskID: fixture.request.OwnerTaskID,
		OwnershipGeneration: fixture.request.OwnershipGeneration, ExecutorType: fixture.request.ExecutorType,
		Slots: []RecoverySlot{{
			WorktreeID: worktree.ID, RepositoryID: worktree.RepositoryID, BranchSlug: worktree.BranchSlug,
			RepositoryPath: destination,
			CloneRelocation: &ManagedCloneRelocationProof{
				ManagedRoot: fixture.managedRoot, ExpectedSourcePath: source, ExpectedDestinationPath: destination,
				Identity: ManagedRepositoryIdentity{Provider: proof.Identity.Provider, Host: proof.Identity.Host, Owner: proof.Identity.Owner, Name: repositoryName},
			},
		}},
	}
	admission, err := manager.AdmitRecovery(context.Background(), request)
	if err != nil || admission == nil {
		t.Fatalf("relocate sibling worktree: admission=%v err=%v", admission, err)
	}
	if err := admission.Release(context.Background()); err != nil {
		t.Fatalf("release sibling relocation admission: %v", err)
	}
	var replacement *Worktree
	for _, candidate := range fixture.store.worktrees {
		if candidate.ID != worktree.ID && candidate.RepositoryID == worktree.RepositoryID {
			replacement = candidate
			break
		}
	}
	if replacement == nil {
		t.Fatal("sibling relocation did not publish a replacement")
	}
	request.Slots[0].WorktreeID = replacement.ID
	request.Slots[0].RepositoryID = replacement.RepositoryID
	request.Slots[0].BranchSlug = replacement.BranchSlug
	fixture.request.Slots = append(fixture.request.Slots, request.Slots[0])
	if err := os.RemoveAll(source); err != nil {
		t.Fatalf("remove sibling source clone: %v", err)
	}
}
