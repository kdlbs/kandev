package gitlab

import (
	"context"
	"strings"
	"testing"
)

type captureIssueQueryClient struct {
	*MockClient
	filter      string
	customQuery string
	listCalls   int
}

func (c *captureIssueQueryClient) ListIssues(_ context.Context, filter, customQuery string) ([]*Issue, error) {
	c.filter = filter
	c.customQuery = customQuery
	c.listCalls++
	return nil, nil
}

func TestFetchIssuesRejectsEmptyAuthenticatedUsernameBeforeListing(t *testing.T) {
	client := &captureIssueQueryClient{MockClient: NewMockClient(DefaultHost)}
	client.SetUser("")
	svc := NewService(DefaultHost, client, "mock", nil, newTestLogger(t))
	svc.workspaceClients["ws-1"] = client

	issues, err := svc.fetchIssues(context.Background(), &IssueWatch{WorkspaceID: "ws-1"})
	if err == nil || !strings.Contains(err.Error(), "username") {
		t.Fatalf("err = %v, want missing username error", err)
	}
	if len(issues) != 0 || client.listCalls != 0 {
		t.Fatalf("issues=%v ListIssues calls=%d, want none", issues, client.listCalls)
	}
}

func TestFetchIssuesDefaultsToAuthenticatedAssignee(t *testing.T) {
	client := &captureIssueQueryClient{MockClient: NewMockClient(DefaultHost)}
	client.SetUser("issue owner")
	svc := NewService(DefaultHost, client, "mock", nil, newTestLogger(t))
	svc.workspaceClients["ws-1"] = client

	if _, err := svc.fetchIssues(context.Background(), &IssueWatch{WorkspaceID: "ws-1"}); err != nil {
		t.Fatalf("fetch issues: %v", err)
	}
	if !strings.Contains(client.filter, "assignee_username=issue+owner") {
		t.Fatalf("filter = %q, want authenticated assignee", client.filter)
	}
	if client.customQuery != "" {
		t.Fatalf("custom query = %q, want empty", client.customQuery)
	}
}

func TestFetchIssuesUsesExplicitCustomQueryWithoutImplicitAssignee(t *testing.T) {
	client := &captureIssueQueryClient{MockClient: NewMockClient(DefaultHost)}
	svc := NewService(DefaultHost, client, "mock", nil, newTestLogger(t))
	svc.workspaceClients["ws-1"] = client
	custom := "state=opened&assignee_username=someone-else"

	if _, err := svc.fetchIssues(context.Background(), &IssueWatch{WorkspaceID: "ws-1", CustomQuery: custom}); err != nil {
		t.Fatalf("fetch issues: %v", err)
	}
	if client.filter != "" || client.customQuery != custom {
		t.Fatalf("filter=%q custom=%q", client.filter, client.customQuery)
	}
}
