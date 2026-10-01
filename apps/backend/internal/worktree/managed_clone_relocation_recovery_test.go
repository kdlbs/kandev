package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestAdmitRecoveryReconcilesPublishedRelocationClaimAfterRestart(t *testing.T) {
	managedRoot := filepath.Join(t.TempDir(), "repos")
	source := filepath.Join(managedRoot, "_providers", "github", "github.com", "acme", "widget")
	destination := filepath.Join(managedRoot, "workspaces", "workspace-1", "github", "acme", "widget")
	for _, path := range []string{source, destination} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	seed := initGitRepoForWorktreeTest(t)
	runGit(t, seed, "clone", "--no-hardlinks", seed, source)
	runGit(t, seed, "clone", "--no-hardlinks", seed, destination)
	configureManagedCloneRelocationGitIdentity(t, source)
	for _, clone := range []string{source, destination} {
		runGit(t, clone, "remote", "set-url", "origin", "https://github.com/acme/widget.git")
	}
	branch := "feature/relocation-restart"
	runGit(t, source, "checkout", "-b", branch)
	if err := os.WriteFile(filepath.Join(source, "unpublished.txt"), []byte("kept\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, source, "add", "unpublished.txt")
	runGit(t, source, "commit", "-m", "unpublished task commit")
	head := strings.TrimSpace(runGit(t, source, "rev-parse", "HEAD"))
	runGit(t, source, "checkout", "main")

	config := newTestConfig(t)
	original := filepath.Join(config.TasksBasePath, "task-restart", "widget")
	replacement := original + ".relocated-12345678"
	if err := os.MkdirAll(filepath.Dir(original), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, source, "worktree", "add", original, branch)
	runGit(t, destination, "fetch", "--no-tags", source, head)
	runGit(t, destination, "update-ref", "refs/heads/"+branch, head)
	runGit(t, destination, "worktree", "add", replacement, branch)

	const operationID = "123e4567-e89b-12d3-a456-426614174000"
	claim := &models.TaskEnvironmentRecoveryClaim{
		TaskEnvironmentID: "env-restart", OwnerTaskID: "task-restart", OwnershipGeneration: 1,
		SessionID: "session-restart", OperationID: operationID, ExecutorType: string(models.ExecutorTypeWorktree),
	}
	record := managedCloneRelocationRecord{
		OperationID: operationID, TaskID: "task-restart", EnvironmentID: claim.TaskEnvironmentID,
		WorktreeID: "wt-old", Original: original, OriginalWorkspacePath: original,
		Replacement: replacement, ReplacementID: "wt-new", SourcePath: source,
		SourceCommon: filepath.Join(source, ".git"), DestPath: destination,
		DestCommon: filepath.Join(destination, ".git"), Branch: branch, Head: head,
		State: managedCloneRelocationStateMaterialized,
	}
	if err := writeManagedCloneRelocationRecord(replacement+".kandev-clone-relocation.json", record, true); err != nil {
		t.Fatal(err)
	}
	wt := &Worktree{
		ID: record.ReplacementID, TaskID: record.TaskID, TaskEnvironmentID: record.EnvironmentID,
		RepositoryID: "repo-restart", Path: replacement, RepositoryPath: destination,
		Branch: branch, BranchSlug: "branch-restart", Status: StatusActive,
	}
	store := &managedCloneRelocationStore{recoveryCASStore: &recoveryCASStore{mockStore: newMockStore()}, claim: claim}
	store.worktrees[wt.ID] = wt
	manager, err := NewManager(config, store, newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	admission, err := manager.AdmitRecovery(context.Background(), RecoveryAdmissionRequest{
		TaskID: wt.TaskID, SessionID: claim.SessionID, TaskEnvironmentID: claim.TaskEnvironmentID,
		OwnerTaskID: claim.OwnerTaskID, OwnershipGeneration: claim.OwnershipGeneration,
		ExecutorType: claim.ExecutorType, Slots: []RecoverySlot{{
			WorktreeID: wt.ID, RepositoryID: wt.RepositoryID, BranchSlug: wt.BranchSlug,
			RepositoryPath: destination, CloneRelocation: &ManagedCloneRelocationProof{
				ManagedRoot: managedRoot, ExpectedSourcePath: source, ExpectedDestinationPath: destination,
				Identity: ManagedRepositoryIdentity{Provider: "github", Host: "github.com", Owner: "acme", Name: "widget"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("AdmitRecovery after published replacement: %v", err)
	}
	if admission != nil {
		_ = admission.Release(context.Background())
		t.Fatal("published replacement unexpectedly requested new recovery admission")
	}
	if store.claim != nil {
		t.Fatal("restart reconciliation left the durable recovery claim active")
	}
	if _, err := os.Lstat(original); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old checkout remains beside the replacement: %v", err)
	}
	completed, err := readManagedCloneRelocationRecord(replacement + ".kandev-clone-relocation.json")
	if err != nil || completed.State != string(RecoveryStateComplete) {
		t.Fatalf("replacement relocation journal = %+v, %v", completed, err)
	}
	if completed.Original == original {
		t.Fatal("restart did not move the retained original outside the task root")
	}
}

func TestAdmitRecoveryResumesPreparedRelocationAfterRestart(t *testing.T) {
	managedRoot := filepath.Join(t.TempDir(), "repos")
	source := filepath.Join(managedRoot, "_providers", "github", "github.com", "acme", "widget")
	destination := filepath.Join(managedRoot, "workspaces", "workspace-1", "github", "acme", "widget")
	for _, path := range []string{source, destination} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	seed := initGitRepoForWorktreeTest(t)
	runGit(t, seed, "clone", "--no-hardlinks", seed, source)
	runGit(t, seed, "clone", "--no-hardlinks", seed, destination)
	configureManagedCloneRelocationGitIdentity(t, source)
	for _, clone := range []string{source, destination} {
		runGit(t, clone, "remote", "set-url", "origin", "https://github.com/acme/widget.git")
	}
	branch := "feature/relocation-restart-prepared"
	runGit(t, source, "checkout", "-b", branch)
	if err := os.WriteFile(filepath.Join(source, "unpushed.txt"), []byte("keep after restart\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, source, "add", "unpushed.txt")
	runGit(t, source, "commit", "-m", "unpushed commit")
	head := strings.TrimSpace(runGit(t, source, "rev-parse", "HEAD"))
	runGit(t, source, "checkout", "main")

	config := newTestConfig(t)
	original := filepath.Join(config.TasksBasePath, "task-restart-prepared", "widget")
	if err := os.MkdirAll(filepath.Dir(original), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, source, "worktree", "add", original, branch)
	const operationID = "223e4567-e89b-12d3-a456-426614174000"
	replacement := original + ".relocated-" + operationID[:8]
	claim := &models.TaskEnvironmentRecoveryClaim{
		TaskEnvironmentID: "env-restart-prepared", OwnerTaskID: "task-restart-prepared",
		OwnershipGeneration: 1, SessionID: "session-restart-prepared", OperationID: operationID,
		ExecutorType: string(models.ExecutorTypeWorktree),
	}
	worktree := &Worktree{
		ID: "wt-restart-prepared", TaskID: claim.OwnerTaskID, TaskEnvironmentID: claim.TaskEnvironmentID,
		RepositoryID: "repo-restart-prepared", Path: original, RepositoryPath: destination,
		Branch: branch, BranchSlug: "branch-restart-prepared", Status: StatusActive,
	}
	record := managedCloneRelocationRecord{
		OperationID: operationID, TaskID: claim.OwnerTaskID, EnvironmentID: claim.TaskEnvironmentID,
		WorktreeID: worktree.ID, Original: original, OriginalWorkspacePath: original,
		Replacement: replacement, ReplacementID: "wt-restart-prepared-new", SourcePath: source,
		SourceCommon: filepath.Join(source, ".git"), DestPath: destination,
		DestCommon: filepath.Join(destination, ".git"), Branch: branch, Head: head,
		State: managedCloneRelocationStatePrepared,
	}
	if err := writeManagedCloneRelocationRecord(original+".kandev-clone-relocation.json", record, true); err != nil {
		t.Fatal(err)
	}
	store := &managedCloneRelocationStore{
		recoveryCASStore: &recoveryCASStore{mockStore: newMockStore()}, claim: claim,
	}
	store.worktrees[worktree.ID] = worktree
	manager, err := NewManager(config, store, newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	admission, err := manager.AdmitRecovery(context.Background(), RecoveryAdmissionRequest{
		TaskID: claim.OwnerTaskID, SessionID: claim.SessionID, TaskEnvironmentID: claim.TaskEnvironmentID,
		OwnerTaskID: claim.OwnerTaskID, OwnershipGeneration: claim.OwnershipGeneration,
		ExecutorType: claim.ExecutorType, Slots: []RecoverySlot{{
			WorktreeID: worktree.ID, RepositoryID: worktree.RepositoryID, BranchSlug: worktree.BranchSlug,
			RepositoryPath: destination, CloneRelocation: &ManagedCloneRelocationProof{
				ManagedRoot: managedRoot, ExpectedSourcePath: source, ExpectedDestinationPath: destination,
				Identity: ManagedRepositoryIdentity{Provider: "github", Host: "github.com", Owner: "acme", Name: "widget"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("AdmitRecovery after prepared relocation restart: %v", err)
	}
	if admission == nil {
		t.Fatal("prepared relocation restart returned no admission")
	}
	defer func() { _ = admission.Release(context.Background()) }()
	if store.worktrees[record.ReplacementID] == nil || store.worktrees[record.ReplacementID].Path != replacement {
		t.Fatal("prepared relocation restart did not publish its deterministic replacement")
	}
	if got := strings.TrimSpace(runGit(t, replacement, "rev-parse", "HEAD")); got != head {
		t.Fatalf("restarted replacement HEAD = %s, want exact source commit %s", got, head)
	}
	completed, err := readManagedCloneRelocationRecord(original + ".kandev-clone-relocation.json")
	if err != nil || completed.State != string(RecoveryStateComplete) {
		t.Fatalf("prepared relocation journal = %+v, %v", completed, err)
	}
}
