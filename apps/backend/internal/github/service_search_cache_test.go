package github

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

type countingSearchClient struct {
	*MockClient
	prCalls    atomic.Int32
	issueCalls atomic.Int32
}

func (c *countingSearchClient) SearchPRsPaged(ctx context.Context, filter, customQuery string, page, perPage int) (*PRSearchPage, error) {
	c.prCalls.Add(1)
	return c.MockClient.SearchPRsPaged(ctx, filter, customQuery, page, perPage)
}

func (c *countingSearchClient) ListIssuesPaged(ctx context.Context, filter, customQuery string, page, perPage int) (*IssueSearchPage, error) {
	c.issueCalls.Add(1)
	return c.MockClient.ListIssuesPaged(ctx, filter, customQuery, page, perPage)
}

func newTestService(client Client) *Service {
	return &Service{
		client:        client,
		authMethod:    AuthMethodPAT,
		logger:        logger.Default(),
		searchCache:   newTTLCache(),
		prStatusCache: newTTLCache(),
	}
}

func TestService_SearchUserPRsPaged_UsesCache(t *testing.T) {
	client := &countingSearchClient{MockClient: NewMockClient()}
	svc := newTestService(client)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, err := svc.SearchUserPRsPaged(ctx, "", "is:open author:@me", 1, 25); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if got := client.prCalls.Load(); got != 1 {
		t.Fatalf("expected 1 upstream PR call, got %d", got)
	}

	if _, err := svc.SearchUserPRsPaged(ctx, "", "is:open author:@me", 2, 25); err != nil {
		t.Fatal(err)
	}
	if got := client.prCalls.Load(); got != 2 {
		t.Fatalf("expected new fetch for different page, got %d", got)
	}
}

func TestService_SearchUserIssuesPaged_UsesCache(t *testing.T) {
	client := &countingSearchClient{MockClient: NewMockClient()}
	svc := newTestService(client)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, err := svc.SearchUserIssuesPaged(ctx, "", "is:open", 1, 25); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if got := client.issueCalls.Load(); got != 1 {
		t.Fatalf("expected 1 upstream issue call, got %d", got)
	}
}

func TestService_SearchUserPRsPaged_PRsIssuesIsolated(t *testing.T) {
	client := &countingSearchClient{MockClient: NewMockClient()}
	svc := newTestService(client)
	ctx := context.Background()

	if _, err := svc.SearchUserPRsPaged(ctx, "", "q", 1, 25); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SearchUserIssuesPaged(ctx, "", "q", 1, 25); err != nil {
		t.Fatal(err)
	}
	if got := client.prCalls.Load(); got != 1 {
		t.Fatalf("pr calls = %d", got)
	}
	if got := client.issueCalls.Load(); got != 1 {
		t.Fatalf("issue calls = %d", got)
	}
}
