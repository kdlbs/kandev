package worktree

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	BranchOwnerUnknown  = "unknown"
	BranchOwnerManaged  = "kandev"
	BranchOwnerExternal = "external"
)

type BranchRetentionReason string

const (
	RetainedUnknownOwner            BranchRetentionReason = "unknown_owner"
	RetainedExternalOwner           BranchRetentionReason = "external_owner"
	RetainedAmbiguousOwner          BranchRetentionReason = "ambiguous_owner"
	RetainedActiveReference         BranchRetentionReason = "active_reference"
	RetainedMissingMetadata         BranchRetentionReason = "missing_metadata"
	RetainedLiveWorktree            BranchRetentionReason = "live_worktree"
	RetainedProtectedRef            BranchRetentionReason = "protected_ref"
	RetainedProtectedRefUnavailable BranchRetentionReason = "protected_ref_unavailable"
	RetainedMissingRef              BranchRetentionReason = "missing_ref"
	RetainedAncestryProbeFailed     BranchRetentionReason = "ancestry_probe_failed"
	RetainedNotIntegrated           BranchRetentionReason = "not_integrated"
	RetainedRecoveryPersistFailed   BranchRetentionReason = "recovery_persist_failed"
	RetainedHeadChanged             BranchRetentionReason = "head_changed"
	RetainedSafeDeleteRefused       BranchRetentionReason = "safe_delete_refused"
	RetainedArchiveStateChanged     BranchRetentionReason = "archive_state_changed"
)

type BranchCleanupReceipt struct {
	Attempted       int                           `json:"attempted"`
	Deleted         int                           `json:"deleted"`
	Retained        int                           `json:"retained"`
	RetainedReasons map[BranchRetentionReason]int `json:"retained_reasons"`
}

var branchCleanupMetrics = expvar.NewMap("worktree_branch_cleanup_total")

func createdBranchOwner(checkoutBranch, actualBranch string) string {
	if checkoutBranch == "" || checkoutBranch != actualBranch {
		return BranchOwnerManaged
	}
	return BranchOwnerExternal
}

func newBranchCleanupReceipt() BranchCleanupReceipt {
	return BranchCleanupReceipt{RetainedReasons: make(map[BranchRetentionReason]int)}
}

func (r *BranchCleanupReceipt) merge(other BranchCleanupReceipt) {
	r.Attempted += other.Attempted
	r.Deleted += other.Deleted
	r.Retained += other.Retained
	if r.RetainedReasons == nil {
		r.RetainedReasons = make(map[BranchRetentionReason]int)
	}
	for reason, count := range other.RetainedReasons {
		r.RetainedReasons[reason] += count
	}
}

func (r *BranchCleanupReceipt) retain(reason BranchRetentionReason) {
	r.Retained++
	r.RetainedReasons[reason]++
	branchCleanupMetrics.Add("retained:"+string(reason), 1)
}

func (r BranchCleanupReceipt) reasonFields() []zap.Field {
	reasons := make([]string, 0, len(r.RetainedReasons))
	for reason := range r.RetainedReasons {
		reasons = append(reasons, string(reason))
	}
	sort.Strings(reasons)
	fields := make([]zap.Field, 0, len(reasons)+3)
	fields = append(fields,
		zap.Int("attempted", r.Attempted),
		zap.Int("deleted", r.Deleted),
		zap.Int("retained", r.Retained),
	)
	for _, reason := range reasons {
		fields = append(fields, zap.Int("retained_"+reason, r.RetainedReasons[BranchRetentionReason(reason)]))
	}
	return fields
}

var commitSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}([0-9a-fA-F]{24})?$`)

func (m *Manager) compactManagedBranch(ctx context.Context, wt *Worktree) BranchCleanupReceipt {
	return m.compactManagedBranchWithMode(ctx, wt, false)
}

func (m *Manager) compactArchivedManagedBranch(ctx context.Context, wt *Worktree) BranchCleanupReceipt {
	return m.compactManagedBranchWithMode(ctx, wt, true)
}

func (m *Manager) compactManagedBranchWithMode(
	ctx context.Context, wt *Worktree, requireArchived bool,
) BranchCleanupReceipt {
	receipt := newBranchCleanupReceipt()
	receipt.Attempted = 1
	branchCleanupMetrics.Add("attempted", 1)

	metadataStore, reason := m.branchMetadataStoreForCleanup(ctx, wt)
	if reason != "" {
		receipt.retain(reason)
		return receipt
	}
	completed, reason := m.reconcileInterruptedArchivedCompaction(ctx, metadataStore, wt, requireArchived)
	if reason != "" {
		receipt.retain(reason)
		return receipt
	}
	if completed {
		receipt.Deleted = 1
		branchCleanupMetrics.Add("deleted", 1)
		return receipt
	}
	branchHead, reason := m.verifiedIntegratedBranchHead(ctx, wt, requireArchived)
	if reason != "" {
		receipt.retain(reason)
		return receipt
	}
	if reason = m.persistRecoveryAndDeleteBranch(ctx, metadataStore, wt, branchHead, requireArchived); reason != "" {
		receipt.retain(reason)
		return receipt
	}

	receipt.Deleted = 1
	branchCleanupMetrics.Add("deleted", 1)
	return receipt
}

func (m *Manager) branchMetadataStoreForCleanup(
	ctx context.Context, wt *Worktree,
) (BranchMetadataStore, BranchRetentionReason) {
	switch wt.BranchOwner {
	case BranchOwnerManaged:
	case BranchOwnerExternal:
		return nil, RetainedExternalOwner
	default:
		return nil, RetainedUnknownOwner
	}
	if strings.TrimSpace(wt.ID) == "" || strings.TrimSpace(wt.RepositoryID) == "" ||
		strings.TrimSpace(wt.RepositoryPath) == "" || strings.TrimSpace(wt.Branch) == "" ||
		strings.TrimSpace(wt.IntegrationRef) == "" {
		return nil, RetainedMissingMetadata
	}
	metadataStore, ok := m.store.(BranchMetadataStore)
	if !ok {
		return nil, RetainedMissingMetadata
	}
	owners, err := metadataStore.CountWorktreeBranchOwners(ctx, wt.RepositoryPath, wt.Branch)
	if err != nil || owners != 1 {
		return nil, RetainedAmbiguousOwner
	}
	return metadataStore, ""
}

func (m *Manager) verifiedIntegratedBranchHead(
	ctx context.Context, wt *Worktree, requireArchived bool,
) (string, BranchRetentionReason) {
	branchRef := "refs/heads/" + wt.Branch
	live, err := m.branchCheckedOutInWorktree(ctx, wt.RepositoryPath, branchRef)
	if err != nil {
		return "", RetainedAncestryProbeFailed
	}
	if live {
		return "", RetainedLiveWorktree
	}
	if protectedBranchName(wt.Branch, wt.BaseBranch) ||
		protectedBranchName(wt.Branch, wt.IntegrationRef) {
		return "", RetainedProtectedRef
	}
	protected, reason := m.protectedRepositoryDefaultBranch(ctx, wt)
	if reason != "" {
		return "", reason
	}
	if protected {
		return "", RetainedProtectedRef
	}

	branchHead, err := m.resolveCommit(ctx, wt.RepositoryPath, branchRef)
	if err != nil {
		return "", RetainedMissingRef
	}
	if reason := m.verifyHeadAgainstIntegration(ctx, wt, branchHead); reason != "" {
		return "", reason
	}
	return branchHead, ""
}

// protectedRepositoryDefaultBranch loads the canonical default branch from
// persisted repository metadata. Worktree BaseBranch and IntegrationRef may
// differ from the canonical default branch, so every inferred-safe compaction
// must prove this protection independently. The lookup is intentionally exact
// and never derives a protected ref from the candidate branch name or a remote
// short name.
func (m *Manager) protectedRepositoryDefaultBranch(
	ctx context.Context, wt *Worktree,
) (bool, BranchRetentionReason) {
	if m.repoProvider == nil || wt == nil || strings.TrimSpace(wt.RepositoryID) == "" {
		return false, RetainedProtectedRefUnavailable
	}
	repo, err := m.repoProvider.GetRepository(ctx, wt.RepositoryID)
	if err != nil || repo == nil || strings.TrimSpace(repo.DefaultBranch) == "" {
		if err != nil {
			m.logger.Debug("repository default branch unavailable during cleanup", zap.Error(err))
		}
		return false, RetainedProtectedRefUnavailable
	}
	return protectedBranchName(wt.Branch, repo.DefaultBranch), ""
}

func (m *Manager) verifyHeadAgainstIntegration(
	ctx context.Context, wt *Worktree, branchHead string,
) BranchRetentionReason {
	integrationRef := normalizeIntegrationRef(wt.IntegrationRef)
	integrationHead, err := m.resolveCommit(ctx, wt.RepositoryPath, integrationRef)
	if err != nil {
		return RetainedMissingRef
	}
	integrated, err := m.refContains(ctx, wt.RepositoryPath, integrationHead, branchHead)
	if err != nil {
		return RetainedAncestryProbeFailed
	}
	if !integrated {
		return RetainedNotIntegrated
	}
	return ""
}

func (m *Manager) persistRecoveryAndDeleteBranch(
	ctx context.Context,
	metadataStore BranchMetadataStore,
	wt *Worktree,
	branchHead string,
	requireArchived bool,
) BranchRetentionReason {
	persisted, reason := m.persistBranchRecoveryHead(
		ctx, metadataStore, wt, branchHead, requireArchived,
	)
	if reason != "" {
		return reason
	}
	wt.RecoveryHeadSHA = branchHead
	if requireArchived && !m.archivedBranchMutationStillAllowed(ctx, metadataStore, wt, branchHead) {
		return RetainedArchiveStateChanged
	}
	if !persisted {
		return RetainedRecoveryPersistFailed
	}
	reason = m.deleteExpectedBranchRef(ctx, metadataStore, wt, branchHead)
	if reason == "" {
		m.recordBranchCompactionComplete(ctx, metadataStore, wt, branchHead, requireArchived)
	}
	return reason
}

func (m *Manager) persistBranchRecoveryHead(
	ctx context.Context,
	metadataStore BranchMetadataStore,
	wt *Worktree,
	branchHead string,
	requireArchived bool,
) (bool, BranchRetentionReason) {
	if requireArchived {
		maintenanceStore, ok := m.store.(ArchivedBranchMaintenanceStore)
		if !ok {
			return false, RetainedArchiveStateChanged
		}
		persisted, err := maintenanceStore.PersistArchivedBranchRecoveryHead(
			ctx, wt.ID, wt.RecoveryHeadSHA, branchHead,
		)
		if err == nil && !persisted {
			return false, RetainedArchiveStateChanged
		}
		if err != nil {
			return false, RetainedRecoveryPersistFailed
		}
		return true, ""
	}
	persisted, err := metadataStore.PersistBranchRecoveryHead(ctx, wt.ID, wt.RecoveryHeadSHA, branchHead)
	if err != nil || !persisted {
		return false, RetainedRecoveryPersistFailed
	}
	return true, ""
}

func (m *Manager) archivedBranchMutationStillAllowed(
	ctx context.Context, metadataStore BranchMetadataStore, wt *Worktree, branchHead string,
) bool {
	maintenanceStore, ok := m.store.(ArchivedBranchMaintenanceStore)
	if !ok {
		return false
	}
	eligible, err := maintenanceStore.IsArchivedBranchCandidate(ctx, wt.ID)
	if err == nil && eligible {
		return true
	}
	m.clearBranchRecoveryHead(ctx, metadataStore, wt, branchHead)
	return false
}

func (m *Manager) deleteExpectedBranchRef(
	ctx context.Context, metadataStore BranchMetadataStore, wt *Worktree, branchHead string,
) BranchRetentionReason {
	branchRef := "refs/heads/" + wt.Branch
	live, err := m.branchCheckedOutInWorktree(ctx, wt.RepositoryPath, branchRef)
	if err != nil {
		m.clearBranchRecoveryHead(ctx, metadataStore, wt, branchHead)
		return RetainedAncestryProbeFailed
	}
	if live {
		m.clearBranchRecoveryHead(ctx, metadataStore, wt, branchHead)
		return RetainedLiveWorktree
	}
	// Recheck immediately before the destructive ref mutation. A worktree can
	// be attached after the initial eligibility probe without changing the
	// branch head, and update-ref does not enforce Git's checked-out-branch
	// protection.
	live, err = m.branchCheckedOutInWorktree(ctx, wt.RepositoryPath, branchRef)
	if err != nil {
		m.clearBranchRecoveryHead(ctx, metadataStore, wt, branchHead)
		return RetainedAncestryProbeFailed
	}
	if live {
		m.clearBranchRecoveryHead(ctx, metadataStore, wt, branchHead)
		return RetainedLiveWorktree
	}
	mutationCtx, cancel := context.WithTimeout(ctx, m.inspectTimeout)
	defer cancel()
	cmd := m.newNonInteractiveGitCmd(mutationCtx, wt.RepositoryPath, "update-ref", "-d", branchRef, branchHead)
	output, err := runGitCmdCombinedOutput(mutationCtx, cmd)
	if err == nil {
		return m.verifyDeletionDidNotRaceLiveness(ctx, metadataStore, wt, branchRef, branchHead)
	}
	currentHead, resolveErr := m.resolveCommit(ctx, wt.RepositoryPath, branchRef)
	if resolveErr == nil {
		m.clearBranchRecoveryHead(ctx, metadataStore, wt, branchHead)
		if currentHead != branchHead {
			return RetainedHeadChanged
		}
	}
	m.logger.Debug("exact managed branch deletion refused", zap.String("output", strings.TrimSpace(string(output))))
	return RetainedSafeDeleteRefused
}

func (m *Manager) verifyDeletionDidNotRaceLiveness(
	ctx context.Context,
	metadataStore BranchMetadataStore,
	wt *Worktree,
	branchRef string,
	branchHead string,
) BranchRetentionReason {
	safetyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), m.inspectTimeout)
	defer cancel()
	live, err := m.branchCheckedOutInWorktree(safetyCtx, wt.RepositoryPath, branchRef)
	if err == nil && !live {
		return ""
	}
	restoreReason := m.restoreDeletedBranchRef(safetyCtx, wt, branchRef, branchHead)
	if restoreReason != "" {
		return restoreReason
	}
	m.clearBranchRecoveryHead(safetyCtx, metadataStore, wt, branchHead)
	if err != nil {
		return RetainedAncestryProbeFailed
	}
	return RetainedLiveWorktree
}

func (m *Manager) restoreDeletedBranchRef(
	ctx context.Context, wt *Worktree, branchRef, branchHead string,
) BranchRetentionReason {
	zeroOID := strings.Repeat("0", len(branchHead))
	cmd := m.newNonInteractiveGitCmd(ctx, wt.RepositoryPath, "update-ref", branchRef, branchHead, zeroOID)
	if _, err := runGitCmdCombinedOutput(ctx, cmd); err == nil {
		return ""
	}
	currentHead, err := m.resolveCommit(ctx, wt.RepositoryPath, branchRef)
	if err != nil {
		return RetainedSafeDeleteRefused
	}
	if currentHead != branchHead {
		return RetainedHeadChanged
	}
	return ""
}

func (m *Manager) recordBranchCompactionComplete(
	ctx context.Context,
	metadataStore BranchMetadataStore,
	wt *Worktree,
	branchHead string,
	requireArchived bool,
) {
	var persisted bool
	var err error
	if requireArchived {
		maintenanceStore, ok := m.store.(ArchivedBranchMaintenanceStore)
		if ok {
			persisted, err = maintenanceStore.PersistArchivedBranchCompactionComplete(ctx, wt.ID, branchHead)
		}
	} else {
		persisted, err = metadataStore.PersistBranchCompactionComplete(ctx, wt.ID, branchHead)
	}
	if err != nil || !persisted {
		m.logger.Warn("managed branch deletion completion was not persisted", zap.Error(err))
		return
	}
	now := time.Now().UTC()
	wt.BranchCompactedAt = &now
}

func (m *Manager) reconcileInterruptedArchivedCompaction(
	ctx context.Context,
	metadataStore BranchMetadataStore,
	wt *Worktree,
	requireArchived bool,
) (bool, BranchRetentionReason) {
	if !requireArchived || strings.TrimSpace(wt.RecoveryHeadSHA) == "" {
		return false, ""
	}
	branchRef := "refs/heads/" + wt.Branch
	exists, err := m.localBranchRefExists(ctx, wt.RepositoryPath, branchRef)
	if err != nil {
		return true, RetainedAncestryProbeFailed
	}
	if exists {
		currentHead, resolveErr := m.resolveCommit(ctx, wt.RepositoryPath, branchRef)
		if resolveErr != nil {
			return true, RetainedMissingRef
		}
		if !strings.EqualFold(currentHead, wt.RecoveryHeadSHA) {
			// A local ref may have advanced after the recovery head was
			// durably recorded. Retain it and never replace the recovery SHA.
			return true, RetainedHeadChanged
		}
		return false, ""
	}
	return m.completeInterruptedArchivedCompaction(ctx, metadataStore, wt, branchRef)
}

func (m *Manager) completeInterruptedArchivedCompaction(
	ctx context.Context,
	metadataStore BranchMetadataStore,
	wt *Worktree,
	branchRef string,
) (bool, BranchRetentionReason) {
	live, err := m.branchCheckedOutInWorktree(ctx, wt.RepositoryPath, branchRef)
	if err != nil {
		return true, RetainedAncestryProbeFailed
	}
	if live {
		reason := m.restoreDeletedBranchRef(ctx, wt, branchRef, wt.RecoveryHeadSHA)
		if reason == "" {
			m.clearBranchRecoveryHead(ctx, metadataStore, wt, wt.RecoveryHeadSHA)
			reason = RetainedLiveWorktree
		}
		return true, reason
	}
	if protectedBranchName(wt.Branch, wt.BaseBranch) ||
		protectedBranchName(wt.Branch, wt.IntegrationRef) {
		return true, RetainedProtectedRef
	}
	protected, reason := m.protectedRepositoryDefaultBranch(ctx, wt)
	if reason != "" {
		return true, reason
	}
	if protected {
		return true, RetainedProtectedRef
	}
	branchHead, err := m.resolveCommit(ctx, wt.RepositoryPath, wt.RecoveryHeadSHA)
	if err != nil || branchHead != strings.ToLower(wt.RecoveryHeadSHA) {
		return true, RetainedMissingRef
	}
	if reason := m.verifyHeadAgainstIntegration(ctx, wt, branchHead); reason != "" {
		return true, reason
	}
	maintenanceStore, ok := m.store.(ArchivedBranchMaintenanceStore)
	if !ok {
		return true, RetainedArchiveStateChanged
	}
	persisted, err := maintenanceStore.PersistArchivedBranchCompactionComplete(ctx, wt.ID, branchHead)
	if err != nil {
		return true, RetainedRecoveryPersistFailed
	}
	if !persisted {
		return true, RetainedArchiveStateChanged
	}
	return true, ""
}

func (m *Manager) localBranchRefExists(ctx context.Context, repoPath, branchRef string) (bool, error) {
	_, err := m.runBoundedGitInspect(ctx, repoPath, "show-ref", "--verify", "--quiet", branchRef)
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

func (m *Manager) clearBranchRecoveryHead(
	ctx context.Context, metadataStore BranchMetadataStore, wt *Worktree, branchHead string,
) {
	cleared, err := metadataStore.PersistBranchRecoveryHead(ctx, wt.ID, branchHead, "")
	if err == nil && cleared {
		wt.RecoveryHeadSHA = ""
	}
}

func protectedBranchName(branch, protectedRef string) bool {
	protectedRef = strings.TrimSpace(protectedRef)
	protectedRef = strings.TrimPrefix(protectedRef, "refs/heads/")
	protectedRef = strings.TrimPrefix(protectedRef, "refs/remotes/origin/")
	protectedRef = strings.TrimPrefix(protectedRef, "origin/")
	return protectedRef != "" && branch == protectedRef
}

func normalizeIntegrationRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "refs/") {
		return ref
	}
	if strings.HasPrefix(ref, "origin/") {
		return "refs/remotes/" + ref
	}
	return "refs/heads/" + ref
}

func (m *Manager) branchCheckedOutInWorktree(ctx context.Context, repoPath, branchRef string) (bool, error) {
	output, err := m.runBoundedGitInspect(ctx, repoPath, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return false, err
	}
	for _, registration := range parseWorktreeRegistrations(output) {
		if registration.branch == branchRef {
			return true, nil
		}
	}
	return false, nil
}

func (m *Manager) resolveCommit(ctx context.Context, repoPath, ref string) (string, error) {
	output, err := m.runBoundedGitInspect(ctx, repoPath, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(output)
	if !commitSHA.MatchString(sha) {
		return "", fmt.Errorf("resolved invalid commit object")
	}
	return strings.ToLower(sha), nil
}
