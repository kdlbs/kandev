package gitlab

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// MockClient is an in-memory GitLab client used by E2E tests.
// Activated when KANDEV_MOCK_GITLAB=true. It serves a small fixed set of
// MRs/issues/discussions plus accepts dynamic seeding via mock_controller.
//
// The mock intentionally implements the full Client interface but covers
// only the fields E2E flows exercise — the goal is to drive UI tests, not
// to emulate GitLab faithfully.
type MockClient struct {
	host string

	mu          sync.Mutex
	username    string
	mrs         map[mockMRKey]*MR
	discussions map[mockMRKey][]MRDiscussion
	// pipelines is keyed by project — ListPipelines below returns the
	// project's seeded set regardless of branch or iid, matching the
	// real PATClient.ListPipelines flow (one MR head ref → all
	// pipelines for that project). Keying by mockMRKey here would
	// make iteration order matter when multiple MRs share a project.
	pipelines map[string][]Pipeline
	// approvals tracks who has approved each MR. requiredApprovals tracks
	// the project-level required-count GitLab returns alongside the
	// approved_by list. Both are seeded separately because the GitLab
	// /approvals endpoint conflates them on one payload — the mock keeps
	// them split so tests can express e.g. "2 approvals required, 1 given".
	approvals         map[mockMRKey][]MRApproval
	requiredApprovals map[mockMRKey]int
	issues            map[mockIssueKey]*Issue
	branches          map[string][]RepoBranch
	nextMRIID         int
}

type mockMRKey struct {
	Project string
	IID     int
}

type mockIssueKey struct {
	Project string
	IID     int
}

// NewMockClient builds a fresh mock with a small canned dataset.
func NewMockClient(host string) *MockClient {
	if host == "" {
		host = DefaultHost
	}
	c := &MockClient{
		host:              host,
		username:          "kandev-tester",
		mrs:               make(map[mockMRKey]*MR),
		discussions:       make(map[mockMRKey][]MRDiscussion),
		pipelines:         make(map[string][]Pipeline),
		approvals:         make(map[mockMRKey][]MRApproval),
		requiredApprovals: make(map[mockMRKey]int),
		issues:            make(map[mockIssueKey]*Issue),
		branches:          make(map[string][]RepoBranch),
		nextMRIID:         100,
	}
	return c
}

// SetUser overrides the authenticated user reported by the mock.
func (c *MockClient) SetUser(username string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.username = username
}

// SeedMR registers an MR for (projectPath, iid). If iid == 0 the mock
// assigns one and returns it. The MR is stored verbatim.
func (c *MockClient) SeedMR(projectPath string, mr *MR) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	iid := mr.IID
	if iid == 0 {
		iid = c.nextMRIID
		c.nextMRIID++
		mr.IID = iid
	}
	mr.ProjectPath = projectPath
	c.mrs[mockMRKey{Project: projectPath, IID: iid}] = mr
	return iid
}

// SeedDiscussions sets the discussions returned for an MR.
func (c *MockClient) SeedDiscussions(projectPath string, iid int, discussions []MRDiscussion) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.discussions[mockMRKey{Project: projectPath, IID: iid}] = discussions
}

// SeedIssue registers an issue.
func (c *MockClient) SeedIssue(projectPath string, issue *Issue) {
	c.mu.Lock()
	defer c.mu.Unlock()
	issue.ProjectPath = projectPath
	c.issues[mockIssueKey{Project: projectPath, IID: issue.IID}] = issue
}

// SeedBranches sets the branches returned for a project.
func (c *MockClient) SeedBranches(projectPath string, branches []RepoBranch) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.branches[projectPath] = branches
}

// SeedPipelines registers the pipelines returned for projectPath.
// The mock's ListPipelines returns every pipeline seeded under the project
// regardless of branch, mirroring how the real PATClient surfaces a single
// project-level pipeline list. Keyed by project (not MR iid) so two MRs in
// the same project share one canonical list — calling SeedPipelines twice
// overwrites rather than racing on map iteration order.
func (c *MockClient) SeedPipelines(projectPath string, pipelines []Pipeline) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pipelines[projectPath] = pipelines
}

// SeedApprovals registers the approval state for (projectPath, iid):
// the list of who has approved + the required-count for "approved" to
// flip on. Tests can express "1 of 2 approvals" by passing one approver
// and required=2.
func (c *MockClient) SeedApprovals(projectPath string, iid int, approvals []MRApproval, required int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := mockMRKey{Project: projectPath, IID: iid}
	c.approvals[key] = approvals
	c.requiredApprovals[key] = required
}

func (c *MockClient) Host() string { return c.host }

func (c *MockClient) IsAuthenticated(context.Context) (bool, error) {
	return true, nil
}

func (c *MockClient) GetAuthenticatedUser(context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.username, nil
}

func (c *MockClient) GetMR(_ context.Context, projectPath string, iid int) (*MR, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	mr, ok := c.mrs[mockMRKey{Project: projectPath, IID: iid}]
	if !ok {
		return nil, fmt.Errorf("mock: MR %s!%d not found", projectPath, iid)
	}
	return mr, nil
}

func (c *MockClient) FindMRByBranch(_ context.Context, projectPath, branch string) (*MR, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, mr := range c.mrs {
		if mr.ProjectPath == projectPath && mr.HeadBranch == branch && mr.State == mrStateOpen {
			return mr, nil
		}
	}
	return nil, nil
}

func (c *MockClient) ListAuthoredMRs(_ context.Context, projectPath string) ([]*MR, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	user := c.username
	out := []*MR{}
	for _, mr := range c.mrs {
		if mr.ProjectPath == projectPath && mr.AuthorUsername == user {
			out = append(out, mr)
		}
	}
	return out, nil
}

func (c *MockClient) ListReviewRequestedMRs(context.Context, string, string) ([]*MR, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []*MR{}
	for _, mr := range c.mrs {
		for _, r := range mr.Reviewers {
			if r.Username == c.username {
				out = append(out, mr)
				break
			}
		}
	}
	return out, nil
}

func (c *MockClient) ListUserGroups(context.Context) ([]Group, error) {
	return []Group{{ID: 1, Path: "kandev", Name: "Kandev"}}, nil
}

func (c *MockClient) SearchGroupProjects(context.Context, string, string, int) ([]Project, error) {
	return []Project{}, nil
}

func (c *MockClient) ListMRApprovals(_ context.Context, projectPath string, iid int) ([]MRApproval, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if a, ok := c.approvals[mockMRKey{Project: projectPath, IID: iid}]; ok {
		return a, nil
	}
	return []MRApproval{}, nil
}

func (c *MockClient) ListMRDiscussions(_ context.Context, projectPath string, iid int, _ *time.Time) ([]MRDiscussion, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.discussions[mockMRKey{Project: projectPath, IID: iid}], nil
}

func (c *MockClient) CreateMRDiscussionNote(_ context.Context, projectPath string, iid int, discussionID, body string) (*MRNote, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := mockMRKey{Project: projectPath, IID: iid}
	now := time.Now().UTC()
	note := MRNote{
		ID:        now.UnixNano(),
		Author:    c.username,
		Body:      body,
		CreatedAt: now,
		UpdatedAt: now,
	}
	for i, d := range c.discussions[key] {
		if d.ID == discussionID {
			c.discussions[key][i].Notes = append(c.discussions[key][i].Notes, note)
			c.discussions[key][i].UpdatedAt = now
			return &note, nil
		}
	}
	return nil, fmt.Errorf("mock: discussion %s not found", discussionID)
}

func (c *MockClient) ResolveMRDiscussion(_ context.Context, projectPath string, iid int, discussionID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := mockMRKey{Project: projectPath, IID: iid}
	for i, d := range c.discussions[key] {
		if d.ID == discussionID {
			c.discussions[key][i].Resolved = true
			return nil
		}
	}
	return fmt.Errorf("mock: discussion %s not found", discussionID)
}

func (c *MockClient) ListPipelines(_ context.Context, projectPath, _ string) ([]Pipeline, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if p, ok := c.pipelines[projectPath]; ok {
		return p, nil
	}
	return []Pipeline{}, nil
}

func (c *MockClient) GetMRFeedback(ctx context.Context, projectPath string, iid int) (*MRFeedback, error) {
	mr, err := c.GetMR(ctx, projectPath, iid)
	if err != nil {
		return nil, err
	}
	d, _ := c.ListMRDiscussions(ctx, projectPath, iid, nil)
	approvals, _ := c.ListMRApprovals(ctx, projectPath, iid)
	// Mirror PATClient.GetMRFeedback: only consider pipelines when the MR
	// actually has a head ref. MockClient.ListPipelines ignores its branch
	// argument and returns every pipeline seeded under the project, so
	// without this guard a fresh MR with no head would still inherit a
	// failing pipeline from a sibling MR in the same project.
	var pipelines []Pipeline
	if mr.HeadSHA != "" || mr.HeadBranch != "" {
		pipelines, _ = c.ListPipelines(ctx, projectPath, mr.HeadBranch)
	}
	return &MRFeedback{
		MR:          mr,
		Approvals:   approvals,
		Discussions: d,
		Pipelines:   pipelines,
		HasIssues:   hasOpenDiscussions(d) || pipelineFailing(pipelines),
	}, nil
}

func (c *MockClient) GetMRStatus(ctx context.Context, projectPath string, iid int) (*MRStatus, error) {
	mr, err := c.GetMR(ctx, projectPath, iid)
	if err != nil {
		return nil, err
	}
	var pipelines []Pipeline
	if mr.HeadSHA != "" || mr.HeadBranch != "" {
		pipelines, _ = c.ListPipelines(ctx, projectPath, mr.HeadBranch)
	}
	approvals, _ := c.ListMRApprovals(ctx, projectPath, iid)
	c.mu.Lock()
	required := c.requiredApprovals[mockMRKey{Project: projectPath, IID: iid}]
	c.mu.Unlock()
	pipelineState, jobsTotal, jobsPassing := summarizePipelines(pipelines)
	approvalState := summarizeApprovals(len(approvals), required)
	return &MRStatus{
		MR:                  mr,
		ApprovalState:       approvalState,
		PipelineState:       pipelineState,
		MergeStatus:         mr.MergeStatus,
		ApprovalCount:       len(approvals),
		RequiredApprovals:   required,
		PipelineJobsTotal:   jobsTotal,
		PipelineJobsPassing: jobsPassing,
	}, nil
}

func (c *MockClient) ListMRFiles(context.Context, string, int) ([]MRFile, error) {
	return []MRFile{}, nil
}

func (c *MockClient) ListMRCommits(context.Context, string, int) ([]MRCommitInfo, error) {
	return []MRCommitInfo{}, nil
}

func (c *MockClient) SubmitMRApproval(context.Context, string, int) error   { return nil }
func (c *MockClient) SubmitMRUnapproval(context.Context, string, int) error { return nil }

func (c *MockClient) CreateMR(_ context.Context, projectPath, sourceBranch, targetBranch, title, description string, draft bool) (*MR, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	iid := c.nextMRIID
	c.nextMRIID++
	mr := &MR{
		IID:            iid,
		Title:          title,
		Body:           description,
		HeadBranch:     sourceBranch,
		BaseBranch:     targetBranch,
		State:          mrStateOpen,
		Draft:          draft,
		AuthorUsername: c.username,
		ProjectPath:    projectPath,
		WebURL:         fmt.Sprintf("%s/%s/-/merge_requests/%d", c.host, projectPath, iid),
		URL:            fmt.Sprintf("%s/%s/-/merge_requests/%d", c.host, projectPath, iid),
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	c.mrs[mockMRKey{Project: projectPath, IID: iid}] = mr
	return mr, nil
}

func (c *MockClient) ListProjectBranches(_ context.Context, projectPath string) ([]RepoBranch, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.branches[projectPath], nil
}

func (c *MockClient) ListIssues(context.Context, string, string) ([]*Issue, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []*Issue{}
	for _, i := range c.issues {
		out = append(out, i)
	}
	return out, nil
}

func (c *MockClient) ListIssuesPaged(ctx context.Context, filter, customQuery string, page, perPage int) (*IssueSearchPage, error) {
	issues, _ := c.ListIssues(ctx, filter, customQuery)
	return &IssueSearchPage{Issues: issues, TotalCount: len(issues), Page: page, PerPage: perPage}, nil
}

func (c *MockClient) SearchMRs(context.Context, string, string) ([]*MR, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []*MR{}
	for _, mr := range c.mrs {
		out = append(out, mr)
	}
	return out, nil
}

func (c *MockClient) SearchMRsPaged(ctx context.Context, filter, customQuery string, page, perPage int) (*MRSearchPage, error) {
	mrs, _ := c.SearchMRs(ctx, filter, customQuery)
	return &MRSearchPage{MRs: mrs, TotalCount: len(mrs), Page: page, PerPage: perPage}, nil
}

func (c *MockClient) GetIssueState(_ context.Context, projectPath string, iid int) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if i, ok := c.issues[mockIssueKey{Project: projectPath, IID: iid}]; ok {
		return i.State, nil
	}
	return "", fmt.Errorf("mock: issue %s#%d not found", projectPath, iid)
}

// MergeMR marks the seeded MR as merged.
func (c *MockClient) MergeMR(_ context.Context, projectPath string, iid int, _ bool, _ string) (*MR, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	mr, ok := c.mrs[mockMRKey{Project: projectPath, IID: iid}]
	if !ok {
		return nil, fmt.Errorf("mock: MR %s!%d not found", projectPath, iid)
	}
	mr.State = gitlabStateMerged
	now := time.Now().UTC()
	mr.MergedAt = &now
	return mr, nil
}

// GetProjectMergeMethods returns a canned merge-methods response (merge + squash).
func (c *MockClient) GetProjectMergeMethods(context.Context, string) (*ProjectMergeMethods, error) {
	return &ProjectMergeMethods{Merge: true, AllowSquash: true}, nil
}

// GetProtectedBranch always reports the branch as unprotected in the mock.
func (c *MockClient) GetProtectedBranch(context.Context, string, string) (*ProtectedBranch, error) {
	return nil, nil
}

// ListUserProjects returns one fake project for the mock.
func (c *MockClient) ListUserProjects(context.Context) ([]Project, error) {
	return []Project{
		{ID: 1, PathWithNamespace: "kandev/sample", Namespace: "kandev", Path: "sample", Name: "sample"},
	}, nil
}

// SearchProjects returns the same fake project filtered by query substring.
func (c *MockClient) SearchProjects(ctx context.Context, query string, _ int) ([]Project, error) {
	projects, _ := c.ListUserProjects(ctx)
	if query == "" {
		return projects, nil
	}
	needle := strings.ToLower(query)
	out := make([]Project, 0, len(projects))
	for _, p := range projects {
		if strings.Contains(strings.ToLower(p.PathWithNamespace), needle) ||
			strings.Contains(strings.ToLower(p.Name), needle) ||
			strings.Contains(strings.ToLower(p.Path), needle) {
			out = append(out, p)
		}
	}
	return out, nil
}

// SetMRLabels is a no-op in the mock (seeded MRs are accepted verbatim).
func (c *MockClient) SetMRLabels(context.Context, string, int, []string) error { return nil }

// SetMRAssignees is a no-op in the mock.
func (c *MockClient) SetMRAssignees(context.Context, string, int, []int) error { return nil }

// Stats returns a summary of the seeded data, useful for E2E assertions.
func (c *MockClient) Stats() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return fmt.Sprintf(
		"mrs=%d discussions=%d issues=%d",
		len(c.mrs), totalDiscussions(c.discussions), len(c.issues),
	)
}

func totalDiscussions(m map[mockMRKey][]MRDiscussion) int {
	total := 0
	for _, d := range m {
		total += len(d)
	}
	return total
}
