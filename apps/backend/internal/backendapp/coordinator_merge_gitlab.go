package backendapp

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/coordinator/mergestate"
	"github.com/kandev/kandev/internal/gitlab"
)

const (
	gitlabMethodSquash = "squash"
	gitlabMethodMerge  = "merge"
)

// gitlabMergeAPI is the part of gitlab.Service the merge adapter uses.
type gitlabMergeAPI interface {
	ReadMRLiveForAutomation(ctx context.Context, workspaceID, host, projectPath string, mrIID int) (*gitlab.MRLiveRead, error)
	MergeMRForAutomation(ctx context.Context, workspaceID, host, projectPath string, mrIID int, method, expectedSHA string) (*gitlab.MR, error)
	ProjectMergePolicyForAutomation(ctx context.Context, workspaceID, host, projectPath string) (gitlab.ProjectMergePolicy, error)
	ReadProjectProtectionForAutomation(ctx context.Context, workspaceID, host, projectPath, branch string) (*gitlab.ProjectProtection, error)
	ListBranchCommitsForAutomation(ctx context.Context, workspaceID, host, projectPath, branch string, page int) ([]gitlab.MRCommitInfo, bool, error)
}

type gitlabMerge struct{ api gitlabMergeAPI }

func (g gitlabMerge) readLive(ctx context.Context, ws string, t mergeTarget, number int) (mergestate.Snapshot, error) {
	read, err := g.api.ReadMRLiveForAutomation(ctx, ws, t.callHost, t.path, number)
	if err != nil {
		return mergestate.Snapshot{}, err
	}
	if read == nil || read.Status == nil || read.Status.MR == nil {
		return mergestate.Snapshot{}, errors.New("gitlab returned no merge request")
	}
	return gitlabSnapshotOf(read), nil
}

func gitlabSnapshotOf(read *gitlab.MRLiveRead) mergestate.Snapshot {
	st, mr := read.Status, read.Status.MR
	detailed := mr.DetailedMergeStatus
	if detailed == "" {
		detailed = st.DetailedMergeStatus
	}
	merged := mr.State == providerStateMerged
	snap := mergestate.Snapshot{
		State: changeStateOf(mr.State), Draft: triOfMet(!mr.Draft && detailed != "draft_status"),
		TargetBranch: mr.BaseBranch, HeadSHA: mr.HeadSHA, MergeCommit: mr.MergeCommitSHA,
		Pipeline:          gitlabPipelineOf(st),
		Mergeable:         mergestate.GitLabMergeable(mergestate.GitLabMR{DetailedMergeStatus: detailed, MergeStatus: mr.MergeStatus}),
		UnresolvedThreads: read.UnresolvedDiscussions, Approvals: gitlabApprovalsOf(st), Behind: -1,
		InProgress: mr.State == "locked" || detailed == "locked",
	}
	if merged {
		snap.MergedHeadSHA = mr.HeadSHA
	}
	in := mergestate.ConflictInput{Provider: mergeProviderGitLab, DetailedMergeStatus: detailed}
	switch {
	case mr.HasConflicts:
		yes := true
		in.HasConflicts = &yes
	case detailed == "mergeable":
		no := false
		in.HasConflicts = &no
	}
	snap.Conflict = mergestate.ReadConflict(in)
	return snap
}

func gitlabPipelineOf(st *gitlab.MRStatus) mergestate.Pipeline {
	switch {
	case st.PipelineState == "failure":
		return mergestate.PipelineRed
	case st.PipelineState == providerStatePending:
		return mergestate.PipelinePending
	case st.PipelineState == "success":
		return mergestate.PipelineGreen
	case st.PipelineState == "" && st.PipelineJobsTotal == 0:
		return mergestate.PipelineNone
	}
	return mergestate.PipelineUnavailable
}

func gitlabApprovalsOf(st *gitlab.MRStatus) mergestate.Tri {
	switch {
	case st.ApprovalState == "approved":
		return mergestate.Met
	case st.ApprovalState == providerStatePending:
		return mergestate.Unmet
	case st.ApprovalState == "" && st.RequiredApprovals == 0:
		return mergestate.Met
	}
	return mergestate.Unavailable
}

// methods uses the coordinator's labels: "squash" and "merge". The project's
// own merge method (merge, rebase or fast-forward) is what "merge" means.
func (g gitlabMerge) methods(ctx context.Context, ws string, t mergeTarget) (coordinator.MethodSet, error) {
	policy, err := g.api.ProjectMergePolicyForAutomation(ctx, ws, t.callHost, t.path)
	if err != nil {
		return coordinator.MethodSet{}, err
	}
	switch policy.SquashOption {
	case "never":
		return coordinator.MethodSet{Allowed: []string{gitlabMethodMerge}, Default: gitlabMethodMerge}, nil
	case "always":
		return coordinator.MethodSet{Allowed: []string{gitlabMethodSquash}, Default: gitlabMethodSquash}, nil
	case "default_on":
		return coordinator.MethodSet{Allowed: []string{gitlabMethodSquash, gitlabMethodMerge}, Default: gitlabMethodSquash}, nil
	}
	return coordinator.MethodSet{Allowed: []string{gitlabMethodMerge, gitlabMethodSquash}, Default: gitlabMethodMerge}, nil
}

func (g gitlabMerge) merge(ctx context.Context, ws string, t mergeTarget, number int, headSHA, method string) (coordinator.MergeResult, error) {
	var providerMethod string
	switch method {
	case gitlabMethodSquash:
		providerMethod = gitlabMethodSquash
	case gitlabMethodMerge:
		// The project's configured merge method applies; no squash.
	default:
		return coordinator.MergeResult{}, errors.New("unknown merge method " + method)
	}
	mr, err := g.api.MergeMRForAutomation(ctx, ws, t.callHost, t.path, number, providerMethod, headSHA)
	var refused *gitlab.MergeRefusedError
	switch {
	case errors.Is(err, gitlab.ErrMergeHeadMoved):
		return coordinator.MergeResult{Outcome: coordinator.MergeStaleHead, Reason: boundedReason(err)}, nil
	case errors.As(err, &refused), errors.Is(err, gitlab.ErrMRIsDraft):
		return coordinator.MergeResult{Outcome: coordinator.MergeRejected, Reason: boundedReason(err)}, nil
	case err != nil:
		return coordinator.MergeResult{}, err
	case mr == nil:
		return coordinator.MergeResult{}, errors.New("gitlab returned no merge request")
	case mr.State == providerStateMerged:
		return coordinator.MergeResult{Outcome: coordinator.MergeAccepted, MergeCommit: mr.MergeCommitSHA}, nil
	}
	return coordinator.MergeResult{Outcome: coordinator.MergeQueued}, nil
}

func (g gitlabMerge) commits(ctx context.Context, ws string, t mergeTarget, number, page int) ([]branchCommit, bool, error) {
	snap, err := g.readLive(ctx, ws, t, number)
	if err != nil {
		return nil, false, err
	}
	if snap.TargetBranch == "" {
		return nil, false, errors.New("change has no target branch")
	}
	list, full, err := g.api.ListBranchCommitsForAutomation(ctx, ws, t.callHost, t.path, snap.TargetBranch, page)
	if err != nil {
		return nil, false, err
	}
	out := make([]branchCommit, 0, len(list))
	for _, c := range list {
		out = append(out, branchCommit{sha: c.SHA, message: c.Message})
	}
	return out, full, nil
}

func (g gitlabMerge) protection(ctx context.Context, ws string, t mergeTarget, branch string) (mergestate.Protection, error) {
	p, err := g.api.ReadProjectProtectionForAutomation(ctx, ws, t.callHost, t.path, branch)
	if err != nil {
		return mergestate.UnknownProtection(), err
	}
	if p == nil {
		return mergestate.UnknownProtection(), errors.New("gitlab returned no project settings")
	}
	out := mergestate.Protection{
		Pipeline: enforcementOf(p.PipelineRequired), Threads: enforcementOf(p.ThreadsRequired),
		Approvals: mergestate.UnknownEnf,
	}
	if p.ApprovalsKnown {
		out.Approvals = enforcementOf(p.ApprovalsRequired)
	}
	return out, nil
}

func enforcementOf(required bool) mergestate.Enforcement {
	if required {
		return mergestate.Enforced
	}
	return mergestate.NotEnforced
}
