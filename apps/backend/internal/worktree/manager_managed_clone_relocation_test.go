package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryartifact"
	"github.com/kandev/kandev/internal/task/recoveryoperation"
)

type managedCloneRelocationStore struct {
	*recoveryCASStore
	claim        *models.TaskEnvironmentRecoveryClaim
	claimRequest models.TaskEnvironmentRecoveryClaimRequest
	acquireErr   error
	stalePublish bool
	artifacts    map[string]recoveryartifact.Registered
}

func (s *managedCloneRelocationStore) RegisterTaskEnvironmentRecoveryArtifacts(
	_ context.Context,
	registration recoveryartifact.Registration,
) error {
	if s.claim == nil || s.claim.TaskEnvironmentID != registration.TaskEnvironmentID ||
		s.claim.OwnerTaskID != registration.OwnerTaskID || s.claim.OwnershipGeneration != registration.OwnershipGeneration ||
		s.claim.SessionID != registration.SessionID || s.claim.OperationID != registration.OperationID ||
		s.claim.ExecutorType != registration.ExecutorType {
		return recoveryartifact.ErrIdentityMismatch
	}
	current := s.worktrees[registration.WorktreeID]
	if current == nil {
		current = s.worktrees[registration.ReplacementID]
	}
	if current == nil || current.TaskEnvironmentID != registration.TaskEnvironmentID ||
		current.RepositoryID != registration.RepositoryID ||
		(current.ID != registration.WorktreeID && current.ID != registration.ReplacementID) ||
		(current.Path != registration.OriginalPath && current.Path != registration.ReplacementPath) {
		return recoveryartifact.ErrIdentityMismatch
	}
	if s.artifacts == nil {
		s.artifacts = make(map[string]recoveryartifact.Registered)
	}
	key := registration.TaskEnvironmentID + "\x00" + registration.OperationID + "\x00" + registration.WorktreeID
	item := s.artifacts[key]
	if item.OperationID != "" {
		if item.OwnerTaskID != registration.OwnerTaskID || item.OwnershipGeneration != registration.OwnershipGeneration ||
			item.ReplacementID != registration.ReplacementID || item.ReplacementPath != registration.ReplacementPath {
			return recoveryartifact.ErrIdentityMismatch
		}
	}
	registration.ArtifactIdentities = make(map[string]string, len(registration.ArtifactPaths))
	for _, path := range registration.ArtifactPaths {
		if identity, ok := recoveryartifact.FilesystemIdentity(path); ok {
			registration.ArtifactIdentities[path] = identity
		}
	}
	item.Registration = registration
	paths := append(append([]string(nil), item.ArtifactPaths...), registration.ArtifactPaths...)
	item.ArtifactPaths = uniqueSortedPaths(paths)
	s.artifacts[key] = item
	return nil
}

func (s *managedCloneRelocationStore) ListTaskEnvironmentRecoveryArtifacts(
	_ context.Context,
	environmentID string,
) ([]recoveryartifact.Registered, error) {
	var result []recoveryartifact.Registered
	for _, item := range s.artifacts {
		if item.TaskEnvironmentID != environmentID {
			continue
		}
		current := s.worktrees[item.WorktreeID]
		if current == nil {
			current = s.worktrees[item.ReplacementID]
		}
		if current == nil || current.Path != item.OriginalPath && current.Path != item.ReplacementPath {
			continue
		}
		result = append(result, item)
	}
	return result, nil
}

func uniqueSortedPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	return result
}

func (s *managedCloneRelocationStore) AcquireTaskEnvironmentRecoveryClaim(
	_ context.Context,
	req models.TaskEnvironmentRecoveryClaimRequest,
) (*models.TaskEnvironmentRecoveryClaim, error) {
	s.claimRequest = req
	if s.acquireErr != nil {
		return nil, s.acquireErr
	}
	if s.claim != nil {
		if s.claim.TaskEnvironmentID == req.TaskEnvironmentID && s.claim.OwnerTaskID == req.OwnerTaskID &&
			s.claim.OwnershipGeneration == req.OwnershipGeneration && s.claim.SessionID == req.SessionID &&
			s.claim.OperationID == req.OperationID && s.claim.ExecutorType == req.ExecutorType {
			return s.claim, nil
		}
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

func (s *managedCloneRelocationStore) GetTaskEnvironmentRecoveryClaim(
	_ context.Context,
	environmentID string,
) (*models.TaskEnvironmentRecoveryClaim, error) {
	if s.claim == nil || s.claim.TaskEnvironmentID != environmentID {
		return nil, nil
	}
	claim := *s.claim
	return &claim, nil
}

func (s *managedCloneRelocationStore) CompareAndSwapWorktreeWithRecoveryClaim(
	ctx context.Context,
	expected, replacement *Worktree,
	claim *models.TaskEnvironmentRecoveryClaim,
) (bool, error) {
	if s.claim == nil || claim == nil || s.claim.OperationID != claim.OperationID {
		return false, errRecoveryOperationClaimed
	}
	if s.stalePublish {
		delete(s.worktrees, expected.ID)
		s.worktrees[replacement.ID] = replacement
		return false, nil
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
	configureManagedCloneRelocationGitIdentity(t, sourceClone)
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
	progressReporter := &recordingRecoveryProgressReporter{}
	mgr.SetRecoveryProgressReporter(progressReporter)

	recoveryRequest := RecoveryAdmissionRequest{
		TaskID: "task-1", SessionID: "session-1", TaskEnvironmentID: "env-1", OwnerTaskID: "task-1",
		OwnershipGeneration: 1, ExecutorType: string(models.ExecutorTypeWorktree),
		Slots: []RecoverySlot{{
			WorktreeID: "wt-legacy", RepositoryID: "repo-1", BranchSlug: "branch-1", RepositoryPath: destinationClone,
			CloneRelocation: &ManagedCloneRelocationProof{
				ManagedRoot: managedRoot, ExpectedSourcePath: sourceClone, ExpectedDestinationPath: destinationClone,
				Identity: ManagedRepositoryIdentity{Provider: "github", Host: "github.com", Owner: "acme", Name: "widget"},
			},
		}},
	}
	store.claim = &models.TaskEnvironmentRecoveryClaim{
		TaskEnvironmentID: "env-1", OwnerTaskID: "another-task", OwnershipGeneration: 1,
		SessionID: "another-session", OperationID: "123e4567-e89b-12d3-a456-426614174000",
		ExecutorType: string(models.ExecutorTypeWorktree),
	}
	if _, err := mgr.AdmitRecovery(context.Background(), recoveryRequest); err == nil {
		t.Fatal("AdmitRecovery() ignored a competing environment recovery claim")
	}
	if store.worktrees[wt.ID] == nil || store.worktrees[wt.ID].Path != originalPath {
		t.Fatal("competing recovery claim changed the original worktree inventory")
	}
	if _, err := os.Lstat(originalPath + ".kandev-clone-relocation.json"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("competing recovery claim created a relocation journal: %v", err)
	}
	store.claim = nil
	store.stalePublish = true
	admission, err := mgr.AdmitRecovery(context.Background(), recoveryRequest)
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
	var relocationRecordPath string
	for _, artifact := range store.artifacts {
		if artifact.LayoutVersion != 2 || artifact.ReplacementID != replacement.ID || artifact.ReplacementPath != replacement.Path {
			continue
		}
		for _, path := range artifact.ArtifactPaths {
			if filepath.Base(path) == "relocation.json" && filepath.Base(filepath.Dir(path)) == "records" {
				relocationRecordPath = path
				break
			}
		}
	}
	if relocationRecordPath == "" {
		t.Fatal("private relocation journal was not registered")
	}
	if _, err := os.Lstat(originalPath + ".kandev-clone-relocation.json"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("relocation journal remains beside working files: %v", err)
	}
	relocationRecord, err := readManagedCloneRelocationRecord(relocationRecordPath)
	if err != nil {
		t.Fatalf("read relocation record: %v", err)
	}
	if relocationRecord.Original == originalPath || pathWithin(filepath.Join(config.TasksBasePath, "task-1"), relocationRecord.Original) {
		t.Fatalf("retained original remains discoverable under the task root: %q", relocationRecord.Original)
	}
	if got := strings.TrimSpace(runGit(t, relocationRecord.Original, "rev-parse", "HEAD")); got != wantHead {
		t.Fatalf("original checkout HEAD = %s, want retained commit %s", got, wantHead)
	}
	if _, err := os.Stat(filepath.Join(relocationRecord.Original, "unpublished.txt")); err != nil {
		t.Fatalf("original unpushed file was not retained: %v", err)
	}
	if got := strings.TrimSpace(runGit(t, replacement.Path, "rev-parse", "--path-format=absolute", "--git-common-dir")); filepath.Clean(got) != filepath.Clean(filepath.Join(destinationClone, ".git")) {
		t.Fatalf("replacement common dir = %q, want destination clone", got)
	}
	if err := admission.Release(context.Background()); err != nil {
		t.Fatalf("release initial admission: %v", err)
	}
	if progressReporter.start.OperationID != "" || len(progressReporter.updates) != 0 {
		t.Fatalf("automatic clean relocation emitted explicit repair progress: start=%+v updates=%+v", progressReporter.start, progressReporter.updates)
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
	configureManagedCloneRelocationGitIdentity(t, sourceClone)
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

func TestManagedCloneRecoveryPrivateArtifactsPreserveContent(t *testing.T) {
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
	configureManagedCloneRelocationGitIdentity(t, sourceClone)
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
	ignoredName := "ignored-relocation.txt"
	excludePath := filepath.Join(sourceClone, ".git", "info", "exclude")
	exclude, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read clone exclude file: %v", err)
	}
	if err := os.WriteFile(excludePath, append(exclude, []byte("\n"+ignoredName+"\n")...), 0o600); err != nil {
		t.Fatalf("ignore relocation test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(originalPath, ignoredName), []byte("ignored but retained\n"), 0o640); err != nil {
		t.Fatalf("write ignored file: %v", err)
	}
	if err := os.Symlink("dirty-untracked.txt", filepath.Join(originalPath, "dirty-link")); err != nil {
		t.Fatalf("create dirty symlink: %v", err)
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
	ctx = WithManagedCloneRelocationAuthorization(ctx, func(context.Context) error { return nil })
	request := RecoveryAdmissionRequest{
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
	}
	store.acquireErr = errors.New("environment has a live session or runtime")
	if _, err := mgr.AdmitRecovery(ctx, request); err == nil {
		t.Fatal("AdmitRecovery() accepted dirty relocation while the requesting session runtime was live")
	}
	if store.claimRequest.AllowCurrentSessionRuntime {
		t.Fatal("dirty relocation excluded the requesting session from runtime-consumer checks")
	}
	if store.worktrees[wt.ID] == nil || store.worktrees[wt.ID].Path != originalPath {
		t.Fatal("busy-runtime refusal changed the worktree inventory")
	}
	if _, err := os.Stat(originalPath + ".kandev-recovery.json"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("busy-runtime refusal started a snapshot: %v", err)
	}
	store.acquireErr = nil
	staleValidationCalls := 0
	staleCtx := WithDirtyCloneRelocation(context.Background())
	staleCtx = WithManagedCloneRelocationAuthorization(staleCtx, func(context.Context) error {
		staleValidationCalls++
		if staleValidationCalls == 3 {
			return errors.New("session error stamp changed")
		}
		return nil
	})
	if _, err := mgr.AdmitRecovery(staleCtx, request); !errors.Is(err, ErrManagedCloneRelocationAuthorizationStale) {
		t.Fatalf("AdmitRecovery() error = %v, want stale relocation authorization", err)
	}
	if staleValidationCalls != 3 {
		t.Fatalf("authorization validator calls = %d, want pre-claim, post-claim, and pre-mutation checks", staleValidationCalls)
	}
	if store.claim != nil {
		t.Fatal("stale authorization retained the durable recovery claim")
	}
	if store.worktrees[wt.ID] == nil || store.worktrees[wt.ID].Path != originalPath {
		t.Fatal("stale authorization changed the worktree inventory")
	}
	if _, err := os.Stat(originalPath + ".kandev-recovery.json"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale authorization started a snapshot: %v", err)
	}
	admission, err := mgr.AdmitRecovery(ctx, request)
	if err != nil {
		t.Fatalf("AdmitRecovery: %v", err)
	}
	if admission == nil {
		t.Fatal("AdmitRecovery() returned no admission for explicit dirty relocation")
	}
	if store.claimRequest.AllowCurrentSessionRuntime {
		t.Fatal("explicit dirty relocation excluded the requesting session runtime")
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
	if got, err := os.ReadFile(filepath.Join(replacement.Path, ignoredName)); err != nil || string(got) != "ignored but retained\n" {
		t.Fatalf("ignored file = %q, %v", got, err)
	}
	if target, err := os.Readlink(filepath.Join(replacement.Path, "dirty-link")); err != nil || target != "dirty-untracked.txt" {
		t.Fatalf("dirty symlink target = %q, %v", target, err)
	}
	if got := strings.TrimSpace(runGit(t, replacement.Path, "diff", "--cached", "--", "committed.txt")); got != "" {
		t.Fatalf("staging state unexpectedly transferred: %s", got)
	}
	if got := strings.TrimSpace(runGit(t, replacement.Path, "diff", "--", "committed.txt")); got == "" {
		t.Fatal("staged content was not retained as an unstaged modification")
	}
	artifactRecord := managedCloneRelocationRecord{
		OperationID: store.claim.OperationID, TaskID: wt.TaskID, EnvironmentID: wt.TaskEnvironmentID,
		WorktreeID: wt.ID, OriginalWorkspacePath: originalPath,
	}
	archivePath, err := mgr.managedCloneRelocationArchivePath(&artifactRecord)
	if err != nil {
		t.Fatalf("resolve private recovery bucket: %v", err)
	}
	privateBucket := filepath.Dir(archivePath)
	recoveryRecord, err := readRecoveryRecord(filepath.Join(privateBucket, "records", "recovery.json"))
	if err != nil {
		t.Fatalf("read private recovery snapshot record: %v", err)
	}
	if _, err := os.Stat(recoveryRecord.Snapshot); err != nil {
		t.Fatalf("retained snapshot missing: %v", err)
	}
	if _, err := os.Lstat(originalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("original checkout remains beside the replacement: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(recoveryRecord.Original, "dirty-untracked.txt")); err != nil || string(got) != "keep me\n" {
		t.Fatalf("original checkout was not retained: %q, %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(recoveryRecord.Original, ignoredName)); err != nil || string(got) != "ignored but retained\n" {
		t.Fatalf("ignored file was not retained in the original checkout: %q, %v", got, err)
	}
	if target, err := os.Readlink(filepath.Join(recoveryRecord.Original, "dirty-link")); err != nil || target != "dirty-untracked.txt" {
		t.Fatalf("original dirty symlink target = %q, %v", target, err)
	}
	for _, adjacent := range []string{
		originalPath + ".kandev-clone-relocation.json",
		originalPath + ".kandev-clone-relocation.claim",
		originalPath + ".kandev-recovery.json",
		originalPath + ".kandev-recovery.claim",
	} {
		if _, err := os.Lstat(adjacent); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("recovery artifact remains beside working files at %q: %v", adjacent, err)
		}
	}
	privateRelocation, err := readManagedCloneRelocationRecord(filepath.Join(privateBucket, "records", "relocation.json"))
	if err != nil || privateRelocation.State != string(RecoveryStateComplete) {
		t.Fatalf("private relocation record = %+v, %v", privateRelocation, err)
	}
	privateRecovery, err := readRecoveryRecord(filepath.Join(privateBucket, "records", "recovery.json"))
	if err != nil {
		t.Fatalf("read private recovery record: %v", err)
	}
	if !pathWithin(privateBucket, privateRecovery.Snapshot) {
		t.Fatalf("snapshot path %q is outside private bucket %q", privateRecovery.Snapshot, privateBucket)
	}
}

type recordingRecoveryProgressReporter struct {
	start   RecoveryProgressStart
	updates []RecoveryProgressUpdate
	binding RecoveryProgressBinding
	ended   bool
}

func (r *recordingRecoveryProgressReporter) BeginWorkspaceRecovery(
	_ context.Context,
	start RecoveryProgressStart,
) (RecoveryProgressBinding, error) {
	r.start = start
	r.binding = RecoveryProgressBinding{
		TaskEnvironmentID: start.TaskEnvironmentID, OwnerTaskID: start.OwnerTaskID,
		OwnershipGeneration: start.OwnershipGeneration, SessionID: start.SessionID,
		OperationID: start.OperationID, AttemptID: "attempt-progress", RunnerInstanceID: "runner-progress",
		Revision: 1,
	}
	return r.binding, nil
}

func (r *recordingRecoveryProgressReporter) UpdateWorkspaceRecovery(
	_ context.Context,
	binding RecoveryProgressBinding,
	update RecoveryProgressUpdate,
) (RecoveryProgressBinding, error) {
	if binding.Revision != r.binding.Revision || binding.AttemptID != r.binding.AttemptID {
		return RecoveryProgressBinding{}, recoveryoperation.ErrStaleWriter
	}
	r.updates = append(r.updates, update)
	r.binding.Revision++
	return r.binding, nil
}

func (r *recordingRecoveryProgressReporter) HeartbeatWorkspaceRecovery(
	_ context.Context,
	binding RecoveryProgressBinding,
) (RecoveryProgressBinding, error) {
	if binding.Revision != r.binding.Revision {
		return RecoveryProgressBinding{}, recoveryoperation.ErrStaleWriter
	}
	r.binding.Revision++
	return r.binding, nil
}

func (r *recordingRecoveryProgressReporter) EndWorkspaceRecoveryRunner(context.Context, RecoveryProgressBinding) {
	r.ended = true
}

func TestManagedCloneRecoveryProgressCoversEverySelectedRepositoryAndAgentOutcome(t *testing.T) {
	managedRoot := filepath.Join(t.TempDir(), "repos")
	config := newTestConfig(t)
	seed := initGitRepoForWorktreeTest(t)
	store := &managedCloneRelocationStore{recoveryCASStore: &recoveryCASStore{mockStore: newMockStore()}}
	reporter := &recordingRecoveryProgressReporter{}
	slots := make([]RecoverySlot, 0, 2)
	for index := 0; index < 2; index++ {
		name := fmt.Sprintf("progress-repo-%d", index)
		source := filepath.Join(managedRoot, "_providers", "github", "github.com", "acme", name)
		destination := filepath.Join(managedRoot, "workspaces", "workspace-1", "github", "acme", name)
		for _, path := range []string{source, destination} {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		runGit(t, seed, "clone", "--no-hardlinks", seed, source)
		runGit(t, seed, "clone", "--no-hardlinks", seed, destination)
		origin := "https://github.com/acme/" + name + ".git"
		runGit(t, source, "remote", "set-url", "origin", origin)
		runGit(t, destination, "remote", "set-url", "origin", origin)
		branch := fmt.Sprintf("feature/progress-%d", index)
		runGit(t, source, "checkout", "-b", branch)
		runGit(t, source, "checkout", "main")
		original := filepath.Join(config.TasksBasePath, "task-recovery-progress", name)
		if err := os.MkdirAll(filepath.Dir(original), 0o755); err != nil {
			t.Fatal(err)
		}
		runGit(t, source, "worktree", "add", original, branch)
		if err := os.WriteFile(filepath.Join(original, "dirty-progress.txt"), []byte("retain me\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		wt := &Worktree{
			ID: fmt.Sprintf("wt-progress-%d", index), TaskID: "task-recovery-progress",
			TaskEnvironmentID: "env-recovery-progress", RepositoryID: name,
			Path: original, RepositoryPath: destination, Branch: branch, BranchSlug: name, Status: StatusActive,
		}
		store.worktrees[wt.ID] = wt
		slots = append(slots, RecoverySlot{
			WorktreeID: wt.ID, RepositoryID: name, BranchSlug: name, RepositoryPath: destination,
			CloneRelocation: &ManagedCloneRelocationProof{
				ManagedRoot: managedRoot, ExpectedSourcePath: source, ExpectedDestinationPath: destination,
				Identity: ManagedRepositoryIdentity{Provider: "github", Host: "github.com", Owner: "acme", Name: name},
			},
		})
	}
	manager, err := NewManager(config, store, newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	manager.SetRecoveryProgressReporter(reporter)
	ctx := WithDirtyCloneRelocation(context.Background())
	ctx = WithManagedCloneRelocationErrorStamp(ctx, "error-progress")
	ctx = WithManagedCloneRelocationAuthorization(ctx, func(context.Context) error { return nil })
	admission, err := manager.AdmitRecovery(ctx, RecoveryAdmissionRequest{
		TaskID: "task-recovery-progress", SessionID: "session-recovery-progress",
		TaskEnvironmentID: "env-recovery-progress", OwnerTaskID: "task-recovery-progress",
		OwnershipGeneration: 8, ExecutorType: string(models.ExecutorTypeWorktree),
		ErrorStamp: "error-progress", RelocateDirty: true, Slots: slots,
	})
	if err != nil {
		t.Fatalf("AdmitRecovery: %v", err)
	}
	if admission == nil {
		t.Fatal("AdmitRecovery returned no admission for dirty managed clones")
	}
	if len(reporter.start.SelectedRepositoryIDs) != 2 ||
		reporter.start.SelectedRepositoryIDs[0] != "progress-repo-0" ||
		reporter.start.SelectedRepositoryIDs[1] != "progress-repo-1" {
		t.Fatalf("selected inventory = %+v", reporter.start.SelectedRepositoryIDs)
	}
	if err := admission.CompleteRecoveryResume(context.Background(), true, ""); err != nil {
		t.Fatalf("CompleteRecoveryResume: %v", err)
	}
	if err := admission.Release(context.Background()); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if !reporter.ended {
		t.Fatal("recovery runner was not unregistered")
	}
	phasesByRepository := make(map[string]map[string]bool)
	var final RecoveryProgressUpdate
	for _, update := range reporter.updates {
		if update.RepositoryID != "" {
			if phasesByRepository[update.RepositoryID] == nil {
				phasesByRepository[update.RepositoryID] = make(map[string]bool)
			}
			phasesByRepository[update.RepositoryID][update.Phase] = true
		}
		final = update
	}
	for _, repositoryID := range reporter.start.SelectedRepositoryIDs {
		for _, phase := range []string{
			recoveryoperation.PhaseSnapshotting, recoveryoperation.PhaseVerifyingSnapshot,
			recoveryoperation.PhaseVerifyingReplacement, recoveryoperation.PhasePublishing,
		} {
			if !phasesByRepository[repositoryID][phase] {
				t.Fatalf("repository %s omitted phase %s: %+v", repositoryID, phase, phasesByRepository[repositoryID])
			}
		}
	}
	if final.State != recoveryoperation.StateCompleted || final.Phase != recoveryoperation.PhaseResuming ||
		!final.WorkspaceComplete || !final.AgentReady || final.CompletedSlots != 2 {
		t.Fatalf("terminal recovery outcome = %+v", final)
	}
}

func TestManagedCloneRelocationRejectsWrongProviderOrigin(t *testing.T) {
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
	runGit(t, source, "remote", "set-url", "origin", "https://github.com/other/widget.git")
	runGit(t, destination, "remote", "set-url", "origin", "https://github.com/acme/widget.git")
	manager, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	identity := ManagedRepositoryIdentity{Provider: "github", Host: "github.com", Owner: "acme", Name: "widget"}
	if err := verifyManagedCloneOrigins(context.Background(), manager, source, destination, identity); err == nil {
		t.Fatal("managed clone relocation accepted a source with a different provider owner")
	}
}

func TestCanonicalManagedCloneDestinationRejectsForeignWorkspace(t *testing.T) {
	managedRoot := filepath.Join(t.TempDir(), "repos")
	expected := filepath.Join(managedRoot, "workspaces", "workspace-1", "github", "acme", "widget")
	foreign := filepath.Join(managedRoot, "workspaces", "workspace-2", "github", "acme", "widget")
	for _, path := range []string{expected, foreign} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	worktree := &Worktree{TaskID: "task-foreign", RepositoryPath: foreign}
	proof := &ManagedCloneRelocationProof{
		ManagedRoot: managedRoot, ExpectedDestinationPath: expected,
		Identity: ManagedRepositoryIdentity{Provider: "github", Host: "github.com", Owner: "acme", Name: "widget"},
	}
	if _, _, err := canonicalManagedCloneDestination(worktree.TaskID, worktree, proof); err == nil {
		t.Fatal("managed clone relocation accepted a destination from another workspace")
	}
}

func TestAdmitRecoveryDoesNotPartiallyRelocateMultiRepositoryInventory(t *testing.T) {
	managedRoot := filepath.Join(t.TempDir(), "repos")
	config := newTestConfig(t)
	seed := initGitRepoForWorktreeTest(t)
	store := &managedCloneRelocationStore{recoveryCASStore: &recoveryCASStore{mockStore: newMockStore()}}
	slots := make([]RecoverySlot, 0, 2)
	originalPaths := make([]string, 0, 2)
	for index := 0; index < 2; index++ {
		name := fmt.Sprintf("repo-%d", index)
		source := filepath.Join(managedRoot, "_providers", "github", "github.com", "acme", name)
		destination := filepath.Join(managedRoot, "workspaces", "workspace-1", "github", "acme", name)
		for _, path := range []string{source, destination} {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		runGit(t, seed, "clone", "--no-hardlinks", seed, source)
		runGit(t, seed, "clone", "--no-hardlinks", seed, destination)
		origin := "https://github.com/acme/" + name + ".git"
		if index == 1 {
			origin = "https://github.com/other/" + name + ".git"
		}
		runGit(t, source, "remote", "set-url", "origin", origin)
		runGit(t, destination, "remote", "set-url", "origin", "https://github.com/acme/"+name+".git")
		branch := fmt.Sprintf("feature/partial-%d", index)
		runGit(t, source, "checkout", "-b", branch)
		runGit(t, source, "checkout", "main")
		originalPath := filepath.Join(config.TasksBasePath, "task-partial", name)
		if err := os.MkdirAll(filepath.Dir(originalPath), 0o755); err != nil {
			t.Fatal(err)
		}
		runGit(t, source, "worktree", "add", originalPath, branch)
		worktree := &Worktree{
			ID: fmt.Sprintf("wt-partial-%d", index), TaskID: "task-partial", TaskEnvironmentID: "env-partial",
			RepositoryID: name, Path: originalPath, RepositoryPath: destination,
			Branch: branch, BranchSlug: name, Status: StatusActive,
		}
		store.worktrees[worktree.ID] = worktree
		slots = append(slots, RecoverySlot{
			WorktreeID: worktree.ID, RepositoryID: worktree.RepositoryID, BranchSlug: worktree.BranchSlug,
			RepositoryPath: destination, CloneRelocation: &ManagedCloneRelocationProof{
				ManagedRoot: managedRoot, ExpectedSourcePath: source, ExpectedDestinationPath: destination,
				Identity: ManagedRepositoryIdentity{Provider: "github", Host: "github.com", Owner: "acme", Name: name},
			},
		})
		originalPaths = append(originalPaths, originalPath)
	}
	manager, err := NewManager(config, store, newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.AdmitRecovery(context.Background(), RecoveryAdmissionRequest{
		TaskID: "task-partial", SessionID: "session-partial", TaskEnvironmentID: "env-partial",
		OwnerTaskID: "task-partial", OwnershipGeneration: 1,
		ExecutorType: string(models.ExecutorTypeWorktree), Slots: slots,
	})
	if err == nil {
		t.Fatal("AdmitRecovery() accepted a multi-repository inventory with a wrong origin")
	}
	if store.claim != nil {
		t.Fatal("failed multi-repository inspection acquired a durable claim")
	}
	for index, originalPath := range originalPaths {
		if _, err := os.Stat(originalPath); err != nil {
			t.Fatalf("repository %d original checkout was changed: %v", index, err)
		}
		replacements, err := filepath.Glob(originalPath + ".relocated-*")
		if err != nil || len(replacements) != 0 {
			t.Fatalf("repository %d created partial relocation paths %v, %v", index, replacements, err)
		}
		if store.worktrees[fmt.Sprintf("wt-partial-%d", index)].Path != originalPath {
			t.Fatalf("repository %d changed its authoritative worktree row", index)
		}
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

func configureManagedCloneRelocationGitIdentity(t *testing.T, repository string) {
	t.Helper()
	runGit(t, repository, "config", "user.name", "Relocation Test")
	runGit(t, repository, "config", "user.email", "relocation-test@example.invalid")
}
