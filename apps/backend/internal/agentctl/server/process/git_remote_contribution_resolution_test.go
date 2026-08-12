package process

import (
	"context"
	"strings"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestGitOperatorSupportsExactContributionLeaseFlag(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)

	head := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "push", "origin", "HEAD:refs/heads/feature/contribution")

	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	_, err := operator.runGitCommand(
		context.Background(),
		"push",
		"--force-with-lease=refs/heads/feature/contribution:"+head,
		"origin",
		"HEAD:refs/heads/feature/contribution",
	)
	if err != nil {
		t.Fatalf("exact contribution lease push failed: %v", err)
	}
}

func TestGitOperatorRejectsMalformedContributionLeaseFlag(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)

	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	_, err := operator.runGitCommand(
		context.Background(),
		"push",
		"--force-with-lease=refs/heads/feature/contribution:not-a-commit",
		"origin",
		"HEAD:refs/heads/feature/contribution",
	)
	if err == nil || !strings.Contains(err.Error(), "potentially unsafe flag") {
		t.Fatalf("malformed contribution lease error = %v, want an unsafe-flag error", err)
	}
}

func TestGitOperatorReplaceRemoteContributionUsesExactHeadLease(t *testing.T) {
	repoDir, originDir, _, binding, providerOne, providerTwo, localHead := setupDivergedContributionRepo(t)
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	operator.setRemoteContribution(binding)

	stale, err := operator.ReplaceRemoteContribution(context.Background(), providerOne)
	if err != nil {
		t.Fatalf("stale ReplaceRemoteContribution returned error: %v", err)
	}
	if stale.Success {
		t.Fatalf("stale replacement = %+v, want failure", stale)
	}
	if got := strings.TrimSpace(runGit(t, originDir, "rev-parse", "refs/heads/feature/contribution")); got != providerTwo {
		t.Fatalf("remote head after stale replacement = %q, want %q", got, providerTwo)
	}

	current, err := operator.ReplaceRemoteContribution(context.Background(), providerTwo)
	if err != nil {
		t.Fatalf("ReplaceRemoteContribution returned error: %v", err)
	}
	if !current.Success {
		t.Fatalf("replacement = %+v, want success", current)
	}
	if got := strings.TrimSpace(runGit(t, originDir, "rev-parse", "refs/heads/feature/contribution")); got != localHead {
		t.Fatalf("remote head after replacement = %q, want local %q", got, localHead)
	}
}

func TestGitOperatorUseRemoteContributionCreatesRecoveryAndRetainsUpstream(t *testing.T) {
	repoDir, _, remoteName, binding, _, providerTwo, localHead := setupDivergedContributionRepo(t)
	runGit(t, repoDir, "fetch", remoteName, "feature/contribution")
	runGit(t, repoDir, "branch", "--set-upstream-to="+remoteName+"/feature/contribution", "feature/contribution")

	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	operator.setRemoteContribution(binding)
	result, err := operator.UseRemoteContribution(context.Background(), providerTwo)
	if err != nil {
		t.Fatalf("UseRemoteContribution returned error: %v", err)
	}
	if !result.Success || result.RecoveryBranch == "" {
		t.Fatalf("UseRemoteContribution = %+v, want success with recovery branch", result)
	}
	if got := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/heads/"+result.RecoveryBranch)); got != localHead {
		t.Fatalf("recovery branch head = %q, want old local head %q", got, localHead)
	}
	if got := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD")); got != providerTwo {
		t.Fatalf("task HEAD = %q, want provider head %q", got, providerTwo)
	}
	if got := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")); got != remoteName+"/feature/contribution" {
		t.Fatalf("upstream = %q, want %q", got, remoteName+"/feature/contribution")
	}
}

func TestGitOperatorUseRemoteContributionRejectsStaleFetch(t *testing.T) {
	repoDir, _, _, binding, providerOne, _, localHead := setupDivergedContributionRepo(t)
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	operator.setRemoteContribution(binding)

	result, err := operator.UseRemoteContribution(context.Background(), providerOne)
	if err != nil {
		t.Fatalf("stale UseRemoteContribution returned error: %v", err)
	}
	if result.Success || result.RecoveryBranch != "" {
		t.Fatalf("stale adoption = %+v, want no mutation", result)
	}
	if got := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD")); got != localHead {
		t.Fatalf("task HEAD after stale adoption = %q, want %q", got, localHead)
	}
}

func TestGitOperatorUseRemoteContributionRejectsDirtyWorktree(t *testing.T) {
	for _, dirty := range []struct {
		name  string
		stage bool
	}{
		{name: "unstaged"},
		{name: "staged", stage: true},
		{name: "untracked"},
	} {
		t.Run(dirty.name, func(t *testing.T) {
			repoDir, _, _, binding, _, providerTwo, localHead := setupDivergedContributionRepo(t)
			writeFile(t, repoDir, "dirty-"+dirty.name+".txt", dirty.name+"\n")
			if dirty.stage {
				runGit(t, repoDir, "add", "--", "dirty-"+dirty.name+".txt")
			}

			operator := NewGitOperator(repoDir, newTestLogger(t), nil)
			operator.setRemoteContribution(binding)
			result, err := operator.UseRemoteContribution(context.Background(), providerTwo)
			if err != nil {
				t.Fatalf("UseRemoteContribution returned error: %v", err)
			}
			if result.Success || result.RecoveryBranch != "" || !strings.Contains(result.Error, "clean") {
				t.Fatalf("dirty adoption = %+v, want clean-tree rejection", result)
			}
			if got := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD")); got != localHead {
				t.Fatalf("task HEAD after dirty adoption = %q, want %q", got, localHead)
			}
		})
	}
}

func setupDivergedContributionRepo(t *testing.T) (repoDir, originDir, remoteName string, binding *taskmodels.RemoteContribution, providerOne, providerTwo, localHead string) {
	t.Helper()
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	originDir = strings.TrimSpace(runGit(t, repoDir, "remote", "get-url", "origin"))

	runGit(t, repoDir, "checkout", "-b", "provider-history")
	writeFile(t, repoDir, "provider.txt", "provider one\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "provider one")
	providerOne = strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "push", "origin", "HEAD:refs/heads/feature/contribution")

	writeFile(t, repoDir, "provider.txt", "provider two\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "provider two")
	providerTwo = strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "push", "origin", "HEAD:refs/heads/feature/contribution")

	runGit(t, repoDir, "checkout", "main")
	runGit(t, repoDir, "checkout", "-b", "feature/contribution")
	writeFile(t, repoDir, "local.txt", "local task version\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "local task version")
	localHead = strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	sourceURL := "https://github.com/contributor/widget.git"
	runGit(t, repoDir, "config", "url.file://"+originDir+".insteadOf", sourceURL)
	binding = &taskmodels.RemoteContribution{
		Version:      taskmodels.RemoteContributionVersion,
		Provider:     taskmodels.RemoteContributionProviderGitHub,
		Kind:         taskmodels.RemoteContributionKindPullRequest,
		CanonicalURL: "https://github.com/acme/widget/pull/7",
		Number:       7,
		State:        taskmodels.RemoteContributionStateOpen,
		BaseBranch:   "main",
		HeadBranch:   "feature/contribution",
		HeadSHA:      providerOne,
		SourceRepository: taskmodels.RemoteContributionRepository{
			Host: "github.com", Path: "contributor/widget", RemoteURL: sourceURL,
		},
		CollaborationAllowed: true,
	}
	if err := binding.Validate(); err != nil {
		t.Fatalf("binding.Validate() = %v", err)
	}
	remoteName = binding.ContributionRemoteName()
	runGit(t, repoDir, "remote", "add", remoteName, sourceURL)
	return repoDir, originDir, remoteName, binding, providerOne, providerTwo, localHead
}
