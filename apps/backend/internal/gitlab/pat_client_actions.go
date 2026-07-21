package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// --- Merge ---

// MergeMRRequest is the JSON body for accepting an MR.
type MergeMRRequest struct {
	MergeCommitMessage       string `json:"merge_commit_message,omitempty"`
	SquashCommitMessage      string `json:"squash_commit_message,omitempty"`
	Squash                   bool   `json:"squash,omitempty"`
	ShouldRemoveSourceBranch bool   `json:"should_remove_source_branch,omitempty"`
}

// MergeMR accepts an MR. GitLab supports three project-level merge methods
// (merge, rebase_merge, ff); the caller picks one and the project must allow
// it. Passing squash=true performs a squash merge regardless of method.
func (c *PATClient) MergeMR(ctx context.Context, projectPath string, iid int, squash bool, squashCommitMessage string) (*MR, error) {
	endpoint := fmt.Sprintf("/projects/%s/merge_requests/%d/merge", projectRef(projectPath), iid)
	body, err := json.Marshal(MergeMRRequest{
		Squash:                   squash,
		SquashCommitMessage:      squashCommitMessage,
		ShouldRemoveSourceBranch: false,
	})
	if err != nil {
		return nil, fmt.Errorf("encode merge request: %w", err)
	}
	var raw rawMR
	if err := c.doWrite(ctx, "PUT", endpoint, body, &raw); err != nil {
		return nil, fmt.Errorf("merge MR !%d: %w", iid, err)
	}
	return convertRawMR(&raw), nil
}

// --- Project merge methods ---

// GetProjectMergeMethods reads a project's `merge_method` and `squash_option`
// settings so the merge button UI can offer the right options.
func (c *PATClient) GetProjectMergeMethods(ctx context.Context, projectPath string) (*ProjectMergeMethods, error) {
	var raw struct {
		MergeMethod  string `json:"merge_method"`  // merge, rebase_merge, ff
		SquashOption string `json:"squash_option"` // never, default_off, default_on, always
	}
	endpoint := fmt.Sprintf("/projects/%s", projectRef(projectPath))
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return nil, fmt.Errorf("get project merge methods: %w", err)
	}
	out := &ProjectMergeMethods{}
	switch raw.MergeMethod {
	case "merge":
		out.Merge = true
	case "rebase_merge":
		out.RebaseMerge = true
	case "ff":
		out.FastForward = true
	default:
		out.Merge = true
	}
	if raw.SquashOption != "" && raw.SquashOption != "never" {
		out.AllowSquash = true
	}
	return out, nil
}

// --- Branch protection ---

// ProtectedBranch represents a GitLab protected branch rule.
type ProtectedBranch struct {
	Name                      string `json:"name"`
	PushAccessLevel           int    `json:"push_access_level"`
	MergeAccessLevel          int    `json:"merge_access_level"`
	CodeOwnerApprovalRequired bool   `json:"code_owner_approval_required"`
}

// GetProtectedBranch returns the protected-branch settings for a branch (404 → nil).
func (c *PATClient) GetProtectedBranch(ctx context.Context, projectPath, branch string) (*ProtectedBranch, error) {
	endpoint := fmt.Sprintf("/projects/%s/protected_branches/%s",
		projectRef(projectPath), url.PathEscape(branch))
	var raw struct {
		Name             string `json:"name"`
		PushAccessLevels []struct {
			AccessLevel int `json:"access_level"`
		} `json:"push_access_levels"`
		MergeAccessLevels []struct {
			AccessLevel int `json:"access_level"`
		} `json:"merge_access_levels"`
		CodeOwnerApprovalRequired bool `json:"code_owner_approval_required"`
	}
	if err := c.get(ctx, endpoint, &raw); err != nil {
		// 404 means the branch isn't protected — return nil without error.
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			return nil, nil
		}
		return nil, fmt.Errorf("get protected branch: %w", err)
	}
	out := &ProtectedBranch{
		Name:                      raw.Name,
		CodeOwnerApprovalRequired: raw.CodeOwnerApprovalRequired,
	}
	if len(raw.PushAccessLevels) > 0 {
		out.PushAccessLevel = raw.PushAccessLevels[0].AccessLevel
	}
	if len(raw.MergeAccessLevels) > 0 {
		out.MergeAccessLevel = raw.MergeAccessLevels[0].AccessLevel
	}
	return out, nil
}

// --- Projects (autocomplete + global search) ---

// ListUserProjects lists projects the authenticated user is a member of.
// GitLab equivalent of GitHub's ListUserOrgs + repo enumeration.
func (c *PATClient) ListUserProjects(ctx context.Context) ([]Project, error) {
	var raw []rawProject
	if err := c.get(ctx, "/projects?membership=true&simple=true&per_page=100&order_by=last_activity_at", &raw); err != nil {
		return nil, fmt.Errorf("list user projects: %w", err)
	}
	out := make([]Project, len(raw))
	for i := range raw {
		out[i] = convertRawProject(&raw[i])
	}
	return out, nil
}

// SearchProjects searches all projects matching `query`. Used by the
// project-autocomplete and watch-creation flows.
func (c *PATClient) SearchProjects(ctx context.Context, query string, limit int) ([]Project, error) {
	if limit <= 0 {
		limit = 20
	}
	endpoint := fmt.Sprintf("/projects?membership=true&simple=true&per_page=%d", limit)
	if strings.TrimSpace(query) != "" {
		endpoint += "&search=" + url.QueryEscape(query)
	}
	var raw []rawProject
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return nil, fmt.Errorf("search projects: %w", err)
	}
	out := make([]Project, len(raw))
	for i := range raw {
		out[i] = convertRawProject(&raw[i])
	}
	return out, nil
}

// --- Labels ---

// SetMRLabels replaces an MR's labels with the given set.
func (c *PATClient) SetMRLabels(ctx context.Context, projectPath string, iid int, labels []string) error {
	endpoint := fmt.Sprintf("/projects/%s/merge_requests/%d", projectRef(projectPath), iid)
	body, err := json.Marshal(map[string]string{"labels": strings.Join(labels, ",")})
	if err != nil {
		return fmt.Errorf("encode set-labels payload: %w", err)
	}
	if err := c.doWrite(ctx, "PUT", endpoint, body, nil); err != nil {
		return fmt.Errorf("set MR labels: %w", err)
	}
	return nil
}

// SetMRAssignees replaces an MR's assignees with the given set of user IDs.
// GitLab takes numeric user IDs (not usernames) for this endpoint.
func (c *PATClient) SetMRAssignees(ctx context.Context, projectPath string, iid int, assigneeIDs []int) error {
	endpoint := fmt.Sprintf("/projects/%s/merge_requests/%d", projectRef(projectPath), iid)
	body, err := json.Marshal(map[string][]int{"assignee_ids": assigneeIDs})
	if err != nil {
		return fmt.Errorf("encode set-assignees payload: %w", err)
	}
	if err := c.doWrite(ctx, "PUT", endpoint, body, nil); err != nil {
		return fmt.Errorf("set MR assignees: %w", err)
	}
	return nil
}

func (c *PATClient) ListProjectMembers(ctx context.Context, projectPath, query string) ([]ProjectMember, error) {
	endpoint := fmt.Sprintf("/projects/%s/members/all?per_page=100", projectRef(projectPath))
	if strings.TrimSpace(query) != "" {
		endpoint += "&query=" + url.QueryEscape(query)
	}
	var raw []struct {
		ID        int64  `json:"id"`
		Username  string `json:"username"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
		State     string `json:"state"`
	}
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return nil, fmt.Errorf("list project members: %w", err)
	}
	members := make([]ProjectMember, 0, len(raw))
	for _, member := range raw {
		if member.State != "" && member.State != "active" {
			continue
		}
		members = append(members, ProjectMember{
			ID:        member.ID,
			Username:  member.Username,
			Name:      member.Name,
			AvatarURL: member.AvatarURL,
		})
	}
	return members, nil
}

func (c *PATClient) SetMRReviewers(ctx context.Context, projectPath string, iid int, reviewerIDs []int64) error {
	if reviewerIDs == nil {
		reviewerIDs = []int64{}
	}
	body, err := json.Marshal(map[string][]int64{"reviewer_ids": reviewerIDs})
	if err != nil {
		return fmt.Errorf("encode set-reviewers payload: %w", err)
	}
	endpoint := fmt.Sprintf("/projects/%s/merge_requests/%d", projectRef(projectPath), iid)
	if err := c.doWrite(ctx, http.MethodPut, endpoint, body, nil); err != nil {
		return fmt.Errorf("set MR reviewers: %w", err)
	}
	return nil
}

func (c *PATClient) GetMRSubscription(ctx context.Context, projectPath string, iid int) (*SubscriptionState, error) {
	return c.getSubscription(ctx, projectPath, "merge_requests", iid)
}

func (c *PATClient) SetMRSubscription(ctx context.Context, projectPath string, iid int, subscribed bool) (*SubscriptionState, error) {
	return c.setSubscription(ctx, projectPath, "merge_requests", iid, subscribed)
}

func (c *PATClient) GetIssueSubscription(ctx context.Context, projectPath string, iid int) (*SubscriptionState, error) {
	return c.getSubscription(ctx, projectPath, "issues", iid)
}

func (c *PATClient) SetIssueSubscription(ctx context.Context, projectPath string, iid int, subscribed bool) (*SubscriptionState, error) {
	return c.setSubscription(ctx, projectPath, "issues", iid, subscribed)
}

func (c *PATClient) getSubscription(ctx context.Context, projectPath, resource string, iid int) (*SubscriptionState, error) {
	endpoint := fmt.Sprintf("/projects/%s/%s/%d", projectRef(projectPath), resource, iid)
	var state SubscriptionState
	if err := c.get(ctx, endpoint, &state); err != nil {
		return nil, fmt.Errorf("get %s subscription: %w", resource, err)
	}
	return &state, nil
}

func (c *PATClient) setSubscription(ctx context.Context, projectPath, resource string, iid int, subscribed bool) (*SubscriptionState, error) {
	action := "unsubscribe"
	if subscribed {
		action = "subscribe"
	}
	endpoint := fmt.Sprintf("/projects/%s/%s/%d/%s", projectRef(projectPath), resource, iid, action)
	var state SubscriptionState
	status, err := c.doWriteWithStatus(ctx, http.MethodPost, endpoint, nil, &state)
	if status == http.StatusNotModified {
		return &SubscriptionState{Subscribed: subscribed}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s to %s notifications: %w", action, resource, err)
	}
	return &state, nil
}
