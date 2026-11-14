package gitlab

import (
	"context"
	"fmt"
	"net/url"
	"slices"
)

// ProjectProtection is what GitLab enforces for merging into one branch.
type ProjectProtection struct {
	PipelineRequired bool
	ThreadsRequired  bool
	// ApprovalsKnown is false when the approval rules could not be read.
	ApprovalsKnown    bool
	ApprovalsRequired bool
}

// projectProtectionReader is implemented by clients that can read the
// project settings and approval rules behind a branch.
type projectProtectionReader interface {
	GetProjectProtection(ctx context.Context, projectPath, branch string) (*ProjectProtection, error)
}

// squashOptionReader is implemented by clients that can read the project's
// raw squash option (never, default_off, default_on, always).
type squashOptionReader interface {
	GetProjectSquashOption(ctx context.Context, projectPath string) (string, error)
}

// branchCommitLister is implemented by clients that can list a branch's commits.
type branchCommitLister interface {
	ListBranchCommits(ctx context.Context, projectPath, branch string, page int) ([]MRCommitInfo, error)
}

// GetProjectSquashOption reads the project's squash option.
func (c *PATClient) GetProjectSquashOption(ctx context.Context, projectPath string) (string, error) {
	var raw struct {
		SquashOption string `json:"squash_option"`
	}
	if err := c.get(ctx, fmt.Sprintf("/projects/%s", projectRef(projectPath)), &raw); err != nil {
		return "", fmt.Errorf("get project squash option: %w", err)
	}
	return raw.SquashOption, nil
}

// GetProjectProtection reads the project merge settings and the approval
// rules that apply to a branch; an empty branch is the default branch. A
// failed project read is an error; a failed approval read leaves
// ApprovalsKnown false.
func (c *PATClient) GetProjectProtection(ctx context.Context, projectPath, branch string) (*ProjectProtection, error) {
	var project struct {
		DefaultBranch                 string `json:"default_branch"`
		OnlyAllowMergeIfPipelineOK    bool   `json:"only_allow_merge_if_pipeline_succeeds"`
		OnlyAllowMergeIfDiscussionsOK bool   `json:"only_allow_merge_if_all_discussions_are_resolved"`
	}
	if err := c.get(ctx, fmt.Sprintf("/projects/%s", projectRef(projectPath)), &project); err != nil {
		return nil, fmt.Errorf("get project settings: %w", err)
	}
	if branch == "" {
		branch = project.DefaultBranch
	}
	out := &ProjectProtection{
		PipelineRequired: project.OnlyAllowMergeIfPipelineOK,
		ThreadsRequired:  project.OnlyAllowMergeIfDiscussionsOK,
	}
	out.ApprovalsKnown, out.ApprovalsRequired = c.approvalRequirement(ctx, projectPath, branch)
	return out, nil
}

// approvalRequirement reports whether merging into branch needs approvals. The
// answer is unknown unless both the approval rules and the branch rule read.
func (c *PATClient) approvalRequirement(ctx context.Context, projectPath, branch string) (known, required bool) {
	var rules []struct {
		ApprovalsRequired int `json:"approvals_required"`
		ProtectedBranches []struct {
			Name string `json:"name"`
		} `json:"protected_branches"`
	}
	endpoint := fmt.Sprintf("/projects/%s/approval_rules?per_page=100", projectRef(projectPath))
	if err := c.get(ctx, endpoint, &rules); err != nil {
		return false, false
	}
	for _, r := range rules {
		if r.ApprovalsRequired <= 0 {
			continue
		}
		names := make([]string, 0, len(r.ProtectedBranches))
		for _, b := range r.ProtectedBranches {
			names = append(names, b.Name)
		}
		if len(names) == 0 || slices.Contains(names, branch) {
			return true, true
		}
	}
	protected, err := c.GetProtectedBranch(ctx, projectPath, branch)
	if err != nil {
		return false, false
	}
	return true, protected != nil && protected.CodeOwnerApprovalRequired
}

// ListBranchCommits lists one page of 100 commits of a branch, newest first.
func (c *PATClient) ListBranchCommits(ctx context.Context, projectPath, branch string, page int) ([]MRCommitInfo, error) {
	if page < 1 {
		page = 1
	}
	var raw []struct {
		ID           string `json:"id"`
		Message      string `json:"message"`
		AuthorName   string `json:"author_name"`
		AuthoredDate string `json:"authored_date"`
	}
	endpoint := fmt.Sprintf("/projects/%s/repository/commits?ref_name=%s&per_page=100&page=%d",
		projectRef(projectPath), url.QueryEscape(branch), page)
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return nil, fmt.Errorf("list branch commits: %w", err)
	}
	out := make([]MRCommitInfo, len(raw))
	for i, r := range raw {
		out[i] = MRCommitInfo{SHA: r.ID, Message: r.Message, AuthorName: r.AuthorName, AuthorDate: r.AuthoredDate}
	}
	return out, nil
}
