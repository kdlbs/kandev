package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
)

const replacementBranchAttempts = 20

// replaceUnrecoverableWorktree creates a fresh branch and worktree while
// retaining the existing task-environment repository record. The caller holds
// the repository lock from recreate, so this helper must not acquire it again.
func (m *Manager) replaceUnrecoverableWorktree(
	ctx context.Context,
	existing *Worktree,
	req CreateRequest,
	branchErr *BranchUnrecoverableError,
) (*Worktree, error) {
	if existing == nil {
		return nil, branchErr
	}
	replacementReq, err := replacementRequest(existing, req)
	if err != nil {
		return nil, err
	}
	baseRef, fallbackWarning, fallbackDetail, err := m.resolveBaseRefWithFallback(ctx, &replacementReq)
	if err != nil {
		return nil, fmt.Errorf("resolve replacement base branch: %w", err)
	}
	branchName, worktreePath, err := m.replacementTarget(ctx, replacementReq)
	if err != nil {
		return nil, err
	}
	if _, err := m.gitAddWorktree(ctx, replacementReq.RepositoryPath, branchName, worktreePath, baseRef); err != nil {
		return nil, fmt.Errorf("create replacement worktree for %q: %w", branchErr.BranchName(), err)
	}

	replacement := replacementRecord(existing, replacementReq, branchName, worktreePath, fallbackWarning, fallbackDetail)
	if err := m.persistReplacementWorktree(ctx, &replacement); err != nil {
		if cleanupErr := m.cleanupFailedReplacement(ctx, &replacement); cleanupErr != nil {
			return nil, errors.Join(err, fmt.Errorf("cleanup replacement after persistence failure: %w", cleanupErr))
		}
		return nil, err
	}
	m.copyConfiguredFiles(ctx, replacementReq, &replacement)
	m.runWorktreeSetupScript(ctx, &replacement, replacementReq.ScriptEnv)
	m.logger.Info("replaced unrecoverable worktree branch",
		zap.String("session_id", replacement.SessionID),
		zap.String("task_id", replacement.TaskID),
		zap.String("original_branch", branchErr.BranchName()),
		zap.String("new_branch", replacement.Branch),
		zap.String("path", replacement.Path))
	return &replacement, nil
}

// cleanupFailedReplacement removes the physical checkout and the branch that
// gitAddWorktree created before the replacement record could be persisted. The
// original record remains authoritative when this compensation runs.
func (m *Manager) cleanupFailedReplacement(ctx context.Context, replacement *Worktree) error {
	if replacement == nil {
		return nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), m.inspectTimeout)
	defer cancel()

	var cleanupErrs []error
	if err := m.removeWorktreeDir(cleanupCtx, replacement.Path, replacement.RepositoryPath); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("remove replacement worktree: %w", err))
	}
	if err := m.cleanupFailedReplacementBranch(cleanupCtx, replacement); err != nil {
		cleanupErrs = append(cleanupErrs, err)
	}
	return errors.Join(cleanupErrs...)
}

func (m *Manager) cleanupFailedReplacementBranch(ctx context.Context, replacement *Worktree) error {
	branchRef := "refs/heads/" + replacement.Branch
	exists, err := m.branchExists(ctx, replacement.RepositoryPath, branchRef)
	if err != nil {
		return fmt.Errorf("verify replacement branch: %w", err)
	}
	if !exists {
		return nil
	}
	output, err := m.runBoundedGitInspect(
		ctx,
		replacement.RepositoryPath,
		"rev-parse",
		"--verify",
		branchRef+"^{commit}",
	)
	if err != nil {
		return fmt.Errorf("resolve replacement branch: %w", err)
	}
	_, err = deleteBranchRefIfOwned(ctx, replacement.RepositoryPath, newBranchAddSnapshot{
		branchRef: branchRef,
		branchOID: strings.TrimSpace(output),
	})
	if err != nil {
		return fmt.Errorf("delete replacement branch: %w", err)
	}
	return nil
}

func replacementRequest(existing *Worktree, req CreateRequest) (CreateRequest, error) {
	replacementReq := req
	if replacementReq.BaseBranch == "" {
		replacementReq.BaseBranch = existing.BaseBranch
	}
	if replacementReq.TaskDirName == "" {
		replacementReq.TaskDirName = existing.TaskDirName
	}
	if replacementReq.TaskEnvironmentID == "" {
		replacementReq.TaskEnvironmentID = existing.TaskEnvironmentID
	}
	if replacementReq.BaseBranch == "" || replacementReq.TaskDirName == "" || replacementReq.RepoName == "" {
		return CreateRequest{}, fmt.Errorf("replace unrecoverable worktree: %w", ErrTaskDirRequired)
	}
	return replacementReq, nil
}

func replacementRecord(
	existing *Worktree,
	req CreateRequest,
	branchName, worktreePath, fallbackWarning, fallbackDetail string,
) Worktree {
	identitySlug := existing.BranchSlug
	if identitySlug == "" {
		identitySlug = requestBranchIdentitySlug(req)
	}
	replacement := *existing
	replacement.TaskDirName = req.TaskDirName
	replacement.TaskEnvironmentID = req.TaskEnvironmentID
	replacement.BranchSlug = identitySlug
	replacement.RepositoryPath = req.RepositoryPath
	replacement.Path = worktreePath
	replacement.Branch = branchName
	replacement.BaseBranch = req.BaseBranch
	replacement.Status = StatusActive
	replacement.DeletedAt = nil
	replacement.FetchWarning = ""
	replacement.FetchWarningDetail = ""
	replacement.BaseBranchFallbackWarning = fallbackWarning
	replacement.BaseBranchFallbackDetail = fallbackDetail
	replacement.UpdatedAt = time.Now()
	if replacement.SessionID == "" {
		replacement.SessionID = req.SessionID
	}
	if replacement.TaskID == "" {
		replacement.TaskID = req.TaskID
	}
	return replacement
}

func (m *Manager) persistReplacementWorktree(ctx context.Context, replacement *Worktree) error {
	if m.store != nil {
		if err := m.store.UpdateWorktree(ctx, replacement); err != nil {
			return fmt.Errorf("persist replacement worktree: %w", err)
		}
	}
	if replacement.SessionID != "" {
		m.mu.Lock()
		m.worktrees[cacheKey(replacement.SessionID, replacement.RepositoryID, replacement.BranchSlug)] = replacement
		m.mu.Unlock()
	}
	return nil
}

func (m *Manager) replacementTarget(ctx context.Context, req CreateRequest) (string, string, error) {
	_, branchBase := m.buildWorktreeNames(req)
	for attempt := 0; attempt < replacementBranchAttempts; attempt++ {
		branchName := replacementBranchName(branchBase, attempt)
		exists, err := m.branchExists(ctx, req.RepositoryPath, branchName)
		if err != nil {
			return "", "", fmt.Errorf("check replacement branch %q: %w", branchName, err)
		}
		if exists {
			continue
		}
		path, err := m.replacementPath(req, branchName)
		if err != nil {
			return "", "", err
		}
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			return branchName, path, nil
		} else if err != nil {
			return "", "", fmt.Errorf("inspect replacement worktree path: %w", err)
		}
	}
	return "", "", fmt.Errorf("unable to find a unique replacement branch after %d attempts: %w", replacementBranchAttempts, ErrBranchExists)
}

func replacementBranchName(branchBase string, attempt int) string {
	if attempt == 0 {
		return branchBase
	}
	return branchBase + "-" + SmallSuffix(3)
}

func (m *Manager) replacementPath(req CreateRequest, branchName string) (string, error) {
	pathReq := req
	pathReq.BranchSlug = SanitizeBranchSlug(branchName)
	pathReq.BranchIdentitySlug = requestBranchIdentitySlug(req)
	path, err := m.prepareTaskWorktreePath(pathReq)
	if err != nil {
		return "", fmt.Errorf("prepare replacement worktree path: %w", err)
	}
	return path, nil
}
