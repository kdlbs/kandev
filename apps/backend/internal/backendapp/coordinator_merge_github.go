package backendapp

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/coordinator/mergestate"
	"github.com/kandev/kandev/internal/github"
)

// githubMergeAPI is the part of github.Service the merge adapter uses.
type githubMergeAPI interface {
	ReadPRLiveForAutomation(ctx context.Context, workspaceID, host, owner, repo string, number int) (*github.PRLiveRead, error)
	MergePRForAutomationOutcome(ctx context.Context, workspaceID, host, owner, repo string, number int, mergeMethod, expectedHeadSHA string) (github.AutomationMergeResult, error)
	RepoMergeMethodsForAutomation(ctx context.Context, workspaceID, host, owner, repo string) (github.RepoMergeMethods, error)
	ReadBranchProtectionForAutomation(ctx context.Context, workspaceID, host, owner, repo, branch string) (github.BranchProtectionRead, error)
	ListBranchCommitsForAutomation(ctx context.Context, workspaceID, host, owner, repo, branch string, page int) ([]github.BranchCommit, bool, error)
}

type githubMerge struct{ api githubMergeAPI }

func (g githubMerge) readLive(ctx context.Context, ws string, t mergeTarget, number int) (mergestate.Snapshot, error) {
	read, err := g.api.ReadPRLiveForAutomation(ctx, ws, t.callHost, t.owner, t.name, number)
	if err != nil {
		return mergestate.Snapshot{}, err
	}
	if read == nil || read.Status == nil || read.Status.PR == nil {
		return mergestate.Snapshot{}, errors.New("github returned no pull request")
	}
	return githubSnapshotOf(read), nil
}

func githubSnapshotOf(read *github.PRLiveRead) mergestate.Snapshot {
	st, pr := read.Status, read.Status.PR
	state := st.MergeableState
	if state == "" {
		state = pr.MergeableState
	}
	snap := mergestate.Snapshot{
		State: changeStateOf(pr.State), Draft: triOfMet(!pr.Draft), TargetBranch: pr.BaseBranch,
		HeadSHA: pr.HeadSHA, MergeCommit: read.MergeCommit, Pipeline: githubPipelineOf(st),
		Mergeable: githubMergeableOf(state), UnresolvedThreads: -1, Behind: behindOfState(state),
		Approvals: githubApprovalsOf(st.ReviewState),
	}
	if snap.State == mergestate.StateMerged {
		snap.MergedHeadSHA = pr.HeadSHA
	}
	if st.UnresolvedReviewThreadsPopulated {
		snap.UnresolvedThreads = st.UnresolvedReviewThreads
	}
	in := mergestate.ConflictInput{Provider: mergeProviderGitHub, MergeableState: state}
	if read.HasMergeConflicts != nil {
		clear := !*read.HasMergeConflicts
		in.Mergeable = &clear
	}
	snap.Conflict = mergestate.ReadConflict(in)
	return snap
}

func githubPipelineOf(st *github.PRStatus) mergestate.Pipeline {
	switch {
	case st.ChecksState == "failure":
		return mergestate.PipelineRed
	case st.ChecksState == providerStatePending:
		return mergestate.PipelinePending
	case st.ChecksState == "success" && (!st.ChecksPopulated || st.ChecksTotal > 0):
		return mergestate.PipelineGreen
	case st.ChecksState == "" && st.ChecksPopulated && st.ChecksTotal == 0:
		return mergestate.PipelineNone
	}
	return mergestate.PipelineUnavailable
}

func githubMergeableOf(state string) mergestate.MergeableReading {
	switch state {
	case "clean", "has_hooks":
		return mergestate.MergeableReading{Mergeable: mergestate.Met}
	case "draft":
		return mergestate.MergeableReading{Mergeable: mergestate.Unmet, Explains: mergestate.CondDraft}
	case "unstable":
		return mergestate.MergeableReading{Mergeable: mergestate.Unmet, Explains: mergestate.CondPipeline}
	case "dirty", "blocked", "behind":
		return mergestate.MergeableReading{Mergeable: mergestate.Unmet}
	}
	return mergestate.MergeableReading{Mergeable: mergestate.Unavailable}
}

func githubApprovalsOf(review string) mergestate.Tri {
	switch review {
	case "approved":
		return mergestate.Met
	case "changes_requested", providerStatePending:
		return mergestate.Unmet
	}
	return mergestate.Unavailable
}

func behindOfState(state string) int {
	switch state {
	case "behind":
		return 1
	case "", "unknown":
		return -1
	}
	return 0
}

func (g githubMerge) methods(ctx context.Context, ws string, t mergeTarget) (coordinator.MethodSet, error) {
	m, err := g.api.RepoMergeMethodsForAutomation(ctx, ws, t.callHost, t.owner, t.name)
	if err != nil {
		return coordinator.MethodSet{}, err
	}
	var set coordinator.MethodSet
	for _, c := range []struct {
		ok   bool
		name string
	}{{m.Squash, "squash"}, {m.Merge, "merge"}, {m.Rebase, "rebase"}} {
		if c.ok {
			set.Allowed = append(set.Allowed, c.name)
		}
	}
	set.Default = m.DefaultMergeMethod()
	return set, nil
}

func (g githubMerge) merge(ctx context.Context, ws string, t mergeTarget, number int, headSHA, method string) (coordinator.MergeResult, error) {
	res, err := g.api.MergePRForAutomationOutcome(ctx, ws, t.callHost, t.owner, t.name, number, method, headSHA)
	switch {
	case errors.Is(err, github.ErrMergeHeadMoved):
		return coordinator.MergeResult{Outcome: coordinator.MergeStaleHead, Reason: boundedReason(err)}, nil
	case errors.Is(err, github.ErrMergeRejected):
		return coordinator.MergeResult{Outcome: coordinator.MergeRejected, Reason: boundedReason(err)}, nil
	case err != nil:
		return coordinator.MergeResult{}, err
	}
	switch res.Outcome {
	case github.MergeOutcomeMerged:
		return coordinator.MergeResult{Outcome: coordinator.MergeAccepted, MergeCommit: res.MergeCommit}, nil
	case github.MergeOutcomeQueued:
		return coordinator.MergeResult{Outcome: coordinator.MergeQueued}, nil
	}
	return coordinator.MergeResult{}, fmt.Errorf("github returned no merge outcome")
}

func (g githubMerge) commits(ctx context.Context, ws string, t mergeTarget, number, page int) ([]branchCommit, bool, error) {
	snap, err := g.readLive(ctx, ws, t, number)
	if err != nil {
		return nil, false, err
	}
	if snap.TargetBranch == "" {
		return nil, false, errors.New("change has no target branch")
	}
	list, full, err := g.api.ListBranchCommitsForAutomation(ctx, ws, t.callHost, t.owner, t.name, snap.TargetBranch, page)
	if err != nil {
		return nil, false, err
	}
	out := make([]branchCommit, 0, len(list))
	for _, c := range list {
		out = append(out, branchCommit{sha: c.SHA, message: c.Message})
	}
	return out, full, nil
}

func (g githubMerge) protection(ctx context.Context, ws string, t mergeTarget, branch string) (mergestate.Protection, error) {
	read, err := g.api.ReadBranchProtectionForAutomation(ctx, ws, t.callHost, t.owner, t.name, branch)
	if err != nil {
		return mergestate.UnknownProtection(), err
	}
	classic := mergestate.ClassicRead{
		AdminConfirmed: read.AdminConfirmed,
		Requires:       mergestate.ProtectionRequirements(read.ClassicRequire),
	}
	switch read.Classic {
	case github.ClassicProtectionFound:
		classic.State = mergestate.ClassicFound
	case github.ClassicProtectionAbsent:
		classic.State = mergestate.ClassicAbsent
	default:
		classic.State = mergestate.ClassicFailed
	}
	rules := mergestate.RulesetsRead{OK: read.RulesetsOK, Requires: mergestate.ProtectionRequirements(read.RulesetsRequire)}
	return mergestate.GitHubProtection(classic, rules), nil
}
