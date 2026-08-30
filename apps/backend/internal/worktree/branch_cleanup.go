package worktree

import (
	"context"
	"expvar"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"go.uber.org/zap"
)

const (
	BranchOwnerUnknown  = "unknown"
	BranchOwnerManaged  = "kandev"
	BranchOwnerExternal = "external"
)

type BranchRetentionReason string

const (
	RetainedUnknownOwner          BranchRetentionReason = "unknown_owner"
	RetainedExternalOwner         BranchRetentionReason = "external_owner"
	RetainedAmbiguousOwner        BranchRetentionReason = "ambiguous_owner"
	RetainedActiveReference       BranchRetentionReason = "active_reference"
	RetainedMissingMetadata       BranchRetentionReason = "missing_metadata"
	RetainedLiveWorktree          BranchRetentionReason = "live_worktree"
	RetainedProtectedRef          BranchRetentionReason = "protected_ref"
	RetainedMissingRef            BranchRetentionReason = "missing_ref"
	RetainedAncestryProbeFailed   BranchRetentionReason = "ancestry_probe_failed"
	RetainedNotIntegrated         BranchRetentionReason = "not_integrated"
	RetainedRecoveryPersistFailed BranchRetentionReason = "recovery_persist_failed"
	RetainedHeadChanged           BranchRetentionReason = "head_changed"
	RetainedSafeDeleteRefused     BranchRetentionReason = "safe_delete_refused"
)

type BranchCleanupReceipt struct {
	Attempted       int
	Deleted         int
	Retained        int
	RetainedReasons map[BranchRetentionReason]int
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
	receipt := newBranchCleanupReceipt()
	receipt.Attempted = 1
	branchCleanupMetrics.Add("attempted", 1)

	metadataStore, reason := m.branchMetadataStoreForCleanup(ctx, wt)
	if reason != "" {
		receipt.retain(reason)
		return receipt
	}
	branchHead, reason := m.verifiedIntegratedBranchHead(ctx, wt)
	if reason != "" {
		receipt.retain(reason)
		return receipt
	}
	if reason = m.persistRecoveryAndDeleteBranch(ctx, metadataStore, wt, branchHead); reason != "" {
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
	owners, err := metadataStore.CountWorktreeBranchOwners(ctx, wt.RepositoryID, wt.Branch)
	if err != nil || owners != 1 {
		return nil, RetainedAmbiguousOwner
	}
	return metadataStore, ""
}

func (m *Manager) verifiedIntegratedBranchHead(
	ctx context.Context, wt *Worktree,
) (string, BranchRetentionReason) {
	branchRef := "refs/heads/" + wt.Branch
	live, err := branchCheckedOutInWorktree(ctx, wt.RepositoryPath, branchRef)
	if err != nil {
		return "", RetainedAncestryProbeFailed
	}
	if live {
		return "", RetainedLiveWorktree
	}
	if protectedBranchName(wt.Branch, wt.BaseBranch) || protectedBranchName(wt.Branch, wt.IntegrationRef) {
		return "", RetainedProtectedRef
	}

	branchHead, err := m.resolveCommit(ctx, wt.RepositoryPath, branchRef)
	if err != nil {
		return "", RetainedMissingRef
	}
	integrationRef := normalizeIntegrationRef(wt.IntegrationRef)
	integrationHead, err := m.resolveCommit(ctx, wt.RepositoryPath, integrationRef)
	if err != nil {
		return "", RetainedMissingRef
	}
	integrated, err := m.refContains(ctx, wt.RepositoryPath, integrationHead, branchHead)
	if err != nil {
		return "", RetainedAncestryProbeFailed
	}
	if !integrated {
		return "", RetainedNotIntegrated
	}
	return branchHead, ""
}

func (m *Manager) persistRecoveryAndDeleteBranch(
	ctx context.Context, metadataStore BranchMetadataStore, wt *Worktree, branchHead string,
) BranchRetentionReason {
	persisted, err := metadataStore.PersistBranchRecoveryHead(ctx, wt.ID, wt.RecoveryHeadSHA, branchHead)
	if err != nil || !persisted {
		return RetainedRecoveryPersistFailed
	}
	wt.RecoveryHeadSHA = branchHead

	branchRef := "refs/heads/" + wt.Branch
	currentHead, err := m.resolveCommit(ctx, wt.RepositoryPath, branchRef)
	if err != nil || currentHead != branchHead {
		return RetainedHeadChanged
	}
	cmd := m.newNonInteractiveGitCmd(ctx, wt.RepositoryPath, "branch", "-d", "--", wt.Branch)
	if output, err := runGitCmdCombinedOutput(ctx, cmd); err != nil {
		m.logger.Debug("safe managed branch deletion refused", zap.String("output", strings.TrimSpace(string(output))))
		return RetainedSafeDeleteRefused
	}
	return ""
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

func branchCheckedOutInWorktree(ctx context.Context, repoPath, branchRef string) (bool, error) {
	cmd := newGitCommand(ctx, "worktree", "list", "--porcelain", "-z")
	cmd.Dir = repoPath
	output, err := runGitCmdCombinedOutput(ctx, cmd)
	if err != nil {
		return false, err
	}
	for _, registration := range parseWorktreeRegistrations(string(output)) {
		if registration.branch == branchRef {
			return true, nil
		}
	}
	return false, nil
}

func (m *Manager) resolveCommit(ctx context.Context, repoPath, ref string) (string, error) {
	cmd := m.newNonInteractiveGitCmd(ctx, repoPath, "rev-parse", "--verify", ref+"^{commit}")
	output, err := runGitCmdCombinedOutput(ctx, cmd)
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(output))
	if !commitSHA.MatchString(sha) {
		return "", fmt.Errorf("resolved invalid commit object")
	}
	return strings.ToLower(sha), nil
}
