package gitlab

import (
	"context"
	"strings"
	"testing"
)

type captureReviewQueryClient struct {
	*MockClient
	filter      string
	customQuery string
}

func (c *captureReviewQueryClient) SearchMRs(_ context.Context, filter, customQuery string) ([]*MR, error) {
	c.filter = filter
	c.customQuery = customQuery
	return nil, nil
}

func TestFetchReviewMRsDefaultsToAuthenticatedReviewer(t *testing.T) {
	client := &captureReviewQueryClient{MockClient: NewMockClient(DefaultHost)}
	client.SetUser("reviewer name")
	svc := NewService(DefaultHost, client, "mock", nil, newTestLogger(t))
	svc.workspaceClients["ws-1"] = client

	if _, err := svc.fetchReviewMRs(context.Background(), &ReviewWatch{WorkspaceID: "ws-1"}); err != nil {
		t.Fatalf("fetch review MRs: %v", err)
	}
	if !strings.Contains(client.filter, "reviewer_username=reviewer+name") {
		t.Fatalf("filter = %q, want authenticated reviewer", client.filter)
	}
	if client.customQuery != "" {
		t.Fatalf("custom query = %q, want empty", client.customQuery)
	}
}

func TestFetchReviewMRsUsesExplicitCustomQueryWithoutImplicitReviewer(t *testing.T) {
	client := &captureReviewQueryClient{MockClient: NewMockClient(DefaultHost)}
	svc := NewService(DefaultHost, client, "mock", nil, newTestLogger(t))
	svc.workspaceClients["ws-1"] = client
	custom := "state=opened&reviewer_username=someone-else"

	if _, err := svc.fetchReviewMRs(context.Background(), &ReviewWatch{WorkspaceID: "ws-1", CustomQuery: custom}); err != nil {
		t.Fatalf("fetch review MRs: %v", err)
	}
	if client.filter != "" || client.customQuery != custom {
		t.Fatalf("filter=%q custom=%q", client.filter, client.customQuery)
	}
}
