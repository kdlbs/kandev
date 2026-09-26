package process

import (
	"context"
	"strings"
)

const (
	mainBranchName   = "main"
	masterBranchName = "master"
)

var integrationBranchNames = [...]string{mainBranchName, masterBranchName}

type comparisonBaseGit interface {
	GetMergeBase(ctx context.Context, ref1, ref2 string) (string, error)
	IsAncestor(ctx context.Context, ancestor, descendant string) (bool, error)
}

// CorrectStaleComparisonBase applies the shared stale-base policy used by the
// workspace status, commits, and cumulative-diff paths.
func (g *GitOperator) CorrectStaleComparisonBase(
	ctx context.Context,
	baseCommit, targetBranch string,
) string {
	return correctStaleComparisonBase(ctx, g, baseCommit, targetBranch)
}

func correctStaleComparisonBase(
	ctx context.Context,
	git comparisonBaseGit,
	baseCommit, targetBranch string,
) string {
	if baseCommit == "" || isExplicitComparisonRef(targetBranch) || isIntegrationBranch(targetBranch) {
		return baseCommit
	}
	if targetHasUpstreamRef(ctx, git, targetBranch) {
		return baseCommit
	}
	integrationBase := resolveIntegrationMergeBase(ctx, git)
	if integrationBase == "" || integrationBase == baseCommit {
		return baseCommit
	}
	ancestor, err := git.IsAncestor(ctx, baseCommit, integrationBase)
	if err == nil && ancestor {
		return integrationBase
	}
	return baseCommit
}

func targetHasUpstreamRef(
	ctx context.Context,
	git comparisonBaseGit,
	targetBranch string,
) bool {
	name := strings.TrimPrefix(targetBranch, "origin/")
	if name == "" {
		return false
	}
	mergeBase, err := git.GetMergeBase(ctx, "HEAD", "origin/"+name)
	return err == nil && mergeBase != ""
}

func resolveIntegrationMergeBase(ctx context.Context, git comparisonBaseGit) string {
	for _, name := range integrationBranchNames {
		if mergeBase, err := git.GetMergeBase(ctx, "HEAD", "origin/"+name); err == nil && mergeBase != "" {
			return mergeBase
		}
	}
	for _, name := range integrationBranchNames {
		if mergeBase, err := git.GetMergeBase(ctx, "HEAD", name); err == nil && mergeBase != "" {
			return mergeBase
		}
	}
	return ""
}

func isIntegrationBranch(branch string) bool {
	name := strings.TrimPrefix(branch, "origin/")
	for _, candidate := range integrationBranchNames {
		if name == candidate {
			return true
		}
	}
	return false
}

type workspaceTrackerComparisonGit struct {
	tracker *WorkspaceTracker
}

func (g workspaceTrackerComparisonGit) GetMergeBase(
	ctx context.Context,
	ref1, ref2 string,
) (string, error) {
	output, err := g.tracker.runGitOutput(ctx, "merge-base", ref1, ref2)
	return strings.TrimSpace(string(output)), err
}

func (g workspaceTrackerComparisonGit) IsAncestor(
	ctx context.Context,
	ancestor, descendant string,
) (bool, error) {
	mergeBase, err := g.GetMergeBase(ctx, ancestor, descendant)
	return err == nil && mergeBase == ancestor, err
}

func integrationBranchRefs(includeLocal bool) []string {
	refs := make([]string, 0, len(integrationBranchNames)*2)
	for _, name := range integrationBranchNames {
		refs = append(refs, "origin/"+name)
	}
	if includeLocal {
		refs = append(refs, integrationBranchNames[:]...)
	}
	return refs
}
