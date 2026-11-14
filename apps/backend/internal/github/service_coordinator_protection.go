package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// ProtectionRequirements are the merge requirements one protection source
// enforces on a branch.
type ProtectionRequirements struct {
	Pipeline, Approvals, Threads bool
}

// ClassicProtectionState is the outcome of the classic branch-protection read.
type ClassicProtectionState string

// Classic branch-protection read outcomes.
const (
	// ClassicProtectionFailed is any failed, rate-limited or unauthorized read.
	ClassicProtectionFailed ClassicProtectionState = "failed"
	// ClassicProtectionFound is a rule that was read.
	ClassicProtectionFound ClassicProtectionState = "found"
	// ClassicProtectionAbsent is a 404; it means "no rule" only with admin access.
	ClassicProtectionAbsent ClassicProtectionState = "absent"
)

// BranchProtectionRead is what GitHub enforces for merging into one branch,
// read from classic branch protection and from rulesets.
type BranchProtectionRead struct {
	Classic        ClassicProtectionState
	AdminConfirmed bool
	ClassicRequire ProtectionRequirements
	// RulesetsOK is set when the rulesets that apply to the branch were read.
	RulesetsOK      bool
	RulesetsRequire ProtectionRequirements
}

// ReadBranchProtectionForAutomation reads classic branch protection and the
// rulesets of a branch; an empty branch is the repository default branch. A
// failed repository read is an error; a failed protection read is reported in
// the result, never as an absent rule.
func (s *Service) ReadBranchProtectionForAutomation(
	ctx context.Context, workspaceID, host, owner, repo, branch string,
) (BranchProtectionRead, error) {
	reader, err := s.automationRESTReader(ctx, workspaceID, host, owner, repo)
	if err != nil {
		return BranchProtectionRead{}, err
	}
	var info struct {
		DefaultBranch string `json:"default_branch"`
		Permissions   struct {
			Admin bool `json:"admin"`
		} `json:"permissions"`
	}
	if err := getRESTInto(ctx, reader, fmt.Sprintf("/repos/%s/%s", owner, repo), &info); err != nil {
		return BranchProtectionRead{}, fmt.Errorf("read repository: %w", err)
	}
	if branch == "" {
		branch = info.DefaultBranch
	}
	if branch == "" {
		return BranchProtectionRead{}, errors.New("repository has no default branch")
	}
	escaped := url.PathEscape(branch)
	out := BranchProtectionRead{AdminConfirmed: info.Permissions.Admin}
	out.Classic, out.ClassicRequire = readClassicProtection(ctx, reader, owner, repo, escaped)
	out.RulesetsOK, out.RulesetsRequire = readRulesets(ctx, reader, owner, repo, escaped)
	return out, nil
}

func (s *Service) automationRESTReader(
	ctx context.Context, workspaceID, host, owner, repo string,
) (restJSONReader, error) {
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return nil, err
	}
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, owner, repo)
	if err != nil {
		return nil, err
	}
	if err := requireAutomationHost(resolved, host); err != nil {
		return nil, err
	}
	reader, ok := resolved.Client.(restJSONReader)
	if !ok {
		return nil, errors.New("github client cannot read provider settings")
	}
	return reader, nil
}

func getRESTInto(ctx context.Context, reader restJSONReader, endpoint string, out any) error {
	raw, err := reader.GetRESTJSON(ctx, endpoint)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func readClassicProtection(
	ctx context.Context, reader restJSONReader, owner, repo, escapedBranch string,
) (ClassicProtectionState, ProtectionRequirements) {
	var raw struct {
		RequiredStatusChecks *json.RawMessage `json:"required_status_checks"`
		Reviews              *struct {
			Count int `json:"required_approving_review_count"`
		} `json:"required_pull_request_reviews"`
		Conversation *struct {
			Enabled bool `json:"enabled"`
		} `json:"required_conversation_resolution"`
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/branches/%s/protection", owner, repo, escapedBranch)
	if err := getRESTInto(ctx, reader, endpoint, &raw); err != nil {
		var apiErr *GitHubAPIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return ClassicProtectionAbsent, ProtectionRequirements{}
		}
		return ClassicProtectionFailed, ProtectionRequirements{}
	}
	return ClassicProtectionFound, ProtectionRequirements{
		Pipeline:  raw.RequiredStatusChecks != nil,
		Approvals: raw.Reviews != nil && raw.Reviews.Count > 0,
		Threads:   raw.Conversation != nil && raw.Conversation.Enabled,
	}
}

func readRulesets(
	ctx context.Context, reader restJSONReader, owner, repo, escapedBranch string,
) (bool, ProtectionRequirements) {
	var rules []struct {
		Type       string `json:"type"`
		Parameters struct {
			Count   int  `json:"required_approving_review_count"`
			Threads bool `json:"required_review_thread_resolution"`
		} `json:"parameters"`
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/rules/branches/%s", owner, repo, escapedBranch)
	if err := getRESTInto(ctx, reader, endpoint, &rules); err != nil {
		return false, ProtectionRequirements{}
	}
	var req ProtectionRequirements
	for _, r := range rules {
		switch r.Type {
		case "required_status_checks":
			req.Pipeline = true
		case "pull_request":
			req.Approvals = req.Approvals || r.Parameters.Count > 0
			req.Threads = req.Threads || r.Parameters.Threads
		}
	}
	return true, req
}

// BranchCommit is one commit of a branch listing.
type BranchCommit struct {
	SHA     string
	Message string
}

// ListBranchCommitsForAutomation lists one page of 100 commits of a branch,
// newest first. more reports a full page.
func (s *Service) ListBranchCommitsForAutomation(
	ctx context.Context, workspaceID, host, owner, repo, branch string, page int,
) (commits []BranchCommit, more bool, err error) {
	reader, err := s.automationRESTReader(ctx, workspaceID, host, owner, repo)
	if err != nil {
		return nil, false, err
	}
	if page < 1 {
		page = 1
	}
	var raw []struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/commits?sha=%s&per_page=100&page=%d", owner, repo, url.QueryEscape(branch), page)
	if err := getRESTInto(ctx, reader, endpoint, &raw); err != nil {
		return nil, false, fmt.Errorf("list branch commits: %w", err)
	}
	for _, c := range raw {
		commits = append(commits, BranchCommit{SHA: c.SHA, Message: c.Commit.Message})
	}
	return commits, len(raw) >= 100, nil
}
