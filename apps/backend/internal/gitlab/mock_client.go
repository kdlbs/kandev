package gitlab

import (
	"context"
	"fmt"
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
	pipelines   map[mockMRKey][]Pipeline
	issues      map[mockIssueKey]*Issue
	branches    map[string][]RepoBranch
	nextMRIID   int
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
		host:        host,
		username:    "kandev-tester",
		mrs:         make(map[mockMRKey]*MR),
		discussions: make(map[mockMRKey][]MRDiscussion),
		pipelines:   make(map[mockMRKey][]Pipeline),
		issues:      make(map[mockIssueKey]*Issue),
		branches:    make(map[string][]RepoBranch),
		nextMRIID:   100,
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
		if mr.ProjectPath == projectPath && mr.HeadBranch == branch && mr.State == "open" {
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

func (c *MockClient) ListMRApprovals(context.Context, string, int) ([]MRApproval, error) {
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
	for key, p := range c.pipelines {
		if key.Project == projectPath {
			return p, nil
		}
	}
	return []Pipeline{}, nil
}

func (c *MockClient) GetMRFeedback(ctx context.Context, projectPath string, iid int) (*MRFeedback, error) {
	mr, err := c.GetMR(ctx, projectPath, iid)
	if err != nil {
		return nil, err
	}
	d, _ := c.ListMRDiscussions(ctx, projectPath, iid, nil)
	return &MRFeedback{
		MR:          mr,
		Approvals:   []MRApproval{},
		Discussions: d,
		Pipelines:   []Pipeline{},
		HasIssues:   hasOpenDiscussions(d),
	}, nil
}

func (c *MockClient) GetMRStatus(ctx context.Context, projectPath string, iid int) (*MRStatus, error) {
	mr, err := c.GetMR(ctx, projectPath, iid)
	if err != nil {
		return nil, err
	}
	return &MRStatus{MR: mr, MergeStatus: mr.MergeStatus}, nil
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
		State:          "open",
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
