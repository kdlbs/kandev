package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

// tryReuseRetainedTaskCheckout handles only the already assigned task path.
// Incompatible or unreadable occupants remain untouched for explicit recovery.
func (m *Manager) tryReuseRetainedTaskCheckout(ctx context.Context, req CreateRequest) (*Worktree, bool, error) {
	if req.TaskDirName == "" || req.RepoName == "" || req.RemoteContribution != nil || req.PRNumber > 0 {
		return nil, false, nil
	}
	root, path, err := m.retainedTaskCheckoutPath(req)
	if err != nil {
		return nil, true, err
	}
	if _, err = os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	release, err := acquireWorktreeTargetPath(ctx, path)
	if err != nil {
		return nil, true, err
	}
	defer release()
	handle, err := m.validateWorktreePathSafe(path)
	if err != nil {
		return nil, true, err
	}
	if handle != nil {
		defer func() { _ = handle.Close() }()
	}
	if err = m.validateRetainedTaskCheckout(root, path, req); err != nil {
		return nil, true, err
	}
	return m.reuseRetainedCheckoutAtPath(ctx, req, path)
}

func (m *Manager) reuseRetainedCheckoutAtPath(ctx context.Context, req CreateRequest, path string) (*Worktree, bool, error) {
	existing, err := m.retainedCheckoutRecord(ctx, req, path)
	if err != nil {
		return nil, true, err
	}
	expected := req.CheckoutBranch
	if existing != nil {
		if expected != "" && expected != existing.Branch {
			return nil, true, retainedCheckoutError(req.TaskID, path, "requested branch differs from the registered checkout")
		}
		expected = existing.Branch
	}
	if expected == "" {
		return nil, true, retainedCheckoutError(req.TaskID, path, "branch identity is unavailable for an unregistered checkout")
	}
	if err = m.validateRetainedCheckoutBranch(ctx, req.TaskID, path, expected); err != nil {
		return nil, true, err
	}
	if existing != nil {
		return existing, true, nil
	}
	wt := m.buildWorktreeRecord(uuid.New().String(), req, path, expected)
	wt.BranchOwner = createdBranchOwner(req.CheckoutBranch, expected)
	if err = m.registerRetainedCheckout(ctx, req, wt); err != nil {
		return nil, true, err
	}
	return wt, true, nil
}

func (m *Manager) retainedTaskCheckoutPath(req CreateRequest) (root, path string, err error) {
	if req.WorkspaceRelativePath != "" {
		relative, validateErr := validateTaskRelativeWorktreeRequest(req)
		if validateErr != nil {
			return "", "", validateErr
		}
		return m.resolveTaskRelativeWorktreePath(req.TaskDirName, relative)
	}
	repoDir := SanitizeRepoDirName(req.RepoName)
	if repoDir == "" {
		return "", "", ErrInvalidRepoName
	}
	path, err = m.config.TaskWorktreePath(req.TaskDirName, repoDir, SanitizeBranchSlug(req.BranchSlug))
	if err != nil {
		return "", "", err
	}
	return filepath.Dir(path), path, nil
}

func (m *Manager) validateRetainedTaskCheckout(root, path string, req CreateRequest) error {
	owner, found, err := storageworkspaces.ReadOwnershipMarker(root)
	if err != nil {
		return retainedCheckoutError(req.TaskID, path, "task ownership metadata is unavailable")
	}
	if found && ((owner.TaskID != "" && owner.TaskID != req.TaskID) || (owner.TaskDirName != "" && owner.TaskDirName != req.TaskDirName)) {
		return fmt.Errorf("%w: assigned task directory belongs to another task", ErrWorktreePathOwnedByAnotherTask)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return retainedCheckoutError(req.TaskID, path, "assigned checkout is unavailable")
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return retainedCheckoutError(req.TaskID, path, "assigned checkout is not a plain directory")
	}
	if inspectLinkedWorktree(path).class != linkedWorktreeHealthy {
		return retainedCheckoutError(req.TaskID, path, "assigned checkout is not a healthy linked worktree; main checkout relocation is not permitted")
	}
	matches, resolved := checkoutIdentityMatches(path, req.RepositoryPath)
	if !resolved {
		return retainedCheckoutError(req.TaskID, path, "repository identity is unavailable")
	}
	if !matches {
		return retainedCheckoutError(req.TaskID, path, "assigned checkout belongs to a different repository")
	}
	return nil
}

// retainedCheckoutRecord preserves existing row IDs. A conflicting registered
// slot at the assigned path is not treated as missing inventory.
func (m *Manager) retainedCheckoutRecord(ctx context.Context, req CreateRequest, path string) (*Worktree, error) {
	if m.store == nil {
		return nil, nil
	}
	rows, err := m.store.GetWorktreesByTaskID(ctx, req.TaskID)
	if err != nil {
		return nil, retainedCheckoutError(req.TaskID, path, "checkout registration is unavailable")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, retainedCheckoutError(req.TaskID, path, "assigned checkout is unavailable")
	}
	var existing *Worktree
	for _, row := range rows {
		if row == nil {
			continue
		}
		if row.Path == "" {
			if row.ID != "" && retainedCheckoutSlotMatches(row, req) {
				return nil, retainedCheckoutError(req.TaskID, path, "registered repository slot has no checkout path")
			}
			continue
		}
		other, statErr := os.Stat(row.Path)
		if statErr != nil || !os.SameFile(info, other) {
			if retainedCheckoutSlotMatches(row, req) {
				return nil, retainedCheckoutError(req.TaskID, path, "repository slot references another or unavailable checkout")
			}
			continue
		}
		if existing != nil {
			return nil, retainedCheckoutError(req.TaskID, path, "multiple registrations claim the assigned checkout")
		}
		if !retainedCheckoutRecordMatches(row, req) {
			return nil, retainedCheckoutError(req.TaskID, path, "assigned checkout registration does not match the requested task and repository slot")
		}
		existing = row
	}
	return existing, nil
}

// Use normal registration admission without the fresh-checkout cleanup wrapper:
// a failed write must not remove files that predate this request.
func (m *Manager) registerRetainedCheckout(ctx context.Context, req CreateRequest, wt *Worktree) error {
	if m.store != nil && req.SessionID != "" {
		if err := m.store.CreateWorktree(ctx, wt); err != nil && !errors.Is(err, ErrEnvironmentNotResolved) {
			return fmt.Errorf("register retained checkout without changing its files: %w", err)
		}
	}
	if req.SessionID != "" {
		m.mu.Lock()
		m.worktrees[cacheKey(req.SessionID, req.RepositoryID, wt.BranchSlug)] = wt
		m.mu.Unlock()
	}
	return nil
}

func retainedCheckoutError(taskID, path, reason string) error {
	return &WorktreeRecoveryError{TaskID: taskID, Checkout: path, State: "retained_checkout", Reason: reason}
}

func (m *Manager) validateRetainedCheckoutBranch(ctx context.Context, taskID, path, expected string) error {
	branch, err := m.runBoundedGitInspect(ctx, path, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return retainedCheckoutError(taskID, path, "checkout branch is unavailable")
	}
	if strings.TrimSpace(branch) != expected {
		return retainedCheckoutError(taskID, path, "checkout does not hold the requested branch")
	}
	return nil
}

func retainedCheckoutRecordMatches(row *Worktree, req CreateRequest) bool {
	return row.TaskID == req.TaskID && retainedCheckoutSlotMatches(row, req) &&
		(req.TaskEnvironmentID == "" || row.TaskEnvironmentID == req.TaskEnvironmentID) &&
		(row.Status == "" || row.Status == StatusActive)
}

func retainedCheckoutSlotMatches(row *Worktree, req CreateRequest) bool {
	return row.RepositoryID == req.RepositoryID &&
		SanitizeBranchSlug(row.BranchSlug) == requestBranchIdentitySlug(req)
}
