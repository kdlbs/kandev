package gitlab

import "context"

// MRAutomationSnapshot is the automation-specific fetch for one evaluation
// pass: the MR itself, its latest pipeline's failing jobs, and its
// unresolved discussions. Unlike MRStatus (the poller's per-minute fetch,
// discussion-free to bound API cost — see MRStatus's type doc) and
// MRFeedback (the full live UI fetch), this is scoped to exactly what
// auto-fix's delta builder and auto-merge's readiness gate need.
type MRAutomationSnapshot struct {
	MR *MR
	// PipelineStatus is the latest pipeline's raw GitLab status ("running",
	// "pending", "success", "failed", "canceled", "skipped", "manual", ...),
	// or "" when the MR has no pipeline yet. Distinct from FailingJobs: a
	// pipeline with some jobs already failed but others still running is
	// not settled, and auto-fix must wait rather than dispatch on a partial
	// picture (AC9).
	PipelineStatus        string
	FailingJobs           []PipelineJob
	Discussions           []MRDiscussion
	UnresolvedDiscussions int
}

// GetMRAutomationSnapshot fetches an MRAutomationSnapshot, scoped to the
// linked row's own workspace + host (C3): an automation call can never be
// routed to a different GitLab host than the one the MR was linked
// against, even if the workspace's configured host changed since. A host
// mismatch surfaces ErrWorkspaceHostMismatch.
func (s *Service) GetMRAutomationSnapshot(
	ctx context.Context, workspaceID, host, projectPath string, mrIID int,
) (*MRAutomationSnapshot, error) {
	var snapshot *MRAutomationSnapshot
	err := s.RunWithWorkspaceClient(ctx, workspaceID, host, func(client Client) error {
		built, err := buildMRAutomationSnapshot(ctx, client, projectPath, mrIID)
		if err != nil {
			return err
		}
		snapshot = built
		return nil
	})
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func buildMRAutomationSnapshot(ctx context.Context, client Client, projectPath string, mrIID int) (*MRAutomationSnapshot, error) {
	mr, err := client.GetMR(ctx, projectPath, mrIID)
	if err != nil {
		return nil, err
	}
	pipelineStatus, failingJobs, err := latestPipelineStatusAndFailingJobs(ctx, client, projectPath, mr)
	if err != nil {
		return nil, err
	}
	discussions, err := client.ListMRDiscussions(ctx, projectPath, mrIID, nil)
	if err != nil {
		return nil, err
	}
	return &MRAutomationSnapshot{
		MR:                    mr,
		PipelineStatus:        pipelineStatus,
		FailingJobs:           failingJobs,
		Discussions:           discussions,
		UnresolvedDiscussions: countUnresolvedDiscussions(discussions),
	}, nil
}

// latestPipelineStatusAndFailingJobs fetches the MR's most recent
// pipeline's raw status plus the jobs that count as failed (allow_failure
// jobs excluded — see summarizePipelineJobs). Returns ("", nil, nil) when
// the MR has no pipeline yet.
func latestPipelineStatusAndFailingJobs(ctx context.Context, client Client, projectPath string, mr *MR) (string, []PipelineJob, error) {
	if mr == nil || mr.HeadBranch == "" {
		return "", nil, nil
	}
	pipelines, err := client.ListPipelines(ctx, projectPath, mr.HeadBranch)
	if err != nil {
		return "", nil, err
	}
	if len(pipelines) == 0 {
		return "", nil, nil
	}
	jobs, err := client.ListPipelineJobs(ctx, projectPath, pipelines[0].ID)
	if err != nil {
		return "", nil, err
	}
	_, _, failingJobs := summarizePipelineJobs(jobs)
	return pipelines[0].Status, failingJobs, nil
}

// MergeMRForAutomation merges an MR on behalf of auto-merge, scoped to the
// linked row's own workspace + host (C3) — see GetMRAutomationSnapshot's
// doc for why this matters.
//
// It routes through mergeMRWithClient with an empty method so the project's
// own merge configuration decides, exactly as the interactive merge path
// does. Calling client.MergeMR directly with squash=false instead would
// override that policy: a project with squash *required* rejects the merge
// outright, and a squash-by-default project silently gets unsquashed
// automation merges.
func (s *Service) MergeMRForAutomation(ctx context.Context, workspaceID, host, projectPath string, mrIID int) (*MR, error) {
	var merged *MR
	err := s.RunWithWorkspaceClient(ctx, workspaceID, host, func(client Client) error {
		result, err := s.mergeMRWithClient(ctx, client, projectPath, mrIID, "", "")
		if err != nil {
			return err
		}
		merged = result
		return nil
	})
	if err != nil {
		return nil, err
	}
	return merged, nil
}
