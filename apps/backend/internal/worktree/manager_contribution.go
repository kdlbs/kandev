package worktree

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

func contributionRemoteName(binding *models.RemoteContribution) string {
	if binding == nil {
		return ""
	}
	return binding.ContributionRemoteName()
}

func (m *Manager) ensureContributionRemote(
	ctx context.Context, repoPath, remoteName, remoteURL string,
) error {
	getURL := m.newNonInteractiveGitCmd(ctx, repoPath, "config", "--get", "remote."+remoteName+".url")
	configuredURL, getErr := runGitCmdOutput(ctx, getURL)
	if getErr == nil {
		if strings.TrimSpace(string(configuredURL)) != remoteURL {
			return fmt.Errorf("contribution remote %q is already configured for another URL", remoteName)
		}
		return nil
	}

	add := m.newNonInteractiveGitCmd(ctx, repoPath, "remote", "add", remoteName, remoteURL)
	output, addErr := runGitCmdCombinedOutput(ctx, add)
	if addErr == nil {
		return nil
	}

	// Another materialization can add the same remote between the config read
	// and this command. Re-read after a failed add and accept the result only
	// when it has the exact binding URL.
	readAfterAdd := m.newNonInteractiveGitCmd(ctx, repoPath, "config", "--get", "remote."+remoteName+".url")
	configuredAfterAdd, readErr := runGitCmdOutput(ctx, readAfterAdd)
	if readErr == nil && strings.TrimSpace(string(configuredAfterAdd)) == remoteURL {
		return nil
	}
	if readErr == nil {
		return fmt.Errorf("contribution remote %q is already configured for another URL", remoteName)
	}
	return fmt.Errorf("add contribution remote: %s: %w", strings.TrimSpace(string(output)), addErr)
}

func (m *Manager) materializeRemoteContribution(ctx context.Context, repoPath string, binding *models.RemoteContribution) (string, string, error) {
	if binding == nil {
		return "", "", errors.New("remote contribution binding is required")
	}
	if err := binding.Validate(); err != nil {
		return "", "", fmt.Errorf("validate remote contribution: %w", err)
	}
	remoteName := contributionRemoteName(binding)
	if err := m.ensureContributionRemote(ctx, repoPath, remoteName, binding.SourceRepository.RemoteURL); err != nil {
		return "", "", err
	}
	remoteRef := "refs/remotes/" + remoteName + "/" + binding.HeadBranch
	refspec := "+refs/heads/" + binding.HeadBranch + ":" + remoteRef
	output, err, execCtxErr := m.runGitCombinedAfterAcquire(ctx, m.fetchTimeout, repoPath, "fetch", gitNoTags, remoteName, refspec)
	if err != nil {
		if execCtxErr != nil {
			return "", "", fmt.Errorf("fetch contribution branch: %w", execCtxErr)
		}
		return "", "", fmt.Errorf("fetch contribution branch: %s: %w", strings.TrimSpace(string(output)), err)
	}
	verify := m.newNonInteractiveGitCmd(ctx, repoPath, "rev-parse", "--verify", remoteRef)
	resolved, err := runGitCmdOutput(ctx, verify)
	if err != nil {
		return "", "", fmt.Errorf("verify fetched contribution branch: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(string(resolved)), binding.HeadSHA) {
		return "", "", fmt.Errorf("contribution branch head changed: fetched %s, expected %s", strings.TrimSpace(string(resolved)), binding.HeadSHA)
	}
	return remoteName, remoteRef, nil
}

func (m *Manager) configureContributionDestination(
	ctx context.Context,
	repoPath, worktreePath, branch string,
	destination *models.ContributionDestination,
) error {
	if destination == nil {
		return nil
	}
	if err := destination.Validate(); err != nil {
		return fmt.Errorf("validate contribution destination: %w", err)
	}
	remoteName := destination.ContributionRemoteName()
	if err := m.ensureContributionRemote(ctx, repoPath, remoteName, destination.TargetRepository.RemoteURL); err != nil {
		return fmt.Errorf("configure contribution destination remote: %w", err)
	}
	pushURL := m.newNonInteractiveGitCmd(ctx, repoPath, "config", "--get-all", "remote."+remoteName+".pushurl")
	configured, readErr := runGitCmdOutput(ctx, pushURL)
	if readErr == nil && strings.TrimSpace(string(configured)) != destination.TargetRepository.RemoteURL {
		return fmt.Errorf("contribution destination push URL does not match the validated target")
	}
	if readErr != nil {
		setPushURL := m.newNonInteractiveGitCmd(ctx, repoPath, "config", "--add", "remote."+remoteName+".pushurl", destination.TargetRepository.RemoteURL)
		if err := runGitCmd(ctx, setPushURL); err != nil {
			return fmt.Errorf("set contribution destination push URL: %w", err)
		}
	}
	if branch == "" {
		return nil
	}
	setPushRemote := m.newNonInteractiveGitCmd(ctx, worktreePath, "config", "branch."+branch+".pushRemote", remoteName)
	if err := runGitCmd(ctx, setPushRemote); err != nil {
		return fmt.Errorf("set contribution destination push remote: %w", err)
	}
	return nil
}

func (m *Manager) validateContributionAncestor(ctx context.Context, repoPath, expectedSHA, descendantRef string) error {
	cmd := m.newNonInteractiveGitCmd(ctx, repoPath, "merge-base", "--is-ancestor", expectedSHA, descendantRef)
	if err := runGitCmd(ctx, cmd); err != nil {
		return err
	}
	return nil
}

func (m *Manager) addContributionWorktree(
	ctx context.Context, req CreateRequest, worktreePath, startPoint, remoteName string,
) (string, string, error) {
	branchName := req.CheckoutBranch
	exists, err := m.branchExists(ctx, req.RepositoryPath, "refs/heads/"+branchName)
	if err != nil {
		return "", "", err
	}
	if exists {
		branchName = branchName + "-" + SmallSuffix(3)
	}
	id, err := m.gitAddWorktree(ctx, req.RepositoryPath, branchName, worktreePath, startPoint)
	if err != nil && errors.Is(err, ErrBranchCheckedOut) {
		branchName = req.CheckoutBranch + "-" + SmallSuffix(3)
		id, err = m.gitAddWorktree(ctx, req.RepositoryPath, branchName, worktreePath, startPoint)
	}
	if err != nil {
		return "", "", err
	}
	if err := m.setUpstreamIfExistsRemote(ctx, worktreePath, branchName, remoteName, req.CheckoutBranch); err != nil {
		_ = m.removeWorktreeDir(ctx, worktreePath, req.RepositoryPath)
		return "", "", err
	}
	return id, branchName, nil
}

func (m *Manager) setUpstreamIfExistsRemote(ctx context.Context, worktreePath, localBranch, remoteName, remoteBranch string) error {
	upstream := remoteName + "/" + remoteBranch
	verifyCmd := m.newNonInteractiveGitCmd(ctx, worktreePath, "rev-parse", "--verify", "refs/remotes/"+upstream)
	if err := runGitCmd(ctx, verifyCmd); err != nil {
		return fmt.Errorf("contribution upstream %q is unavailable: %w", upstream, err)
	}
	cmd := m.newNonInteractiveGitCmd(ctx, worktreePath, "branch", "--set-upstream-to="+upstream, localBranch)
	if output, err := runGitCmdCombinedOutput(ctx, cmd); err != nil {
		m.logger.Debug("failed to set contribution upstream",
			zap.String("branch", localBranch), zap.String("upstream", upstream),
			zap.String("output", string(output)), zap.Error(err))
		return fmt.Errorf("set contribution upstream: %w", err)
	}
	return nil
}
