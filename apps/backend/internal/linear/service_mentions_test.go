package linear

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func configureMentionWorkspace(t *testing.T, fixture *svcFixture, workspaceID, orgSlug string) {
	t.Helper()
	ctx := context.Background()
	if err := fixture.store.UpsertConfigForWorkspace(ctx, workspaceID, &LinearConfig{
		AuthMethod: AuthMethodAPIKey,
	}); err != nil {
		t.Fatalf("upsert config: %v", err)
	}
	if err := fixture.store.UpdateAuthHealthForWorkspace(
		ctx,
		workspaceID,
		true,
		"",
		orgSlug,
		time.Now().UTC(),
	); err != nil {
		t.Fatalf("update auth health: %v", err)
	}
	if err := fixture.secrets.Set(
		ctx,
		SecretKeyForWorkspace(workspaceID),
		"Linear API key",
		"lin_api_test",
	); err != nil {
		t.Fatalf("store secret: %v", err)
	}
}

func TestServiceSearchMentionIssuesUsesStructuredQueryAndExplicitWorkspace(t *testing.T) {
	fixture := newSvcFixture(t)
	configureMentionWorkspace(t, fixture, "workspace-1", "acme")
	configureMentionWorkspace(t, fixture, "workspace-2", "other")

	wantIssues := []LinearIssue{{
		ID:         "issue-uuid",
		Identifier: "ENG-123",
		Title:      "Fix authentication",
		URL:        "https://linear.app/acme/issue/ENG-123/fix-authentication",
	}}
	var gotFilter SearchFilter
	var gotPageToken string
	var gotLimit int
	fixture.client.searchIssuesFn = func(filter SearchFilter, pageToken string, limit int) (*SearchResult, error) {
		gotFilter = filter
		gotPageToken = pageToken
		gotLimit = limit
		return &SearchResult{Issues: wantIssues}, nil
	}

	issues, orgSlug, err := fixture.svc.SearchMentionIssues(
		context.Background(),
		"workspace-1",
		"  auth  ",
		99,
	)
	if err != nil {
		t.Fatalf("search mention issues: %v", err)
	}
	if !reflect.DeepEqual(gotFilter, SearchFilter{Query: "auth"}) {
		t.Fatalf("filter = %#v, want structured query only", gotFilter)
	}
	if gotPageToken != "" || gotLimit != 10 {
		t.Fatalf("pagination = token %q limit %d, want first page capped at 10", gotPageToken, gotLimit)
	}
	if orgSlug != "acme" {
		t.Fatalf("org scope = %q, want acme", orgSlug)
	}
	if len(issues) != 1 || !reflect.DeepEqual(issues[0], wantIssues[0]) {
		t.Fatalf("issues = %#v, want immutable identity fields preserved", issues)
	}
}

func TestServiceSearchMentionIssuesRejectsBlankWorkspaceWithoutDefaultFallback(t *testing.T) {
	fixture := newSvcFixture(t)
	configureMentionWorkspace(t, fixture, "default", "default-org")

	_, _, err := fixture.svc.SearchMentionIssues(
		context.Background(),
		" \t ",
		"auth",
		5,
	)
	if err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("error = %v, want explicit workspace rejection", err)
	}
	if got := fixture.factoryHit.Load(); got != 0 {
		t.Fatalf("client factory called %d times; blank workspace must not use default config", got)
	}
}

func TestServiceSearchMentionIssuesRequiresOrganizationScope(t *testing.T) {
	fixture := newSvcFixture(t)
	configureMentionWorkspace(t, fixture, "workspace-1", "")

	_, _, err := fixture.svc.SearchMentionIssues(
		context.Background(),
		"workspace-1",
		"auth",
		5,
	)
	if err == nil || !strings.Contains(err.Error(), "organization") {
		t.Fatalf("error = %v, want missing organization scope rejection", err)
	}
	if got := fixture.factoryHit.Load(); got != 0 {
		t.Fatalf("client factory called %d times; incomplete scope must fail before search", got)
	}
}

func TestServiceSearchMentionIssuesWithoutConfigIsNotConfigured(t *testing.T) {
	fixture := newSvcFixture(t)

	_, _, err := fixture.svc.SearchMentionIssues(
		context.Background(),
		"workspace-1",
		"auth",
		5,
	)
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("error = %v, want not configured", err)
	}
	if got := fixture.factoryHit.Load(); got != 0 {
		t.Fatalf("client factory called %d times without config", got)
	}
}

func TestServiceSearchMentionIssuesPreservesCancellation(t *testing.T) {
	fixture := newSvcFixture(t)
	configureMentionWorkspace(t, fixture, "workspace-1", "acme")
	fixture.client.searchIssuesFn = func(SearchFilter, string, int) (*SearchResult, error) {
		return nil, context.Canceled
	}

	_, _, err := fixture.svc.SearchMentionIssues(
		context.Background(),
		"workspace-1",
		"auth",
		5,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want wrapped context cancellation", err)
	}
}

func TestServiceSearchMentionIssuesCapsUpstreamResults(t *testing.T) {
	tests := []struct {
		name      string
		limit     int
		wantLimit int
	}{
		{name: "default", limit: 0, wantLimit: 5},
		{name: "minimum", limit: -1, wantLimit: 1},
		{name: "maximum", limit: 99, wantLimit: 10},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newSvcFixture(t)
			configureMentionWorkspace(t, fixture, "workspace-1", "acme")
			upstreamIssues := make([]LinearIssue, 12)
			for index := range upstreamIssues {
				upstreamIssues[index] = LinearIssue{ID: string(rune('a' + index))}
			}
			gotLimit := 0
			fixture.client.searchIssuesFn = func(_ SearchFilter, _ string, intLimit int) (*SearchResult, error) {
				gotLimit = intLimit
				return &SearchResult{Issues: upstreamIssues}, nil
			}

			issues, _, err := fixture.svc.SearchMentionIssues(
				context.Background(),
				"workspace-1",
				"auth",
				test.limit,
			)
			if err != nil {
				t.Fatalf("search mention issues: %v", err)
			}
			if gotLimit != test.wantLimit || len(issues) != test.wantLimit {
				t.Fatalf("client limit = %d, returned issues = %d, want cap %d",
					gotLimit, len(issues), test.wantLimit)
			}
		})
	}
}
