package worktree

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/task/models"
)

type managedCloneRelocationInspection struct {
	mismatch   bool
	dirty      bool
	sourcePath string
	sourceGit  string
	destGit    string
	head       string
	branch     string
}

type managedCloneRelocationRecord struct {
	OperationID           string    `json:"operation_id"`
	TaskID                string    `json:"task_id"`
	EnvironmentID         string    `json:"environment_id"`
	WorktreeID            string    `json:"worktree_id"`
	Original              string    `json:"original"`
	OriginalWorkspacePath string    `json:"original_workspace_path,omitempty"`
	Replacement           string    `json:"replacement"`
	ReplacementID         string    `json:"replacement_id"`
	SourcePath            string    `json:"source_path"`
	SourceCommon          string    `json:"source_common_dir"`
	DestPath              string    `json:"destination_path"`
	DestCommon            string    `json:"destination_common_dir"`
	Branch                string    `json:"branch"`
	Head                  string    `json:"head"`
	State                 string    `json:"state"`
	UpdatedAt             time.Time `json:"updated_at"`
}

var relocationCommitPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

const (
	managedCloneRelocationStatePrepared     = "prepared"
	managedCloneRelocationStateMaterialized = "materialized"
	managedCloneRelocationStateBlocked      = "blocked"
	gitProtocolHTTP                         = "http"
	gitProtocolHTTPS                        = "https"
	gitProtocolSSH                          = "ssh"
)

func (m *Manager) inspectManagedCloneRelocation(
	ctx context.Context,
	taskID string,
	wt *Worktree,
	proof *ManagedCloneRelocationProof,
) (managedCloneRelocationInspection, error) {
	if !managedCloneRelocationProofComplete(wt, proof) {
		return managedCloneRelocationInspection{}, nil
	}
	root, destination, err := canonicalManagedCloneDestination(taskID, wt, proof)
	if err != nil {
		return managedCloneRelocationInspection{}, err
	}
	worktreeCommon, err := gitCommonDir(ctx, m, wt.Path)
	if err != nil {
		return managedCloneRelocationInspection{}, managedCloneRelocationError(taskID, "cannot verify the worktree Git identity")
	}
	if filepath.Clean(worktreeCommon) == filepath.Join(destination, ".git") {
		return managedCloneRelocationInspection{}, nil
	}
	source, err := managedCloneRelocationSource(taskID, root, destination, worktreeCommon, proof)
	if err != nil {
		return managedCloneRelocationInspection{}, err
	}
	if err := verifyRecordedManagedCloneSource(source, worktreeCommon, proof); err != nil {
		return managedCloneRelocationInspection{}, managedCloneRelocationError(taskID, "recorded source clone identity does not match Git registration")
	}
	if err := verifyManagedCloneOrigins(ctx, m, source, destination, proof.Identity); err != nil {
		return managedCloneRelocationInspection{}, managedCloneRelocationError(taskID, "managed clone provider identity could not be verified")
	}
	branch, registeredHead, err := inspectManagedCloneBranch(ctx, m, taskID, source, wt)
	if err != nil {
		return managedCloneRelocationInspection{}, err
	}
	if err := rejectUnsupportedCheckoutFeatures(ctx, m, wt.Path, destination); err != nil {
		return managedCloneRelocationInspection{}, managedCloneRelocationError(taskID, "worktree uses a checkout feature that cannot be transferred safely")
	}
	status, err := m.runBoundedGitInspect(ctx, wt.Path, "status", "--porcelain=v1", "--untracked-files=all", "--ignored=traditional")
	if err != nil {
		return managedCloneRelocationInspection{}, managedCloneRelocationError(taskID, "worktree cleanliness could not be verified")
	}
	return managedCloneRelocationInspection{
		mismatch: true, dirty: strings.TrimSpace(status) != "", sourcePath: source,
		sourceGit: worktreeCommon, destGit: filepath.Join(destination, ".git"),
		head: registeredHead, branch: branch,
	}, nil
}

func managedCloneRelocationProofComplete(wt *Worktree, proof *ManagedCloneRelocationProof) bool {
	if proof == nil || wt == nil || wt.Path == "" || proof.Identity.Provider == "" ||
		proof.Identity.Host == "" || proof.Identity.Owner == "" || proof.Identity.Name == "" {
		return false
	}
	return strings.EqualFold(proof.Identity.Provider, "github") || strings.EqualFold(proof.Identity.Provider, "gitlab")
}

func (m *Manager) validateManagedMainCheckoutIdentity(
	ctx context.Context,
	taskID string,
	wt *Worktree,
	proof *ManagedCloneRelocationProof,
) error {
	if !managedCloneRelocationProofComplete(wt, proof) {
		return nil
	}
	_, destination, err := canonicalManagedCloneDestination(taskID, wt, proof)
	if err != nil {
		return err
	}
	if !sameDirectoryIdentity(filepath.Join(wt.Path, ".git"), filepath.Join(destination, ".git")) {
		return managedCloneRelocationError(taskID, "main checkout does not match the selected managed destination")
	}
	if err := verifyManagedCloneOrigin(ctx, m, destination, proof.Identity); err != nil {
		if operationalErr := checkoutInspectionOperationalError(ctx, err); operationalErr != nil {
			return operationalErr
		}
		return managedCloneRelocationError(taskID, "main checkout provider origin does not match the selected repository")
	}
	return nil
}

func canonicalManagedCloneDestination(
	taskID string,
	wt *Worktree,
	proof *ManagedCloneRelocationProof,
) (string, string, error) {
	root, err := canonicalExistingPath(proof.ManagedRoot)
	if err != nil {
		return "", "", managedCloneRelocationError(taskID, "managed repository root is unavailable")
	}
	destination, err := canonicalExistingPath(proof.ExpectedDestinationPath)
	if err != nil || !pathWithin(root, destination) {
		return "", "", managedCloneRelocationError(taskID, "managed destination clone identity is invalid")
	}
	requestedRepository, err := canonicalExistingPath(wt.RepositoryPath)
	if err != nil || filepath.Clean(requestedRepository) != filepath.Clean(destination) {
		return "", "", managedCloneRelocationError(taskID, "selected repository does not match the managed destination")
	}
	return root, destination, nil
}

func managedCloneRelocationSource(
	taskID, root, destination, worktreeCommon string,
	proof *ManagedCloneRelocationProof,
) (string, error) {
	for _, candidate := range []string{proof.ExpectedSourcePath, proof.LegacyOwnerNameSourcePath} {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		canonical, err := canonicalExistingPath(candidate)
		if err != nil || !pathWithin(root, canonical) || filepath.Clean(canonical) == filepath.Clean(destination) {
			continue
		}
		if filepath.Clean(worktreeCommon) == filepath.Join(canonical, ".git") {
			return canonical, nil
		}
	}
	return "", managedCloneRelocationError(taskID, "worktree does not belong to the expected managed source clone")
}

func verifyRecordedManagedCloneSource(source, worktreeCommon string, proof *ManagedCloneRelocationProof) error {
	if proof.RecordedSourcePath == "" && proof.RecordedSourceCommonDir == "" {
		return nil
	}
	recordedPath, pathErr := canonicalExistingPath(proof.RecordedSourcePath)
	recordedCommon, commonErr := canonicalExistingPath(proof.RecordedSourceCommonDir)
	if pathErr != nil || commonErr != nil || recordedPath != source || recordedCommon != worktreeCommon {
		return fmt.Errorf("recorded source does not match worktree registration")
	}
	return nil
}

func verifyManagedCloneOrigins(
	ctx context.Context,
	m *Manager,
	source, destination string,
	identity ManagedRepositoryIdentity,
) error {
	if err := verifyManagedCloneOrigin(ctx, m, source, identity); err != nil {
		return err
	}
	return verifyManagedCloneOrigin(ctx, m, destination, identity)
}

func inspectManagedCloneBranch(
	ctx context.Context,
	m *Manager,
	taskID, source string,
	wt *Worktree,
) (string, string, error) {
	branch := strings.TrimSpace(wt.Branch)
	if branch == "" || strings.HasPrefix(branch, "refs/") {
		return "", "", managedCloneRelocationError(taskID, "worktree branch identity is incomplete")
	}
	if _, err := m.runBoundedGitInspect(ctx, source, "check-ref-format", "--branch", branch); err != nil {
		return "", "", managedCloneRelocationError(taskID, "worktree branch name is invalid")
	}
	head, registered, err := registeredWorktreeHead(ctx, m, source, wt.Path, branch)
	if err != nil || !registered || !relocationCommitPattern.MatchString(head) {
		return "", "", managedCloneRelocationError(taskID, "source clone registration does not match the worktree")
	}
	actualHead, err := m.runBoundedGitInspect(ctx, wt.Path, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || strings.TrimSpace(actualHead) != head {
		return "", "", managedCloneRelocationError(taskID, "worktree commit does not match its source registration")
	}
	if err := verifyRelocationBranch(ctx, m, source, branch, head); err != nil {
		return "", "", managedCloneRelocationError(taskID, "source branch does not point at the worktree commit")
	}
	return branch, head, nil
}

func (m *Manager) relocateCleanManagedCloneWorktree(
	ctx context.Context,
	wt *Worktree,
	proof *ManagedCloneRelocationProof,
	inspection managedCloneRelocationInspection,
	claim *models.TaskEnvironmentRecoveryClaim,
) (*Worktree, error) {
	if wt == nil || proof == nil || claim == nil || !inspection.mismatch || inspection.dirty {
		return nil, managedCloneRelocationError(wtTaskID(wt), "worktree is not eligible for automatic relocation")
	}
	jobPath := wt.Path + ".kandev-clone-relocation.json"
	lock, err := acquireRecoveryOperation(wt.Path + ".kandev-clone-relocation.claim")
	if err != nil {
		return nil, managedCloneRelocationError(wt.TaskID, "another clone relocation is active")
	}
	defer func() { _ = lock.Close() }()
	record, err := prepareManagedCloneRelocationRecord(jobPath, wt, proof, inspection, claim)
	if err != nil {
		return nil, err
	}
	if err := m.materializeManagedCloneRelocation(ctx, jobPath, &record); err != nil {
		return nil, err
	}
	replacement := managedCloneRelocationReplacement(wt, record)
	if !m.IsValid(replacement.Path) {
		return nil, managedCloneRelocationError(wt.TaskID, "replacement worktree failed integrity validation")
	}
	if err := verifyExactRelocationCheckout(ctx, m, &replacement, record); err != nil {
		return nil, err
	}
	if err := writePublishedManagedCloneRelocationRecord(&record); err != nil {
		return nil, managedCloneRelocationError(wt.TaskID, "cannot record replacement relocation state")
	}
	replacementPtr, err := m.publishManagedCloneReplacement(ctx, wt, &replacement, claim)
	if err != nil {
		return nil, err
	}
	archivePath, err := m.retainManagedCloneOriginal(ctx, &record)
	if err != nil {
		return nil, err
	}
	record.Original = archivePath
	if err := markManagedCloneRelocationComplete(jobPath, &record, wt.TaskID); err != nil {
		return nil, err
	}
	m.refreshRecoveredWorktreeCache(wt, replacementPtr)
	return replacementPtr, nil
}

func prepareManagedCloneRelocationRecord(
	jobPath string,
	wt *Worktree,
	proof *ManagedCloneRelocationProof,
	inspection managedCloneRelocationInspection,
	claim *models.TaskEnvironmentRecoveryClaim,
) (managedCloneRelocationRecord, error) {
	if claim == nil {
		return managedCloneRelocationRecord{}, managedCloneRelocationError(wtTaskID(wt), "relocation operation identity is invalid")
	}
	if _, err := uuid.Parse(claim.OperationID); err != nil || len(claim.OperationID) < 8 {
		return managedCloneRelocationRecord{}, managedCloneRelocationError(wt.TaskID, "relocation operation identity is invalid")
	}
	record := managedCloneRelocationRecord{
		OperationID: claim.OperationID, TaskID: wt.TaskID, EnvironmentID: wt.TaskEnvironmentID,
		WorktreeID: wt.ID, Original: wt.Path, OriginalWorkspacePath: wt.Path,
		Replacement:   wt.Path + ".relocated-" + claim.OperationID[:8],
		ReplacementID: uuid.NewString(), SourcePath: inspection.sourcePath,
		SourceCommon: inspection.sourceGit, DestPath: proof.ExpectedDestinationPath,
		DestCommon: inspection.destGit, Branch: inspection.branch, Head: inspection.head,
		State: managedCloneRelocationStatePrepared, UpdatedAt: time.Now().UTC(),
	}
	existing, err := readManagedCloneRelocationRecord(jobPath)
	if err == nil {
		if !managedCloneRelocationRecordMatches(existing, record) || existing.State == managedCloneRelocationStateBlocked {
			return managedCloneRelocationRecord{}, managedCloneRelocationError(wt.TaskID, "existing relocation state does not match the selected worktree")
		}
		return existing, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return managedCloneRelocationRecord{}, managedCloneRelocationError(wt.TaskID, "existing relocation state is unreadable")
	}
	if err := writeManagedCloneRelocationRecord(jobPath, record, true); err != nil {
		return managedCloneRelocationRecord{}, managedCloneRelocationError(wt.TaskID, "cannot record clone relocation")
	}
	return record, nil
}

func (m *Manager) materializeManagedCloneRelocation(
	ctx context.Context,
	jobPath string,
	record *managedCloneRelocationRecord,
) error {
	switch record.State {
	case managedCloneRelocationStatePrepared:
		if err := m.importManagedCommit(ctx, *record); err != nil {
			return err
		}
		if err := m.ensureExactRelocationWorktree(ctx, *record); err != nil {
			return err
		}
		record.State, record.UpdatedAt = managedCloneRelocationStateMaterialized, time.Now().UTC()
		if err := writeManagedCloneRelocationRecord(jobPath, *record, false); err != nil {
			return managedCloneRelocationError(record.TaskID, "cannot record completed clone materialization")
		}
	case managedCloneRelocationStateMaterialized, string(RecoveryStateComplete):
		return nil
	default:
		return managedCloneRelocationError(record.TaskID, "clone relocation state is not recoverable")
	}
	return nil
}

func managedCloneRelocationReplacement(wt *Worktree, record managedCloneRelocationRecord) Worktree {
	replacement := *wt
	replacement.ID = record.ReplacementID
	replacement.Path = record.Replacement
	replacement.RepositoryPath = record.DestPath
	replacement.SourceClonePath = record.DestPath
	replacement.SourceCommonDir = record.DestCommon
	replacement.UpdatedAt = time.Now().UTC()
	return replacement
}

func (m *Manager) publishManagedCloneReplacement(
	ctx context.Context,
	wt, replacement *Worktree,
	claim *models.TaskEnvironmentRecoveryClaim,
) (*Worktree, error) {
	cas, ok := m.store.(CompareAndSwapWorktreeWithRecoveryClaimStore)
	if !ok {
		return nil, managedCloneRelocationError(wt.TaskID, "guarded relocation publication is unavailable")
	}
	swapped, err := cas.CompareAndSwapWorktreeWithRecoveryClaim(ctx, wt, replacement, claim)
	if err != nil {
		return nil, fmt.Errorf("%w: guarded clone relocation publication failed", ErrWorktreeCorrupted)
	}
	if swapped {
		return replacement, nil
	}
	persisted, err := m.persistedRecoveryReplacement(ctx, wt, replacement)
	if err != nil || persisted == nil {
		return nil, managedCloneRelocationError(wt.TaskID, "worktree inventory changed during clone relocation")
	}
	return persisted, nil
}

func markManagedCloneRelocationComplete(jobPath string, record *managedCloneRelocationRecord, taskID string) error {
	record.State, record.UpdatedAt = string(RecoveryStateComplete), time.Now().UTC()
	paths := []string{
		jobPath,
		record.Replacement + ".kandev-clone-relocation.json",
		record.Original + ".kandev-clone-relocation.json",
	}
	for _, path := range paths {
		if path == ".kandev-clone-relocation.json" {
			continue
		}
		if err := writeManagedCloneRelocationRecord(path, *record, false); err != nil {
			return managedCloneRelocationError(taskID, "cannot record clone relocation publication")
		}
	}
	return nil
}

// relocateDirtyManagedCloneWorktree transfers a verified dirty checkout only
// after the recovery request carries its explicit, stamp-fenced authorization.
// The original and the byte/mode snapshot remain beside the task worktree.
func (m *Manager) relocateDirtyManagedCloneWorktree(
	ctx context.Context,
	wt *Worktree,
	proof *ManagedCloneRelocationProof,
	inspection managedCloneRelocationInspection,
	claim *models.TaskEnvironmentRecoveryClaim,
) (*Worktree, error) {
	if wt == nil || proof == nil || claim == nil || !inspection.mismatch || !inspection.dirty {
		return nil, managedCloneRelocationError(wtTaskID(wt), "worktree is not eligible for explicit relocation")
	}
	jobPath := wt.Path + ".kandev-clone-relocation.json"
	lock, err := acquireRecoveryOperation(wt.Path + ".kandev-clone-relocation.claim")
	if err != nil {
		return nil, managedCloneRelocationError(wt.TaskID, "another clone relocation is active")
	}
	defer func() { _ = lock.Close() }()
	record, err := prepareManagedCloneRelocationRecord(jobPath, wt, proof, inspection, claim)
	if err != nil {
		return nil, err
	}
	if err := m.materializeManagedCloneRelocation(ctx, jobPath, &record); err != nil {
		return nil, err
	}
	return m.transferDirtyManagedCloneFiles(ctx, wt, jobPath, &record, claim)
}

func (m *Manager) transferDirtyManagedCloneFiles(
	ctx context.Context,
	wt *Worktree,
	jobPath string,
	record *managedCloneRelocationRecord,
	claim *models.TaskEnvironmentRecoveryClaim,
) (*Worktree, error) {
	recoveryJobPath := wt.Path + ".kandev-recovery.json"
	if err := validateDirtyCloneRelocationAuthorization(ctx); err != nil {
		return nil, err
	}
	recovery, snapshotPath, recoveryLock, err := beginRecoveryWithOperation(wt, recoveryJobPath, claim.OperationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = recoveryLock.Close() }()
	manifest, err := prepareRecoverySnapshot(wt.Path, snapshotPath, recoveryJobPath, recovery)
	if err != nil {
		return nil, managedCloneRelocationError(wt.TaskID, "original checkout could not be safely snapshotted")
	}
	recovery.Manifest = manifest
	recovery.State = RecoveryStateRematerializing
	recovery.UpdatedAt = time.Now().UTC()
	if err := writeRecoveryRecord(recoveryJobPath, recovery); err != nil {
		return nil, managedCloneRelocationError(wt.TaskID, "snapshot state could not be recorded")
	}
	if err := restoreSnapshot(snapshotPath, record.Replacement, manifest); err != nil {
		return nil, blockRecovery(recoveryJobPath, recovery, err)
	}
	if err := verifyDirtyRelocationCheckout(ctx, m, *record, manifest); err != nil {
		return nil, blockRecovery(recoveryJobPath, recovery, err)
	}
	if err := writePublishedManagedCloneRelocationRecord(record); err != nil {
		return nil, blockRecovery(recoveryJobPath, recovery, err)
	}

	replacement := managedCloneRelocationReplacement(wt, *record)
	if _, ok := m.store.(CompareAndSwapWorktreeWithRecoveryClaimStore); !ok {
		return nil, blockRecovery(recoveryJobPath, recovery, fmt.Errorf("guarded relocation publication is unavailable"))
	}
	published, err := m.publishManagedCloneReplacement(ctx, wt, &replacement, claim)
	if err != nil {
		return nil, fmt.Errorf("%w: guarded dirty relocation publication failed", ErrWorktreeCorrupted)
	}
	archivePath, err := m.retainManagedCloneOriginal(ctx, record)
	if err != nil {
		return nil, err
	}
	recovery.Original = archivePath
	if _, err := m.completeRecovery(recoveryJobPath, recovery, wt, published); err != nil {
		return nil, err
	}
	record.Original = archivePath
	if err := markManagedCloneRelocationComplete(jobPath, record, wt.TaskID); err != nil {
		return nil, err
	}
	m.refreshRecoveredWorktreeCache(wt, published)
	return published, nil
}

func verifyDirtyRelocationCheckout(
	ctx context.Context,
	m *Manager,
	record managedCloneRelocationRecord,
	expectedManifest string,
) error {
	head, err := m.runBoundedGitInspect(ctx, record.Replacement, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || strings.TrimSpace(head) != record.Head {
		return managedCloneRelocationError(record.TaskID, "replacement worktree has an unexpected commit")
	}
	branch, err := m.runBoundedGitInspect(ctx, record.Replacement, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || strings.TrimSpace(branch) != record.Branch {
		return managedCloneRelocationError(record.TaskID, "replacement worktree has an unexpected branch")
	}
	common, err := gitCommonDir(ctx, m, record.Replacement)
	if err != nil || filepath.Clean(common) != filepath.Clean(record.DestCommon) {
		return managedCloneRelocationError(record.TaskID, "replacement worktree belongs to another clone")
	}
	manifest, err := checkoutManifest(record.Replacement)
	if err != nil || manifest != expectedManifest {
		return managedCloneRelocationError(record.TaskID, "replacement files do not match the verified snapshot")
	}
	return nil
}

func (m *Manager) importManagedCommit(ctx context.Context, record managedCloneRelocationRecord) error {
	branchRef := "refs/heads/" + record.Branch
	existing, err := m.branchExists(ctx, record.DestPath, branchRef)
	if err != nil {
		return managedCloneRelocationError(record.TaskID, "destination branch availability could not be verified")
	}
	if existing {
		return verifyDestinationRelocationBranch(ctx, m, record, branchRef)
	}
	if _, err := m.runBoundedGitInspect(ctx, record.DestPath, "fetch", "--no-tags", record.SourcePath, record.Head); err != nil {
		return managedCloneRelocationError(record.TaskID, "source commit could not be imported")
	}
	if _, err := m.runBoundedGitInspect(ctx, record.DestPath, "update-ref", branchRef, record.Head, strings.Repeat("0", len(record.Head))); err != nil {
		return managedCloneRelocationError(record.TaskID, "destination branch could not be created")
	}
	return verifyDestinationRelocationBranch(ctx, m, record, branchRef)
}

func verifyDestinationRelocationBranch(
	ctx context.Context,
	m *Manager,
	record managedCloneRelocationRecord,
	branchRef string,
) error {
	commit, err := m.runBoundedGitInspect(ctx, record.DestPath, "rev-parse", "--verify", branchRef+"^{commit}")
	if err != nil || strings.TrimSpace(commit) != record.Head {
		return managedCloneRelocationError(record.TaskID, "destination branch does not match the source commit")
	}
	return nil
}

func (m *Manager) ensureExactRelocationWorktree(ctx context.Context, record managedCloneRelocationRecord) error {
	if m.IsValid(record.Replacement) {
		return nil
	}
	if _, err := os.Lstat(record.Replacement); !errors.Is(err, os.ErrNotExist) {
		return managedCloneRelocationError(record.TaskID, "replacement path is occupied")
	}
	if _, err := m.gitAddWorktreeExisting(ctx, record.DestPath, record.Branch, record.Replacement); err != nil {
		return managedCloneRelocationError(record.TaskID, "destination worktree could not be materialized")
	}
	return nil
}

func verifyExactRelocationCheckout(ctx context.Context, m *Manager, wt *Worktree, record managedCloneRelocationRecord) error {
	head, err := m.runBoundedGitInspect(ctx, wt.Path, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || strings.TrimSpace(head) != record.Head {
		return managedCloneRelocationError(wt.TaskID, "replacement worktree has an unexpected commit")
	}
	branch, err := m.runBoundedGitInspect(ctx, wt.Path, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || strings.TrimSpace(branch) != record.Branch {
		return managedCloneRelocationError(wt.TaskID, "replacement worktree has an unexpected branch")
	}
	common, err := gitCommonDir(ctx, m, wt.Path)
	if err != nil || filepath.Clean(common) != filepath.Clean(record.DestCommon) {
		return managedCloneRelocationError(wt.TaskID, "replacement worktree belongs to another clone")
	}
	status, err := m.runBoundedGitInspect(ctx, wt.Path, "status", "--porcelain=v1", "--untracked-files=all", "--ignored=traditional")
	if err != nil || strings.TrimSpace(status) != "" {
		return managedCloneRelocationError(wt.TaskID, "replacement worktree is not clean")
	}
	return nil
}

func registeredWorktreeHead(ctx context.Context, m *Manager, repositoryPath, worktreePath, branch string) (string, bool, error) {
	output, err := m.runBoundedGitInspect(ctx, repositoryPath, "worktree", "list", "--porcelain")
	if err != nil {
		return "", false, err
	}
	canonical, err := canonicalExistingPath(worktreePath)
	if err != nil {
		return "", false, err
	}
	for _, stanza := range strings.Split(strings.TrimSpace(output), "\n\n") {
		var listedPath, head, listedBranch string
		for _, line := range strings.Split(stanza, "\n") {
			key, value, found := strings.Cut(line, " ")
			if !found {
				continue
			}
			switch key {
			case "worktree":
				listedPath = value
			case "HEAD":
				head = value
			case "branch":
				listedBranch = value
			}
		}
		listedCanonical, pathErr := canonicalExistingPath(listedPath)
		if pathErr == nil && listedCanonical == canonical && listedBranch == "refs/heads/"+branch {
			return head, true, nil
		}
	}
	return "", false, nil
}

func verifyRelocationBranch(ctx context.Context, m *Manager, source, branch, head string) error {
	output, err := m.runBoundedGitInspect(ctx, source, "rev-parse", "--verify", "refs/heads/"+branch+"^{commit}")
	if err != nil || strings.TrimSpace(output) != head {
		return fmt.Errorf("source branch does not identify the registered commit")
	}
	return nil
}

func rejectUnsupportedCheckoutFeatures(ctx context.Context, m *Manager, checkout, destination string) error {
	for _, path := range []string{checkout, destination} {
		output, err := m.runBoundedGitInspect(ctx, path, "config", "--get-regexp", `^filter\.`)
		if strings.TrimSpace(output) != "" {
			return fmt.Errorf("git checkout filters are unsupported")
		}
		if err != nil && !isGitConfigUnset(err, output) {
			return err
		}
	}
	sparse, err := m.runBoundedGitInspect(ctx, checkout, "config", "--bool", "core.sparseCheckout")
	if strings.EqualFold(strings.TrimSpace(sparse), "true") {
		return fmt.Errorf("sparse checkout is unsupported")
	}
	if err != nil && !isGitConfigUnset(err, sparse) {
		return err
	}
	entries, err := m.runBoundedGitInspect(ctx, checkout, "ls-files", "--stage")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(entries, "\n") {
		if strings.HasPrefix(line, "160000 ") {
			return fmt.Errorf("git submodules are unsupported")
		}
	}
	return nil
}

func verifyManagedCloneOrigin(ctx context.Context, m *Manager, path string, identity ManagedRepositoryIdentity) error {
	origin, err := m.runBoundedGitInspect(ctx, path, "remote", "get-url", "origin")
	if err != nil {
		return err
	}
	provider, host, owner, name, err := parseManagedGitOrigin(strings.TrimSpace(origin))
	if err != nil {
		return err
	}
	wantHost, err := normalizeManagedProviderHost(identity.Host)
	if err != nil {
		return err
	}
	if (provider != "" && !strings.EqualFold(provider, strings.TrimSpace(identity.Provider))) || !strings.EqualFold(host, wantHost) ||
		!strings.EqualFold(owner, strings.Trim(strings.TrimSpace(identity.Owner), "/")) ||
		!strings.EqualFold(name, strings.TrimSuffix(strings.TrimSpace(identity.Name), ".git")) {
		return fmt.Errorf("git origin identity mismatch")
	}
	return nil
}

func parseManagedGitOrigin(origin string) (provider, host, owner, name string, err error) {
	if origin == "" || strings.ContainsAny(origin, "\r\n\x00") {
		return "", "", "", "", fmt.Errorf("git origin is empty or malformed")
	}
	rawHost, rawPath, err := parseManagedGitOriginAddress(origin)
	if err != nil {
		return "", "", "", "", err
	}
	parsedHost, hostErr := url.Parse("ssh://" + rawHost)
	if hostErr != nil || parsedHost.Hostname() == "" || parsedHost.User != nil {
		return "", "", "", "", fmt.Errorf("git origin host is malformed")
	}
	host = strings.ToLower(parsedHost.Host)
	owner, name, err = managedGitOriginRepository(rawPath)
	if err != nil {
		return "", "", "", "", err
	}
	if strings.EqualFold(host, "github.com") {
		provider = "github"
	} else if strings.EqualFold(host, "gitlab.com") {
		provider = "gitlab"
	}
	return provider, host, owner, name, nil
}

func parseManagedGitOriginAddress(origin string) (string, string, error) {
	if strings.Contains(origin, "://") {
		return parseManagedGitOriginURL(origin)
	}
	return parseManagedGitOriginSCP(origin)
}

func parseManagedGitOriginURL(origin string) (string, string, error) {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", fmt.Errorf("git origin URL is malformed")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != gitProtocolHTTPS && scheme != gitProtocolHTTP && scheme != gitProtocolSSH {
		return "", "", fmt.Errorf("git origin scheme is unsupported")
	}
	if !managedGitOriginUserAllowed(scheme, parsed.User) {
		return "", "", fmt.Errorf("git origin credentials or SSH identity are unsupported")
	}
	return parsed.Host, parsed.Path, nil
}

func managedGitOriginUserAllowed(scheme string, user *url.Userinfo) bool {
	if user == nil {
		return true
	}
	if scheme != gitProtocolSSH || user.Username() != "git" {
		return false
	}
	password, hasPassword := user.Password()
	return !hasPassword && password == ""
}

func parseManagedGitOriginSCP(origin string) (string, string, error) {
	at := strings.IndexByte(origin, '@')
	colon := strings.IndexByte(origin, ':')
	if at < 0 || colon <= at || strings.Contains(origin[:at], "/") {
		return "", "", fmt.Errorf("git origin syntax is unsupported")
	}
	if origin[:at] != "git" {
		return "", "", fmt.Errorf("git origin SSH identity is unsupported")
	}
	return origin[at+1 : colon], origin[colon+1:], nil
}

func managedGitOriginRepository(rawPath string) (string, string, error) {
	parts := strings.Split(strings.Trim(strings.TrimSuffix(rawPath, ".git"), "/"), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("git origin repository path is incomplete")
	}
	owner, name := strings.Join(parts[:len(parts)-1], "/"), parts[len(parts)-1]
	if owner == "" || name == "" || strings.Contains(owner, "..") || strings.Contains(name, "..") {
		return "", "", fmt.Errorf("git origin repository path is unsafe")
	}
	return owner, name, nil
}

func normalizeManagedProviderHost(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") {
			return "", fmt.Errorf("provider host is invalid")
		}
		value = parsed.Host
	}
	if value == "" || strings.ContainsAny(value, "/\\@?#") {
		return "", fmt.Errorf("provider host is invalid")
	}
	return strings.ToLower(value), nil
}

func isGitConfigUnset(err error, output string) bool {
	var exitErr *exec.ExitError
	return strings.TrimSpace(output) == "" && errors.As(err, &exitErr) && exitErr.ExitCode() == 1
}

func gitCommonDir(ctx context.Context, m *Manager, path string) (string, error) {
	output, err := m.runBoundedGitInspect(ctx, path, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return canonicalExistingPath(strings.TrimSpace(output))
}

func canonicalExistingPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(filepath.Clean(abs))
}

func pathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != "."
}

func managedCloneRelocationError(taskID, reason string) error {
	return &WorktreeRecoveryError{TaskID: taskID, State: "managed-clone-relocation", Reason: reason}
}

func wtTaskID(wt *Worktree) string {
	if wt == nil {
		return ""
	}
	return wt.TaskID
}

func readManagedCloneRelocationRecord(path string) (managedCloneRelocationRecord, error) {
	var record managedCloneRelocationRecord
	data, err := os.ReadFile(path)
	if err != nil {
		return record, err
	}
	err = json.Unmarshal(data, &record)
	if err != nil || record.OperationID == "" || record.TaskID == "" || record.WorktreeID == "" || record.ReplacementID == "" {
		return managedCloneRelocationRecord{}, fmt.Errorf("relocation record is invalid")
	}
	if _, err := uuid.Parse(record.OperationID); err != nil {
		return managedCloneRelocationRecord{}, fmt.Errorf("relocation operation ID is invalid")
	}
	return record, nil
}

func writeManagedCloneRelocationRecord(path string, record managedCloneRelocationRecord, create bool) error {
	return writeRecoveryRecordJSON(path, record, create, ".kandev-relocation-")
}

func managedCloneRelocationRecordMatches(existing, expected managedCloneRelocationRecord) bool {
	return existing.OperationID == expected.OperationID && existing.TaskID == expected.TaskID &&
		existing.EnvironmentID == expected.EnvironmentID && existing.WorktreeID == expected.WorktreeID &&
		existing.Original == expected.Original && existing.Replacement == expected.Replacement &&
		existing.ReplacementID != "" && existing.SourcePath == expected.SourcePath &&
		existing.SourceCommon == expected.SourceCommon && existing.DestPath == expected.DestPath &&
		existing.DestCommon == expected.DestCommon && existing.Branch == expected.Branch && existing.Head == expected.Head
}

// These journal writes use atomic, mode-restricted sidecars.
func writeRecoveryRecordJSON(path string, value any, create bool, prefix string) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), prefix+"*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if create {
		if err := os.Link(tmpPath, path); err != nil {
			return err
		}
	} else if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return syncRecoveryDirectory(filepath.Dir(path))
}
