package azuredevops

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// MockState is the deterministic E2E data exposed by MockClient.
type MockState struct {
	Authenticated bool                        `json:"authenticated"`
	User          TestConnectionResult        `json:"user"`
	Projects      []Project                   `json:"projects"`
	Repositories  []Repository                `json:"repositories"`
	WorkItems     []WorkItem                  `json:"workItems"`
	PullRequests  []PullRequest               `json:"pullRequests"`
	Feedback      map[int]PullRequestFeedback `json:"feedback"`
}

// MockClient implements Client with in-memory state for browser tests.
type MockClient struct {
	mu    sync.RWMutex
	state MockState
}

func NewMockClient() *MockClient {
	client := &MockClient{}
	client.Seed(MockState{Authenticated: true, User: TestConnectionResult{OK: true, ID: "mock-user", DisplayName: "Mock User"}})
	return client
}

func (c *MockClient) Seed(state MockState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if state.Feedback == nil {
		state.Feedback = make(map[int]PullRequestFeedback)
	}
	c.state = state
}

func (c *MockClient) snapshot() MockState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *MockClient) TestAuth(context.Context) (*TestConnectionResult, error) {
	state := c.snapshot()
	if !state.Authenticated {
		return &TestConnectionResult{OK: false, Error: "401 unauthorized"}, nil
	}
	result := state.User
	result.OK = true
	return &result, nil
}

func (c *MockClient) ListProjects(context.Context) ([]Project, error) {
	return c.snapshot().Projects, nil
}

func (c *MockClient) ListRepositories(_ context.Context, projectID string) ([]Repository, error) {
	state := c.snapshot()
	items := make([]Repository, 0)
	for _, repository := range state.Repositories {
		if projectID == "" || repository.ProjectID == projectID {
			items = append(items, repository)
		}
	}
	return items, nil
}

func (c *MockClient) ListBranches(_ context.Context, projectID, repositoryID string) ([]Branch, error) {
	state := c.snapshot()
	for _, repo := range state.Repositories {
		if repo.ProjectID == projectID && repo.ID == repositoryID {
			return []Branch{{Name: strings.TrimPrefix(repo.DefaultBranch, "refs/heads/")}}, nil
		}
	}
	return []Branch{}, nil
}

func (c *MockClient) QueryWIQL(_ context.Context, projectID, _ string, top int) (*WorkItemSearchResult, error) {
	state := c.snapshot()
	items := make([]WorkItem, 0)
	for _, item := range state.WorkItems {
		if projectID == "" || item.Project == "" || item.Project == projectID {
			items = append(items, item)
		}
		if top > 0 && len(items) >= top {
			break
		}
	}
	return &WorkItemSearchResult{Items: items, Count: len(items)}, nil
}

func (c *MockClient) GetWorkItem(_ context.Context, _ string, id int) (*WorkItem, error) {
	for _, item := range c.snapshot().WorkItems {
		if item.ID == id {
			copy := item
			return &copy, nil
		}
	}
	return nil, mockNotFound("work item", id)
}

func (c *MockClient) ListPullRequests(_ context.Context, filter PullRequestFilter) (*PullRequestPage, error) {
	items := make([]PullRequest, 0)
	for _, pr := range c.snapshot().PullRequests {
		if (filter.ProjectID == "" || pr.ProjectID == filter.ProjectID) &&
			(filter.RepositoryID == "" || pr.RepositoryID == filter.RepositoryID) &&
			(filter.Status == "" || pr.Status == filter.Status) {
			items = append(items, pr)
		}
	}
	return &PullRequestPage{Items: items, Count: len(items), Skip: filter.Skip, Top: filter.Top}, nil
}

func (c *MockClient) GetPullRequest(_ context.Context, _, _ string, id int) (*PullRequest, error) {
	for _, pr := range c.snapshot().PullRequests {
		if pr.ID == id {
			copy := pr
			return &copy, nil
		}
	}
	feedback, ok := c.Feedback(id)
	if ok && feedback.PullRequest != nil {
		return feedback.PullRequest, nil
	}
	return nil, mockNotFound("pull request", id)
}

func (c *MockClient) ListReviewers(_ context.Context, _, _ string, id int) ([]Reviewer, error) {
	feedback, ok := c.Feedback(id)
	if !ok {
		return []Reviewer{}, nil
	}
	return feedback.Reviewers, nil
}

func (c *MockClient) ListThreads(_ context.Context, _, _ string, id int) ([]Thread, error) {
	feedback, ok := c.Feedback(id)
	if !ok {
		return []Thread{}, nil
	}
	return feedback.Threads, nil
}

func (c *MockClient) ListLinkedWorkItems(_ context.Context, _, _ string, id int) ([]WorkItemRef, error) {
	feedback, ok := c.Feedback(id)
	if !ok {
		return []WorkItemRef{}, nil
	}
	return feedback.LinkedWorkItems, nil
}

func (c *MockClient) ListPolicyEvaluations(_ context.Context, _ string, id int) ([]PolicyEvaluation, error) {
	feedback, ok := c.Feedback(id)
	if !ok {
		return []PolicyEvaluation{}, nil
	}
	return feedback.Policies, nil
}

func (c *MockClient) Feedback(id int) (PullRequestFeedback, bool) {
	state := c.snapshot()
	feedback, ok := state.Feedback[id]
	return feedback, ok
}

func mockNotFound(kind string, id int) error {
	return &APIError{StatusCode: 404, Endpoint: "mock", Body: fmt.Sprintf("%s %d not found", kind, id)}
}
