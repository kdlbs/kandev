package jira

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

type mentionRecordingClient struct {
	*fakeClient
	search func(context.Context, string, int) (*SearchResult, error)
}

func (c *mentionRecordingClient) SearchTickets(
	ctx context.Context,
	jql, _ string,
	maxResults int,
) (*SearchResult, error) {
	return c.search(ctx, jql, maxResults)
}

func newMentionService(
	t *testing.T,
	workspaceID, siteURL string,
	client Client,
) *Service {
	t.Helper()
	store := newTestStore(t)
	secrets := newFakeSecretStore()
	ctx := context.Background()
	if err := store.UpsertConfigForWorkspace(ctx, workspaceID, &JiraConfig{
		SiteURL:      siteURL,
		Email:        "user@example.test",
		AuthMethod:   AuthMethodAPIToken,
		InstanceType: InstanceTypeCloud,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := secrets.Set(ctx, SecretKeyForWorkspace(workspaceID), "Jira token", "token"); err != nil {
		t.Fatalf("save secret: %v", err)
	}
	return NewService(store, secrets, func(*JiraConfig, string) Client {
		return client
	}, logger.Default())
}

func TestBuildMentionJQL_EscapesPlainKeyOrTitle(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{name: "key", query: "eng-42", want: `key = "ENG-42" ORDER BY updated DESC`},
		{
			name:  "title cannot inject raw JQL",
			query: `  auth" OR status = Done \ path  `,
			want:  `summary ~ "auth\" OR status = Done \\ path" ORDER BY updated DESC`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := buildMentionJQL(test.query)
			if err != nil {
				t.Fatalf("build JQL: %v", err)
			}
			if got != test.want {
				t.Fatalf("JQL = %q, want %q", got, test.want)
			}
		})
	}

	if _, err := buildMentionJQL(" \n\t "); !errors.Is(err, ErrMentionInvalidQuery) {
		t.Fatalf("blank query error = %v, want ErrMentionInvalidQuery", err)
	}
}

func TestService_SearchMentionTicketsForWorkspace_ProjectsAndCaps(t *testing.T) {
	client := &mentionRecordingClient{fakeClient: &fakeClient{}}
	client.search = func(ctx context.Context, jql string, maxResults int) (*SearchResult, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if jql != `summary ~ "auth" ORDER BY updated DESC` {
			t.Fatalf("JQL = %q", jql)
		}
		if maxResults != 10 {
			t.Fatalf("maxResults = %d, want 10", maxResults)
		}
		tickets := make([]JiraTicket, 0, 12)
		for i := 0; i < 12; i++ {
			tickets = append(tickets, JiraTicket{
				ID:      fmt.Sprintf("100%d", i),
				Key:     fmt.Sprintf("ENG-%d", i),
				Summary: fmt.Sprintf("Auth %d", i),
				URL:     fmt.Sprintf("https://jira.example.test/base/browse/ENG-%d", i),
			})
		}
		return &SearchResult{Tickets: tickets}, nil
	}
	service := newMentionService(t, "workspace-1", "https://jira.example.test/base/", client)

	tickets, err := service.SearchMentionTicketsForWorkspace(
		context.Background(), "workspace-1", "auth", 99,
	)
	if err != nil {
		t.Fatalf("search mentions: %v", err)
	}
	if len(tickets) != 10 {
		t.Fatalf("tickets = %d, want capped 10", len(tickets))
	}
	first := tickets[0]
	if first.ID != "1000" || first.Key != "ENG-0" || first.Title != "Auth 0" ||
		first.URL != "https://jira.example.test/base/browse/ENG-0" ||
		first.SiteURL != "https://jira.example.test/base" {
		t.Fatalf("first projection = %#v", first)
	}
}

func TestService_MentionSearchRequiresExplicitConfiguredWorkspace(t *testing.T) {
	clientCalled := false
	client := &mentionRecordingClient{fakeClient: &fakeClient{}}
	client.search = func(context.Context, string, int) (*SearchResult, error) {
		clientCalled = true
		return &SearchResult{}, nil
	}
	service := newMentionService(t, "default", "https://default.example.test", client)

	if _, err := service.SearchMentionTicketsForWorkspace(context.Background(), "", "auth", 5); !errors.Is(err, ErrMentionWorkspaceRequired) {
		t.Fatalf("blank workspace error = %v, want ErrMentionWorkspaceRequired", err)
	}
	if clientCalled {
		t.Fatal("blank workspace fell back to default Jira config")
	}
	if _, err := service.SearchMentionTicketsForWorkspace(context.Background(), "workspace-2", "auth", 5); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("unconfigured workspace error = %v, want ErrNotConfigured", err)
	}
}

func TestService_MentionSiteURLForWorkspace_NormalizesAndRejectsUnsafeConfig(t *testing.T) {
	client := &mentionRecordingClient{fakeClient: &fakeClient{}, search: func(context.Context, string, int) (*SearchResult, error) {
		return &SearchResult{}, nil
	}}
	service := newMentionService(t, "workspace-1", "https://jira.example.test/base/", client)

	siteURL, err := service.MentionSiteURLForWorkspace(context.Background(), "workspace-1")
	if err != nil {
		t.Fatalf("mention site URL: %v", err)
	}
	if siteURL != "https://jira.example.test/base" {
		t.Fatalf("site URL = %q", siteURL)
	}

	unsafe := newMentionService(t, "workspace-1", "https://jira.example.test/base?token=secret", client)
	if _, err := unsafe.MentionSiteURLForWorkspace(context.Background(), "workspace-1"); !errors.Is(err, ErrMentionInvalidSiteURL) {
		t.Fatalf("unsafe config error = %v, want ErrMentionInvalidSiteURL", err)
	}
}

func TestService_SearchMentionTicketsForWorkspace_PropagatesCancellation(t *testing.T) {
	client := &mentionRecordingClient{fakeClient: &fakeClient{}}
	client.search = func(ctx context.Context, _ string, _ int) (*SearchResult, error) {
		return nil, ctx.Err()
	}
	service := newMentionService(t, "workspace-1", "https://jira.example.test", client)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := service.SearchMentionTicketsForWorkspace(ctx, "workspace-1", "auth", 5)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
}
