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

type managedCloneRelocationStore struct {
	*recoveryCASStore
	claim        *models.TaskEnvironmentRecoveryClaim
	claimRequest models.TaskEnvironmentRecoveryClaimRequest
}

func (s *managedCloneRelocationStore) AcquireTaskEnvironmentRecoveryClaim(
	_ context.Context,
	req models.TaskEnvironmentRecoveryClaimRequest,
) (*models.TaskEnvironmentRecoveryClaim, error) {
	s.claimRequest = req
	if s.claim != nil {
		return nil, errRecoveryOperationClaimed
	}
	s.claim = &models.TaskEnvironmentRecoveryClaim{
		TaskEnvironmentID:   req.TaskEnvironmentID,
		OwnerTaskID:         req.OwnerTaskID,
		OwnershipGeneration: req.OwnershipGeneration,
		SessionID:           req.SessionID,
		OperationID:         req.OperationID,
		ExecutorType:        req.ExecutorType,
	}
	return s.claim, nil
}

func (s *managedCloneRelocationStore) ReleaseTaskEnvironmentRecoveryClaim(
	_ context.Context,
	claim *models.TaskEnvironmentRecoveryClaim,
) error {
	if s.claim == nil || claim == nil || s.claim.OperationID != claim.OperationID {
		return errRecoveryOperationClaimed
	}
	s.claim = nil
	return nil
}

func (s *managedCloneRelocationStore) CompareAndSwapWorktreeWithRecoveryClaim(
	ctx context.Context,
	expected, replacement *Worktree,
	claim *models.TaskEnvironmentRecoveryClaim,
) (bool, error) {
	if s.claim == nil || claim == nil || s.claim.OperationID != claim.OperationID {
		return false, errRecoveryOperationClaimed
	}
	return s.CompareAndSwapWorktree(ctx, expected, replacement)
}

func TestManagerAdmitRecoveryRelocatesCleanManagedCloneWorktreeAndPreservesCommit(t *testing.T) {
	managedRoot := filepath.Join(t.TempDir(), "repos")
	sourceClone := filepath.Join(managedRoot, "_providers", "github", "github.com", "acme", "widget")
	destinationClone := filepath.Join(managedRoot, "workspaces", "workspace-1", "github", "acme", "widget")
	for _, path := range []string{sourceClone, destinationClone} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create clone parent: %v", err)
		}
	}

	seed := initGitRepoForWorktreeTest(t)
	runGit(t, seed, "clone", "--no-hardlinks", seed, sourceClone)
	runGit(t, seed, "clone", "--no-hardlinks", seed, destinationClone)
	for _, clone := range []string{sourceClone, destinationClone} {
		runGit(t, clone, "remote", "set-url", "origin", "https://github.com/acme/widget.git")
	}

	branch := "feature/task-123"
	runGit(t, sourceClone, "checkout", "-b", branch)
	if err := os.WriteFile(filepath.Join(sourceClone, "unpublished.txt"), []byte("local commit\n"), 0o644); err != nil {
		t.Fatalf("write unpushed file: %v", err)
	}
	runGit(t, sourceClone, "add", "unpublished.txt")
	runGit(t, sourceClone, "commit", "-m", "unpublished task commit")
	wantHead := strings.TrimSpace(runGit(t, sourceClone, "rev-parse", "HEAD"))
	runGit(t, sourceClone, "checkout", "main")

	config := newTestConfig(t)
	originalPath := filepath.Join(config.TasksBasePath, "task-1", "widget")
	if err := os.MkdirAll(filepath.Dir(originalPath), 0o755); err != nil {
		t.Fatalf("create task root: %v", err)
	}
	runGit(t, sourceClone, "worktree", "add", originalPath, branch)
	originalHead := strings.TrimSpace(runGit(t, originalPath, "rev-parse", "HEAD"))
	if originalHead != wantHead {
		t.Fatalf("original HEAD = %s, want %s", originalHead, wantHead)
	}

	wt := &Worktree{
		ID: "wt-legacy", TaskID: "task-1", TaskEnvironmentID: "env-1", RepositoryID: "repo-1",
		Path: originalPath, RepositoryPath: destinationClone, Branch: branch, BranchSlug: "branch-1", Status: StatusActive,
	}
	store := &managedCloneRelocationStore{recoveryCASStore: &recoveryCASStore{mockStore: newMockStore()}}
	store.worktrees[wt.ID] = wt
	mgr, err := NewManager(config, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	admission, err := mgr.AdmitRecovery(context.Background(), RecoveryAdmissionRequest{
		TaskID: "task-1", SessionID: "session-1", TaskEnvironmentID: "env-1", OwnerTaskID: "task-1",
		OwnershipGeneration: 1, ExecutorType: string(models.ExecutorTypeWorktree),
		Slots: []RecoverySlot{{
			WorktreeID: "wt-legacy", RepositoryID: "repo-1", BranchSlug: "branch-1", RepositoryPath: destinationClone,
			CloneRelocation: &ManagedCloneRelocationProof{
				ManagedRoot: managedRoot, ExpectedSourcePath: sourceClone, ExpectedDestinationPath: destinationClone,
				Identity: ManagedRepositoryIdentity{Provider: "github", Host: "github.com", Owner: "acme", Name: "widget"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("AdmitRecovery: %v", err)
	}
	if admission == nil {
		t.Fatal("AdmitRecovery() returned no admission for a clone mismatch")
	}
	if store.claimRequest.AllowCurrentSessionRuntime {
		t.Fatal("clean relocation unexpectedly allowed the current session runtime")
	}
	defer func() {
		if err := admission.Release(context.Background()); err != nil {
			t.Errorf("release recovery admission: %v", err)
		}
	}()

	if store.worktrees[wt.ID] != nil {
		t.Fatal("durable worktree still points at the original clone")
	}
	var replacement *Worktree
	for id, candidate := range store.worktrees {
		if id != wt.ID {
			replacement = candidate
			break
		}
	}
	if replacement == nil || replacement.Path == originalPath {
		t.Fatal("relocation did not publish a replacement worktree")
	}
	if got := strings.TrimSpace(runGit(t, replacement.Path, "rev-parse", "HEAD")); got != wantHead {
		t.Fatalf("replacement HEAD = %s, want exact source commit %s", got, wantHead)
	}
	if got := strings.TrimSpace(runGit(t, replacement.Path, "branch", "--show-current")); got != branch {
		t.Fatalf("replacement branch = %q, want %q", got, branch)
	}
	if got := strings.TrimSpace(runGit(t, originalPath, "rev-parse", "HEAD")); got != wantHead {
		t.Fatalf("original checkout HEAD = %s, want retained commit %s", got, wantHead)
	}
	if _, err := os.Stat(filepath.Join(originalPath, "unpublished.txt")); err != nil {
		t.Fatalf("original unpushed file was not retained: %v", err)
	}
	if got := strings.TrimSpace(runGit(t, replacement.Path, "rev-parse", "--path-format=absolute", "--git-common-dir")); filepath.Clean(got) != filepath.Clean(filepath.Join(destinationClone, ".git")) {
		t.Fatalf("replacement common dir = %q, want destination clone", got)
	}
	if err := admission.Release(context.Background()); err != nil {
		t.Fatalf("release initial admission: %v", err)
	}
	if err := os.RemoveAll(sourceClone); err != nil {
		t.Fatalf("remove obsolete source clone: %v", err)
	}
	resumeAdmission, err := mgr.AdmitRecovery(context.Background(), RecoveryAdmissionRequest{
		TaskID: "task-1", SessionID: "session-1", TaskEnvironmentID: "env-1", OwnerTaskID: "task-1",
		OwnershipGeneration: 1, ExecutorType: string(models.ExecutorTypeWorktree),
		Slots: []RecoverySlot{{
			WorktreeID: replacement.ID, RepositoryID: "repo-1", BranchSlug: "branch-1", RepositoryPath: destinationClone,
			CloneRelocation: &ManagedCloneRelocationProof{
				ManagedRoot: managedRoot, ExpectedSourcePath: sourceClone, ExpectedDestinationPath: destinationClone,
				Identity: ManagedRepositoryIdentity{Provider: "github", Host: "github.com", Owner: "acme", Name: "widget"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("re-admit replacement after source removal: %v", err)
	}
	if resumeAdmission != nil {
		_ = resumeAdmission.Release(context.Background())
		t.Fatal("re-admission requested relocation for a worktree already attached to the destination")
	}
}

func TestManagerAdmitRecoveryRefusesDirtyManagedCloneWorktree(t *testing.T) {
	managedRoot := filepath.Join(t.TempDir(), "repos")
	sourceClone := filepath.Join(managedRoot, "_providers", "github", "github.com", "acme", "widget")
	destinationClone := filepath.Join(managedRoot, "workspaces", "workspace-1", "github", "acme", "widget")
	for _, path := range []string{sourceClone, destinationClone} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create clone parent: %v", err)
		}
	}
	seed := initGitRepoForWorktreeTest(t)
	runGit(t, seed, "clone", "--no-hardlinks", seed, sourceClone)
	runGit(t, seed, "clone", "--no-hardlinks", seed, destinationClone)
	for _, clone := range []string{sourceClone, destinationClone} {
		runGit(t, clone, "remote", "set-url", "origin", "https://github.com/acme/widget.git")
	}
	branch := "feature/task-123"
	runGit(t, sourceClone, "checkout", "-b", branch)
	if err := os.WriteFile(filepath.Join(sourceClone, "unpublished.txt"), []byte("local commit\n"), 0o644); err != nil {
		t.Fatalf("write unpushed file: %v", err)
	}
	runGit(t, sourceClone, "add", "unpublished.txt")
	runGit(t, sourceClone, "commit", "-m", "unpublished task commit")
	runGit(t, sourceClone, "checkout", "main")

	config := newTestConfig(t)
	originalPath := filepath.Join(config.TasksBasePath, "task-1", "widget")
	if err := os.MkdirAll(filepath.Dir(originalPath), 0o755); err != nil {
		t.Fatalf("create task root: %v", err)
	}
	runGit(t, sourceClone, "worktree", "add", originalPath, branch)
	if err := os.WriteFile(filepath.Join(originalPath, "dirty-untracked.txt"), []byte("keep me\n"), 0o644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}
	wt := &Worktree{
		ID: "wt-dirty", TaskID: "task-1", TaskEnvironmentID: "env-1", RepositoryID: "repo-1",
		Path: originalPath, RepositoryPath: destinationClone, Branch: branch, BranchSlug: "branch-1", Status: StatusActive,
	}
	store := &managedCloneRelocationStore{recoveryCASStore: &recoveryCASStore{mockStore: newMockStore()}}
	store.worktrees[wt.ID] = wt
	mgr, err := NewManager(config, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	_, err = mgr.AdmitRecovery(context.Background(), RecoveryAdmissionRequest{
		TaskID: "task-1", SessionID: "session-1", TaskEnvironmentID: "env-1", OwnerTaskID: "task-1",
		OwnershipGeneration: 1, ExecutorType: string(models.ExecutorTypeWorktree),
		Slots: []RecoverySlot{{
			WorktreeID: wt.ID, RepositoryID: wt.RepositoryID, BranchSlug: wt.BranchSlug,
			RepositoryPath: destinationClone,
			CloneRelocation: &ManagedCloneRelocationProof{
				ManagedRoot: managedRoot, ExpectedSourcePath: sourceClone, ExpectedDestinationPath: destinationClone,
				Identity: ManagedRepositoryIdentity{Provider: "github", Host: "github.com", Owner: "acme", Name: "widget"},
			},
		}},
	})
	if err == nil {
		t.Fatal("AdmitRecovery() succeeded for a dirty worktree without explicit authorization")
	}
	var relocationRequired *ManagedCloneRelocationRequiredError
	if !errors.As(err, &relocationRequired) {
		t.Fatalf("AdmitRecovery() error = %T, want explicit relocation requirement", err)
	}
	if store.worktrees[wt.ID] == nil || store.worktrees[wt.ID].Path != originalPath {
		t.Fatal("dirty worktree inventory changed during automatic recovery refusal")
	}
	if got := strings.TrimSpace(runGit(t, originalPath, "rev-parse", "HEAD")); got != strings.TrimSpace(runGit(t, sourceClone, "rev-parse", "refs/heads/"+branch)) {
		t.Fatalf("original HEAD changed: got %s", got)
	}
	if got, err := os.ReadFile(filepath.Join(originalPath, "dirty-untracked.txt")); err != nil || string(got) != "keep me\n" {
		t.Fatalf("dirty file was changed or lost: %q, %v", got, err)
	}
	if store.claim != nil {
		t.Fatal("automatic refusal acquired a durable recovery claim")
	}
}

func TestManagerAdmitRecoveryRelocatesDirtyManagedCloneOnlyWithExplicitAuthorization(t *testing.T) {
	managedRoot := filepath.Join(t.TempDir(), "repos")
	sourceClone := filepath.Join(managedRoot, "acme", "widget")
	providerSourceClone := filepath.Join(managedRoot, "_providers", "github", "github.com", "acme", "widget")
	destinationClone := filepath.Join(managedRoot, "workspaces", "workspace-1", "github", "acme", "widget")
	for _, path := range []string{sourceClone, destinationClone} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create clone parent: %v", err)
		}
	}
	seed := initGitRepoForWorktreeTest(t)
	runGit(t, seed, "clone", "--no-hardlinks", seed, sourceClone)
	runGit(t, seed, "clone", "--no-hardlinks", seed, destinationClone)
	for _, clone := range []string{sourceClone, destinationClone} {
		runGit(t, clone, "remote", "set-url", "origin", "https://github.com/acme/widget.git")
	}
	branch := "feature/task-123"
	runGit(t, sourceClone, "checkout", "-b", branch)
	if err := os.WriteFile(filepath.Join(sourceClone, "committed.txt"), []byte("local commit\n"), 0o644); err != nil {
		t.Fatalf("write commit file: %v", err)
	}
	runGit(t, sourceClone, "add", "committed.txt")
	runGit(t, sourceClone, "commit", "-m", "unpublished task commit")
	runGit(t, sourceClone, "checkout", "main")

	config := newTestConfig(t)
	originalPath := filepath.Join(config.TasksBasePath, "task-dirty", "widget")
	if err := os.MkdirAll(filepath.Dir(originalPath), 0o755); err != nil {
		t.Fatalf("create task root: %v", err)
	}
	runGit(t, sourceClone, "worktree", "add", originalPath, branch)
	wantHead := strings.TrimSpace(runGit(t, originalPath, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(originalPath, "committed.txt"), []byte("modified after staging\n"), 0o644); err != nil {
		t.Fatalf("modify tracked file: %v", err)
	}
	runGit(t, originalPath, "add", "committed.txt")
	if err := os.WriteFile(filepath.Join(originalPath, "dirty-untracked.txt"), []byte("keep me\n"), 0o755); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}
	wt := &Worktree{
		ID: "wt-dirty-authorized", TaskID: "task-dirty", TaskEnvironmentID: "env-dirty", RepositoryID: "repo-dirty",
		Path: originalPath, RepositoryPath: destinationClone, Branch: branch, BranchSlug: "branch-dirty", Status: StatusActive,
	}
	store := &managedCloneRelocationStore{recoveryCASStore: &recoveryCASStore{mockStore: newMockStore()}}
	store.worktrees[wt.ID] = wt
	mgr, err := NewManager(config, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ctx := WithDirtyCloneRelocation(context.Background())
	admission, err := mgr.AdmitRecovery(ctx, RecoveryAdmissionRequest{
		TaskID: "task-dirty", SessionID: "session-dirty", TaskEnvironmentID: "env-dirty", OwnerTaskID: "task-dirty",
		OwnershipGeneration: 1, ExecutorType: string(models.ExecutorTypeWorktree),
		Slots: []RecoverySlot{{
			WorktreeID: wt.ID, RepositoryID: wt.RepositoryID, BranchSlug: wt.BranchSlug, RepositoryPath: destinationClone,
			CloneRelocation: &ManagedCloneRelocationProof{
				ManagedRoot: managedRoot, ExpectedSourcePath: providerSourceClone,
				LegacyOwnerNameSourcePath: sourceClone, ExpectedDestinationPath: destinationClone,
				Identity: ManagedRepositoryIdentity{Provider: "github", Host: "github.com", Owner: "acme", Name: "widget"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("AdmitRecovery: %v", err)
	}
	if admission == nil {
		t.Fatal("AdmitRecovery() returned no admission for explicit dirty relocation")
	}
	if !store.claimRequest.AllowCurrentSessionRuntime {
		t.Fatal("explicit dirty relocation did not admit its requesting session runtime")
	}
	defer func() {
		if err := admission.Release(context.Background()); err != nil {
			t.Errorf("release recovery admission: %v", err)
		}
	}()
	var replacement *Worktree
	for id, candidate := range store.worktrees {
		if id != wt.ID {
			replacement = candidate
			break
		}
	}
	if replacement.Path == originalPath {
		t.Fatal("worktree inventory still points at the original checkout")
	}
	if got := strings.TrimSpace(runGit(t, replacement.Path, "rev-parse", "HEAD")); got != wantHead {
		t.Fatalf("replacement HEAD = %s, want exact source commit %s", got, wantHead)
	}
	if got, err := os.ReadFile(filepath.Join(replacement.Path, "committed.txt")); err != nil || string(got) != "modified after staging\n" {
		t.Fatalf("modified tracked file = %q, %v", got, err)
	}
	if info, err := os.Stat(filepath.Join(replacement.Path, "dirty-untracked.txt")); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("untracked file mode = %v, %v", info, err)
	}
	if got := strings.TrimSpace(runGit(t, replacement.Path, "diff", "--cached", "--", "committed.txt")); got != "" {
		t.Fatalf("staging state unexpectedly transferred: %s", got)
	}
	if got := strings.TrimSpace(runGit(t, replacement.Path, "diff", "--", "committed.txt")); got == "" {
		t.Fatal("staged content was not retained as an unstaged modification")
	}
	recoveryRecord, err := readRecoveryRecord(originalPath + ".kandev-recovery.json")
	if err != nil {
		t.Fatalf("read recovery snapshot record: %v", err)
	}
	if _, err := os.Stat(recoveryRecord.Snapshot); err != nil {
		t.Fatalf("retained snapshot missing: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(originalPath, "dirty-untracked.txt")); err != nil || string(got) != "keep me\n" {
		t.Fatalf("original checkout was not retained: %q, %v", got, err)
	}
}

func TestParseManagedGitOrigin(t *testing.T) {
	tests := []struct {
		name, origin, provider, host, owner, repo string
		wantErr                                   bool
	}{
		{name: "https github", origin: "https://github.com/acme/widget.git", provider: "github", host: "github.com", owner: "acme", repo: "widget"},
		{name: "scp github", origin: "git@github.com:acme/widget.git", provider: "github", host: "github.com", owner: "acme", repo: "widget"},
		{name: "ssh nested gitlab", origin: "ssh://git@gitlab.example.test:2222/groups/team/widget.git", provider: "", host: "gitlab.example.test:2222", owner: "groups/team", repo: "widget"},
		{name: "embedded credentials", origin: "https://token@example.test/acme/widget.git", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider, host, owner, repo, err := parseManagedGitOrigin(test.origin)
			if (err != nil) != test.wantErr {
				t.Fatalf("parseManagedGitOrigin() err = %v, wantErr %t", err, test.wantErr)
			}
			if err == nil && (provider != test.provider || host != test.host || owner != test.owner || repo != test.repo) {
				t.Fatalf("parseManagedGitOrigin() = (%q, %q, %q, %q), want (%q, %q, %q, %q)", provider, host, owner, repo, test.provider, test.host, test.owner, test.repo)
			}
		})
	}
}
