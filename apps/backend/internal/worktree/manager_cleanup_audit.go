package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
)

type worktreeCleanupAudit struct {
	branchRef    string
	branchOID    string
	deleteBranch bool
	pathPresent  bool
	registration worktreeRegistrationOwnership
}

func (m *Manager) auditWorktreeCleanup(
	ctx context.Context, wt *Worktree, removeBranch bool,
) (worktreeCleanupAudit, error) {
	if wt == nil || wt.RepositoryPath == "" || wt.Path == "" {
		return worktreeCleanupAudit{}, errors.New("worktree cleanup requires repository and worktree paths")
	}
	if err := m.validateExistingWorktreePathOwner(wt.Path, wt); err != nil {
		return worktreeCleanupAudit{}, err
	}
	pathPresent, err := cleanupPathPresent(wt.Path)
	if err != nil {
		return worktreeCleanupAudit{}, err
	}
	branchRef, branchOID, err := m.cleanupBranchIdentity(ctx, wt)
	if err != nil {
		return worktreeCleanupAudit{}, err
	}
	registration, err := inspectCleanupRegistrationOwnership(
		ctx, wt.RepositoryPath, wt.Path, branchRef, branchOID,
	)
	if err != nil {
		return worktreeCleanupAudit{}, fmt.Errorf("inspect worktree cleanup registration: %w", err)
	}
	if registration == worktreeRegistrationCompeting {
		return worktreeCleanupAudit{}, fmt.Errorf("worktree cleanup target is claimed by unrelated Git metadata: %s", wt.Path)
	}
	if pathPresent && registration != worktreeRegistrationOwned {
		return worktreeCleanupAudit{}, fmt.Errorf("worktree cleanup path is not registered to the recorded branch: %s", wt.Path)
	}
	deleteBranch, err := m.auditCleanupBranchDisposition(
		ctx, wt, branchRef, branchOID, pathPresent, removeBranch,
	)
	if err != nil {
		return worktreeCleanupAudit{}, err
	}
	return worktreeCleanupAudit{
		branchRef: branchRef, branchOID: branchOID, deleteBranch: deleteBranch,
		pathPresent: pathPresent, registration: registration,
	}, nil
}

func (m *Manager) auditCleanupBranchDisposition(
	ctx context.Context,
	wt *Worktree,
	branchRef string,
	branchOID string,
	pathPresent bool,
	removeBranch bool,
) (bool, error) {
	if !removeBranch || branchOID == "" {
		return false, nil
	}
	if pathPresent {
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

func (m *Manager) cleanupBranchIdentity(ctx context.Context, wt *Worktree) (string, string, error) {
	if wt.Branch == "" {
		return "", "", nil
	}
	branchRef := "refs/heads/" + wt.Branch
	exists, err := m.branchExists(ctx, wt.RepositoryPath, branchRef)
	if err != nil {
		return "", "", fmt.Errorf("verify cleanup branch %q: %w", wt.Branch, err)
	}
	if !exists {
		return branchRef, "", nil
	}
	output, err := m.runBoundedGitInspect(ctx, wt.RepositoryPath, "rev-parse", "--verify", branchRef+"^{commit}")
	if err != nil {
		return "", "", fmt.Errorf("resolve cleanup branch %q: %w", wt.Branch, err)
	}
	return branchRef, strings.TrimSpace(output), nil
}

func inspectCleanupRegistrationOwnership(
	ctx context.Context, repoPath, worktreePath, branchRef, branchOID string,
) (worktreeRegistrationOwnership, error) {
	cmd := newGitCommand(ctx, "worktree", "list", "--porcelain", "-z")
	cmd.Dir = repoPath
	output, err := runGitCmdCombinedOutput(ctx, cmd)
	if err != nil {
		return worktreeRegistrationAbsent, err
	}
	wantPath, err := normalizedWorktreeTargetPath(worktreePath)
	if err != nil {
		return worktreeRegistrationAbsent, err
	}
	return classifyCleanupRegistrationOwnership(
		parseWorktreeRegistrations(string(output)), wantPath, branchRef, branchOID,
	)
}

func classifyCleanupRegistrationOwnership(
	registrations []worktreeRegistration, wantPath, branchRef, branchOID string,
) (worktreeRegistrationOwnership, error) {
	owned := false
	for _, registration := range registrations {
		currentPath, err := normalizedWorktreeTargetPath(registration.path)
		if err != nil {
			return worktreeRegistrationAbsent, err
		}
		if currentPath != wantPath {
			if branchRef != "" && registration.branch == branchRef {
				return worktreeRegistrationCompeting, nil
			}
			continue
		}
		matchesOID := branchOID == "" || registration.head == branchOID
		if registration.branch != branchRef || !matchesOID || owned {
			return worktreeRegistrationCompeting, nil
		}
		owned = true
	}
	if owned {
		return worktreeRegistrationOwned, nil
	}
	return worktreeRegistrationAbsent, nil
}

func (m *Manager) verifyCleanRedundantCheckout(
	ctx context.Context, wt *Worktree, branchRef string,
) (bool, error) {
	status, err := m.runBoundedGitInspect(ctx, wt.Path, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return false, fmt.Errorf("inspect worktree changes before cleanup: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		return false, fmt.Errorf("worktree cleanup refused because %q contains uncommitted or untracked work", wt.Path)
	}
	return m.verifyCleanupBranchRedundant(ctx, wt, branchRef)
}

func (m *Manager) verifyCleanupBranchRedundant(
	ctx context.Context, wt *Worktree, branchRef string,
) (bool, error) {
	base := strings.TrimPrefix(wt.BaseBranch, "origin/")
	candidates := []string{"refs/remotes/origin/HEAD", "HEAD"}
	if base != "" && base != wt.Branch {
		candidates = append([]string{"refs/remotes/origin/" + base, "refs/heads/" + base}, candidates...)
	}
	foundBase := false
	for _, candidate := range candidates {
		exists, err := m.branchExists(ctx, wt.RepositoryPath, candidate)
		if err != nil {
			return false, fmt.Errorf("verify cleanup base %q: %w", candidate, err)
		}
		if !exists {
			continue
		}
		foundBase = true
		contains, err := m.refContains(ctx, wt.RepositoryPath, candidate, branchRef)
		if err != nil {
			return false, fmt.Errorf("verify cleanup branch ancestry: %w", err)
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
	switch audit.registration {
	case worktreeRegistrationOwned:
		if err := m.removeWorktreeDir(ctx, wt.Path, wt.RepositoryPath); err != nil {
			return fmt.Errorf("remove owned worktree path: %w", err)
		}
		registration, err := inspectCleanupRegistrationOwnership(
			ctx, wt.RepositoryPath, wt.Path, audit.branchRef, audit.branchOID,
		)
		if err != nil {
			return fmt.Errorf("verify removed worktree registration: %w", err)
		}
		if registration != worktreeRegistrationAbsent {
			return fmt.Errorf("worktree registration remains after cleanup: %s", wt.Path)
		}
	case worktreeRegistrationAbsent:
		if audit.pathPresent {
			return fmt.Errorf("refusing to remove unregistered worktree path: %s", wt.Path)
		}
		m.tryRemoveEmptyTaskDir(wt.Path)
	default:
		return fmt.Errorf("refusing to remove competing worktree registration: %s", wt.Path)
	}
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
