package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"go.uber.org/zap"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

// ErrDirtyWorktreeCleanup identifies the expected refusal when cleanup reaches
// a checkout that contains local changes without explicit discard consent.
var ErrDirtyWorktreeCleanup = errors.New("worktree cleanup refused because worktree contains local changes")

// Cleanup inspection stages.
const (
	CleanupInspectionStageBranch       = "branch_lookup"
	CleanupInspectionStageRegistration = "registration_inspection"
	CleanupInspectionStageCommit       = "commit_lookup"
	CleanupInspectionStageStatus       = "working_tree_status"
)

// Cleanup inspection reasons.
const (
	CleanupInspectionReasonGitStartFailed        = "git_start_failed"
	CleanupInspectionReasonRepoUnavailable       = "repository_context_unavailable"
	CleanupInspectionReasonCommandFailed         = "git_command_failed"
	CleanupInspectionReasonContextCanceled       = "context_canceled"
	CleanupInspectionReasonDeadlineExceeded      = "deadline_exceeded"
	CleanupInspectionReasonCompetingRegistration = "competing_registration"
	CleanupInspectionReasonUnregisteredPath      = "unregistered_path"
	CleanupInspectionReasonCommitMismatch        = "commit_mismatch"
)

// CleanupInspectionError classifies a failure encountered during worktree cleanup inspection.
type CleanupInspectionError struct {
	Stage  string
	Reason string
	Err    error
}

func (e *CleanupInspectionError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("worktree cleanup inspection failed at %s (%s)", e.Stage, e.Reason)
}

func (e *CleanupInspectionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func classifyCleanupInspectionError(stage string, err error, repoPath ...string) error {
	if err == nil {
		return nil
	}
	var existing *CleanupInspectionError
	if errors.As(err, &existing) {
		return err
	}
	if errors.Is(err, context.Canceled) {
		return &CleanupInspectionError{Stage: stage, Reason: CleanupInspectionReasonContextCanceled, Err: err}
	}
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "timed out") {
		return &CleanupInspectionError{Stage: stage, Reason: CleanupInspectionReasonDeadlineExceeded, Err: err}
	}
	if len(repoPath) > 0 && repoPath[0] != "" {
		if _, statErr := os.Stat(repoPath[0]); errors.Is(statErr, os.ErrNotExist) || os.IsNotExist(statErr) {
			return &CleanupInspectionError{
				Stage:  stage,
				Reason: CleanupInspectionReasonRepoUnavailable,
				Err:    err,
			}
		}
	}
	return &CleanupInspectionError{
		Stage:  stage,
		Reason: classifyGitInspectionReason(err),
		Err:    err,
	}
}

func classifyGitInspectionReason(err error) string {
	if err == nil {
		return ""
	}
	if reason := classifyContextReason(err); reason != "" {
		return reason
	}
	if reason := classifyPathOrExecReason(err); reason != "" {
		return reason
	}
	return classifyExitOrMessageReason(err)
}

func classifyContextReason(err error) string {
	if errors.Is(err, context.Canceled) {
		return CleanupInspectionReasonContextCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "timed out") {
		return CleanupInspectionReasonDeadlineExceeded
	}
	return ""
}

func classifyPathOrExecReason(err error) string {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		if pathErr.Op == "fork/exec" {
			return CleanupInspectionReasonGitStartFailed
		}
		if errors.Is(pathErr.Err, os.ErrNotExist) {
			return CleanupInspectionReasonRepoUnavailable
		}
		return CleanupInspectionReasonGitStartFailed
	}
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		return CleanupInspectionReasonGitStartFailed
	}
	return ""
}

func classifyExitOrMessageReason(err error) string {
	msg := strings.ToLower(err.Error())
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 128 {
		if isRepoUnavailableMessage(msg) {
			return CleanupInspectionReasonRepoUnavailable
		}
		return CleanupInspectionReasonCommandFailed
	}
	if strings.Contains(msg, "executable file not found") || strings.Contains(msg, "cannot run") || strings.Contains(msg, "fork/exec") {
		return CleanupInspectionReasonGitStartFailed
	}
	if isRepoUnavailableMessage(msg) {
		return CleanupInspectionReasonRepoUnavailable
	}
	return CleanupInspectionReasonCommandFailed
}

func isRepoUnavailableMessage(msg string) bool {
	return strings.Contains(msg, "not a git repository") ||
		strings.Contains(msg, "cannot change to") ||
		strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "no such file or directory") ||
		strings.Contains(msg, "not a directory")
}

type worktreeCleanupAudit struct {
	branchRef    string
	branchOID    string
	deleteBranch bool
	pathPresent  bool
	registration worktreeRegistrationOwnership
	pathHandle   storageworkspaces.DirectoryHandle
}

func (m *Manager) auditWorktreeCleanup(
	ctx context.Context, wt *Worktree, removeBranch bool, options WorktreeCleanupOptions,
) (audit worktreeCleanupAudit, err error) {
	if wt == nil || wt.RepositoryPath == "" || wt.Path == "" {
		return worktreeCleanupAudit{}, errors.New("worktree cleanup requires repository and worktree paths")
	}
	if err = m.validateExistingWorktreePathOwner(wt.Path, wt); err != nil {
		return worktreeCleanupAudit{}, err
	}
	pathPresent, err := cleanupPathPresent(wt.Path)
	if err != nil {
		return worktreeCleanupAudit{}, err
	}
	pathHandle, err := m.openCleanupPathHandle(wt.Path, pathPresent)
	if err != nil {
		return worktreeCleanupAudit{}, err
	}
	defer func() {
		if err != nil && pathHandle != nil {
			_ = pathHandle.Close()
		}
	}()
	branchRef, branchOID, err := m.cleanupBranchIdentity(ctx, wt, pathPresent, removeBranch)
	if err != nil {
		return worktreeCleanupAudit{}, err
	}
	registration, err := inspectWorktreeRegistrationOwnershipWithOptions(
		ctx, wt.RepositoryPath, wt.Path, branchRef, branchOID,
		worktreeRegistrationOwnershipOptions{allowAnyBranchAtPath: branchRef == ""},
	)
	if err != nil {
		return worktreeCleanupAudit{}, classifyCleanupInspectionError(CleanupInspectionStageRegistration, fmt.Errorf("inspect worktree cleanup registration: %w", err), wt.RepositoryPath)
	}
	if registration == worktreeRegistrationCompeting {
		return worktreeCleanupAudit{}, &CleanupInspectionError{
			Stage:  CleanupInspectionStageRegistration,
			Reason: CleanupInspectionReasonCompetingRegistration,
			Err:    fmt.Errorf("worktree cleanup target is claimed by unrelated Git metadata: %s", wt.Path),
		}
	}
	if pathPresent && registration != worktreeRegistrationOwned {
		return worktreeCleanupAudit{}, &CleanupInspectionError{
			Stage:  CleanupInspectionStageRegistration,
			Reason: CleanupInspectionReasonUnregisteredPath,
			Err:    fmt.Errorf("worktree cleanup path is not registered to the recorded branch: %s", wt.Path),
		}
	}
	deleteBranch, err := m.auditCleanupBranchDisposition(
		ctx, wt, branchRef, branchOID, pathPresent, removeBranch, options,
	)
	if err != nil {
		return worktreeCleanupAudit{}, err
	}
	return worktreeCleanupAudit{
		branchRef: branchRef, branchOID: branchOID, deleteBranch: deleteBranch,
		pathPresent: pathPresent, registration: registration, pathHandle: pathHandle,
	}, nil
}

func (m *Manager) auditCleanupBranchDisposition(
	ctx context.Context,
	wt *Worktree,
	branchRef string,
	branchOID string,
	pathPresent bool,
	removeBranch bool,
	options WorktreeCleanupOptions,
) (bool, error) {
	if pathPresent && options.RequireCleanCheckout {
		if err := m.verifyCheckoutClean(ctx, wt); err != nil {
			return false, err
		}
	}
	if !removeBranch || branchRef == "" || branchOID == "" {
		return false, nil
	}
	if pathPresent && !options.DiscardWorktreeChanges {
		if options.RequireCleanCheckout {
			return m.verifyCleanupBranchRedundant(ctx, wt, branchRef)
		}
		return m.verifyCleanRedundantCheckout(ctx, wt, branchRef)
	}
	return m.verifyCleanupBranchRedundant(ctx, wt, branchRef)
}

func cleanupPathPresent(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect worktree cleanup path: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false, fmt.Errorf("worktree cleanup path is not a real directory: %s", path)
	}
	return true, nil
}

func (m *Manager) openCleanupPathHandle(
	path string, pathPresent bool,
) (storageworkspaces.DirectoryHandle, error) {
	if !pathPresent {
		return nil, nil
	}
	handle, err := m.openNoFollowWorktreePath(path)
	if err != nil {
		return nil, err
	}
	if handle == nil {
		return nil, fmt.Errorf("worktree cleanup path disappeared during audit: %s", path)
	}
	return handle, nil
}

func (m *Manager) cleanupBranchIdentity(
	ctx context.Context, wt *Worktree, pathPresent, requireImmutableIdentity bool,
) (string, string, error) {
	branchRef := ""
	if wt.Branch != "" {
		branchRef = "refs/heads/" + wt.Branch
	}
	expectedOID := strings.TrimSpace(wt.CleanupHeadOID)
	if wt.CleanupHeadOIDUnavailable && pathPresent {
		return "", "", &CleanupInspectionError{
			Stage:  CleanupInspectionStageCommit,
			Reason: CleanupInspectionReasonCommitMismatch,
			Err:    fmt.Errorf("cleanup worktree path %q reappeared without immutable expected commit", wt.Path),
		}
	}
	if expectedOID == "" && pathPresent {
		output, err := m.runBoundedGitInspect(ctx, wt.Path, "rev-parse", "--verify", "HEAD^{commit}")
		if err != nil {
			return "", "", classifyCleanupInspectionError(CleanupInspectionStageCommit, fmt.Errorf("capture cleanup worktree HEAD: %w", err), wt.Path)
		}
		expectedOID = strings.TrimSpace(output)
		if expectedOID == "" {
			return "", "", &CleanupInspectionError{
				Stage:  CleanupInspectionStageCommit,
				Reason: CleanupInspectionReasonCommitMismatch,
				Err:    errors.New("capture cleanup worktree HEAD returned an empty commit"),
			}
		}
	}
	if branchRef == "" {
		return branchRef, expectedOID, nil
	}
	exists, err := m.branchExists(ctx, wt.RepositoryPath, branchRef)
	if err != nil {
		return "", "", classifyCleanupInspectionError(CleanupInspectionStageBranch, fmt.Errorf("verify cleanup branch %q: %w", wt.Branch, err), wt.RepositoryPath)
	}
	if !exists {
		// The local branch is already gone. A stale registration, if any, must
		// not be adopted without a live ref identity, so the strict ownership
		// classifier will fail closed rather than deleting another checkout.
		return branchRef, "", nil
	}
	if expectedOID == "" && requireImmutableIdentity {
		return "", "", &CleanupInspectionError{
			Stage:  CleanupInspectionStageCommit,
			Reason: CleanupInspectionReasonCommitMismatch,
			Err:    fmt.Errorf("cleanup branch %q has no immutable expected commit", wt.Branch),
		}
	}
	output, err := m.runBoundedGitInspect(ctx, wt.RepositoryPath, "rev-parse", "--verify", branchRef+"^{commit}")
	if err != nil {
		return "", "", classifyCleanupInspectionError(CleanupInspectionStageCommit, fmt.Errorf("resolve cleanup branch %q: %w", wt.Branch, err), wt.RepositoryPath)
	}
	branchOID := strings.TrimSpace(output)
	if expectedOID != "" && branchOID != expectedOID {
		return branchRef, "", &CleanupInspectionError{
			Stage:  CleanupInspectionStageCommit,
			Reason: CleanupInspectionReasonCommitMismatch,
			Err:    fmt.Errorf("cleanup branch %q advanced from audited commit %s to %s", wt.Branch, expectedOID, branchOID),
		}
	}
	return branchRef, branchOID, nil
}

func (m *Manager) verifyCleanRedundantCheckout(
	ctx context.Context, wt *Worktree, branchRef string,
) (bool, error) {
	if err := m.verifyCheckoutClean(ctx, wt); err != nil {
		return false, err
	}
	return m.verifyCleanupBranchRedundant(ctx, wt, branchRef)
}

func (m *Manager) verifyCheckoutClean(ctx context.Context, wt *Worktree) error {
	status, err := m.runBoundedGitInspect(ctx, wt.Path, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return fmt.Errorf("inspect worktree changes before cleanup: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		return fmt.Errorf("%w: %q contains uncommitted or untracked work", ErrDirtyWorktreeCleanup, wt.Path)
	}
	return nil
}

func (m *Manager) verifyCleanupBranchRedundant(
	ctx context.Context, wt *Worktree, branchRef string,
) (bool, error) {
	base := strings.TrimPrefix(wt.BaseBranch, "origin/")
	candidates := []string{"refs/remotes/origin/HEAD"}
	if base != "" && base != wt.Branch {
		candidates = append([]string{"refs/remotes/origin/" + base, "refs/heads/" + base}, candidates...)
	}
	if base == "" {
		// Session metadata can be gone by the time a durable delete job runs.
		// Keep the offline fallback limited to conventional named branches.
		candidates = append(candidates, "refs/heads/main", "refs/heads/master")
	}
	// A local HEAD is a valid fallback only when it is attached to a named
	// branch. A detached HEAD can point at an arbitrary commit and must never
	// be treated as proof that the task branch is merged.
	if headRef, err := m.runBoundedGitInspect(ctx, wt.RepositoryPath, "symbolic-ref", "--quiet", "--verify", "HEAD"); err == nil {
		headRef = strings.TrimSpace(headRef)
		if strings.HasPrefix(headRef, "refs/heads/") && headRef != "refs/heads/"+wt.Branch {
			candidates = append(candidates, headRef)
		}
	}
	foundBase := false
	for _, candidate := range candidates {
		exists, err := m.branchExists(ctx, wt.RepositoryPath, candidate)
		if err != nil {
			return false, classifyCleanupInspectionError(CleanupInspectionStageBranch, fmt.Errorf("verify cleanup base %q: %w", candidate, err), wt.RepositoryPath)
		}
		if !exists {
			continue
		}
		foundBase = true
		contains, err := m.refContains(ctx, wt.RepositoryPath, candidate, branchRef)
		if err != nil {
			return false, classifyCleanupInspectionError(CleanupInspectionStageCommit, fmt.Errorf("verify cleanup branch ancestry: %w", err), wt.RepositoryPath)
		}
		if contains {
			return true, nil
		}
	}
	if !foundBase {
		return false, fmt.Errorf("worktree cleanup cannot resolve a containing base for branch %q", wt.Branch)
	}
	return false, nil
}

func (m *Manager) completeAuditedWorktreeCleanup(
	ctx context.Context, wt *Worktree, audit worktreeCleanupAudit, removeBranch bool,
) error {
	if audit.pathHandle != nil {
		defer func() { _ = audit.pathHandle.Close() }()
	}
	switch audit.registration {
	case worktreeRegistrationOwned:
		if err := m.completeOwnedWorktreeCleanup(ctx, wt, audit); err != nil {
			return err
		}
	case worktreeRegistrationAbsent:
		if audit.pathPresent {
			return fmt.Errorf("refusing to remove unregistered worktree path: %s", wt.Path)
		}
		m.tryRemoveEmptyTaskDir(wt.Path)
	default:
		return fmt.Errorf("refusing to remove competing worktree registration: %s", wt.Path)
	}
	return m.deleteAuditedCleanupBranch(ctx, wt, audit, removeBranch)
}

func (m *Manager) completeOwnedWorktreeCleanup(
	ctx context.Context, wt *Worktree, audit worktreeCleanupAudit,
) error {
	if audit.pathHandle != nil {
		if err := audit.pathHandle.VerifyPath(wt.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("worktree path changed after audit: %w", err)
		}
	}
	if err := m.removeWorktreeDir(ctx, wt.Path, wt.RepositoryPath, audit.pathHandle); err != nil {
		return fmt.Errorf("remove owned worktree path: %w", err)
	}
	registration, err := inspectWorktreeRegistrationOwnershipWithOptions(
		ctx, wt.RepositoryPath, wt.Path, audit.branchRef, audit.branchOID,
		worktreeRegistrationOwnershipOptions{allowAnyBranchAtPath: audit.branchRef == ""},
	)
	if err != nil {
		return fmt.Errorf("verify removed worktree registration: %w", err)
	}
	if registration != worktreeRegistrationAbsent {
		return fmt.Errorf("worktree registration remains after cleanup: %s", wt.Path)
	}
	return nil
}

func (m *Manager) deleteAuditedCleanupBranch(
	ctx context.Context, wt *Worktree, audit worktreeCleanupAudit, removeBranch bool,
) error {
	if !removeBranch || audit.branchOID == "" {
		return nil
	}
	if !audit.deleteBranch {
		m.logger.Info("preserved cleanup branch with unique commits",
			zap.String("branch", wt.Branch), zap.String("repository_path", wt.RepositoryPath))
		return nil
	}
	cmd := newGitCommand(ctx, "update-ref", "-d", audit.branchRef, audit.branchOID)
	cmd.Dir = wt.RepositoryPath
	if output, err := runGitCmdCombinedOutput(ctx, cmd); err != nil {
		return fmt.Errorf("delete verified cleanup branch %q: %s: %w",
			wt.Branch, strings.TrimSpace(string(output)), err)
	}
	exists, err := m.branchExists(ctx, wt.RepositoryPath, audit.branchRef)
	if err != nil {
		return fmt.Errorf("verify deleted cleanup branch %q: %w", wt.Branch, err)
	}
	if exists {
		return fmt.Errorf("cleanup branch %q remains after deletion", wt.Branch)
	}
	return nil
}
