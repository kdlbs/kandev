package mentions

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/jira"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

type fakeJiraMentionService struct {
	search func(context.Context, string, string, int) ([]jira.MentionTicket, error)
	site   func(context.Context, string) (string, error)
}

func (s *fakeJiraMentionService) SearchMentionTicketsForWorkspace(
	ctx context.Context,
	workspaceID, query string,
	limit int,
) ([]jira.MentionTicket, error) {
	return s.search(ctx, workspaceID, query, limit)
}

func (s *fakeJiraMentionService) MentionSiteURLForWorkspace(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	return s.site(ctx, workspaceID)
}

func TestJiraProviderSearchMapsImmutableWorkspaceProjection(t *testing.T) {
	backend := &fakeJiraMentionService{
		search: func(_ context.Context, workspaceID, query string, limit int) ([]jira.MentionTicket, error) {
			if workspaceID != "workspace-1" || query != "auth" || limit != 4 {
				t.Fatalf("delegated search = workspace %q query %q limit %d", workspaceID, query, limit)
			}
			return []jira.MentionTicket{{
				ID:      "10042",
				Key:     "ENG-42",
				Title:   "Fix authentication",
				URL:     "https://jira.example.test/base/browse/ENG-42",
				SiteURL: "https://jira.example.test/base",
			}}, nil
		},
		site: func(_ context.Context, workspaceID string) (string, error) {
			if workspaceID != "workspace-1" {
				t.Fatalf("authorization workspace = %q", workspaceID)
			}
			return "https://jira.example.test/base", nil
		},
	}
	provider := NewJiraProvider(backend)
	descriptor := provider.Descriptor()
	if descriptor.Source != "jira_issues" || descriptor.Provider != "jira" || descriptor.Kind != "issue" ||
		descriptor.DisplayName != "Jira" || descriptor.KindLabel != "Issue" {
		t.Fatalf("descriptor = %#v", descriptor)
	}
	registry := NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatalf("register: %v", err)
	}
	response, err := NewService(registry).Search(context.Background(), SearchRequest{
		WorkspaceID: "workspace-1",
		Query:       "auth",
		Limit:       4,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(response.Groups) != 1 || len(response.Groups[0].Results) != 1 {
		t.Fatalf("groups = %#v", response.Groups)
	}
	result := response.Groups[0].Results[0]
	if result.ID != "10042" || result.Key != "ENG-42" || result.Title != "Fix authentication" ||
		result.URL != "https://jira.example.test/base/browse/ENG-42" ||
		result.Scope != "https://jira.example.test/base" ||
		result.Ref != canonicalRef("jira", "issue", "https://jira.example.test/base", "10042") {
		t.Fatalf("result = %#v", result)
	}
}

func TestJiraProviderSearchFiltersForeignConfiguredOrigin(t *testing.T) {
	backend := &fakeJiraMentionService{
		search: func(context.Context, string, string, int) ([]jira.MentionTicket, error) {
			return []jira.MentionTicket{{
				ID: "10042", Key: "ENG-42", Title: "Spoofed",
				URL: "https://evil.example.test/browse/ENG-42", SiteURL: "https://jira.example.test/base",
			}}, nil
		},
		site: func(context.Context, string) (string, error) {
			return "https://jira.example.test/base", nil
		},
	}
	registry := NewRegistry()
	if err := registry.Register(NewJiraProvider(backend)); err != nil {
		t.Fatalf("register: %v", err)
	}
	response, err := NewService(registry).Search(context.Background(), SearchRequest{
		WorkspaceID: "workspace-1", Query: "auth",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if got := len(response.Groups[0].Results); got != 0 {
		t.Fatalf("foreign-origin results = %d, want 0", got)
	}
}

func TestJiraProviderMapsSafePartialStatuses(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want Status
	}{
		{name: "not configured", err: jira.ErrNotConfigured, want: StatusNotConfigured},
		{name: "unauthorized", err: &jira.APIError{StatusCode: http.StatusUnauthorized, Message: "secret body"}, want: StatusUnauthorized},
		{name: "login redirect", err: &jira.APIError{StatusCode: http.StatusFound, Message: "secret body"}, want: StatusUnauthorized},
		{name: "rate limited", err: &jira.APIError{StatusCode: http.StatusTooManyRequests, Message: "secret body"}, want: StatusRateLimited},
		{name: "gateway timeout", err: &jira.APIError{StatusCode: http.StatusGatewayTimeout, Message: "secret body"}, want: StatusTimeout},
		{name: "context timeout", err: context.DeadlineExceeded, want: StatusTimeout},
		{name: "upstream", err: errors.New("secret upstream response"), want: StatusUpstreamError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := &fakeJiraMentionService{
				search: func(context.Context, string, string, int) ([]jira.MentionTicket, error) {
					return nil, test.err
				},
				site: func(context.Context, string) (string, error) {
					return "https://jira.example.test", nil
				},
			}
			registry := NewRegistry()
			if err := registry.Register(NewJiraProvider(backend)); err != nil {
				t.Fatalf("register: %v", err)
			}
			response, err := NewService(registry).Search(context.Background(), SearchRequest{
				WorkspaceID: "workspace-1", Query: "auth",
			})
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if len(response.Groups) != 1 || response.Groups[0].Status != test.want {
				t.Fatalf("groups = %#v, want status %q", response.Groups, test.want)
			}
		})
	}
}

func TestJiraProviderWithoutServiceIsNotConfigured(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(NewJiraProvider(nil)); err != nil {
		t.Fatalf("register: %v", err)
	}
	response, err := NewService(registry).Search(context.Background(), SearchRequest{
		WorkspaceID: "workspace-1", Query: "auth",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if response.Groups[0].Status != StatusNotConfigured {
		t.Fatalf("status = %q", response.Groups[0].Status)
	}
}

func TestJiraProviderAuthorizesExactConfiguredSiteForSearchAndSubmission(t *testing.T) {
	siteCalls := 0
	backend := &fakeJiraMentionService{
		search: func(context.Context, string, string, int) ([]jira.MentionTicket, error) {
			return nil, nil
		},
		site: func(_ context.Context, workspaceID string) (string, error) {
			siteCalls++
			if workspaceID == "" {
				t.Fatal("blank workspace fell back to Jira default config")
			}
			if workspaceID != "workspace-1" {
				return "https://other.example.test", nil
			}
			return "https://jira.example.test/base", nil
		},
	}
	provider := NewJiraProvider(backend)
	authorizer, ok := provider.(ReferenceAuthorizer)
	if !ok {
		t.Fatal("Jira provider must implement ReferenceAuthorizer")
	}
	reference := apiv1.EntityReference{
		Version:  apiv1.EntityReferenceVersion,
		Ref:      canonicalRef("jira", "issue", "https://jira.example.test/base", "10042"),
		Provider: "jira", Kind: "issue", ID: "10042", Key: "ENG-42", Title: "Fix auth",
		URL: "https://jira.example.test/base/browse/ENG-42", Scope: "https://jira.example.test/base",
	}
	for _, purpose := range []ReferencePurpose{ReferencePurposeSearch, ReferencePurposeSubmission} {
		if err := authorizer.AuthorizeReference(context.Background(), ReferenceAuthorizationRequest{
			WorkspaceID: "workspace-1", Purpose: purpose, Reference: reference,
		}); err != nil {
			t.Fatalf("authorize %s: %v", purpose, err)
		}
	}

	tests := []struct {
		name        string
		workspaceID string
		mutate      func(*apiv1.EntityReference)
	}{
		{name: "blank workspace", workspaceID: ""},
		{name: "other workspace", workspaceID: "workspace-2"},
		{name: "wrong scope", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.Scope = "https://jira.example.test"
		}},
		{name: "wrong origin", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.URL = "https://evil.example.test/base/browse/ENG-42"
		}},
		{name: "wrong base path", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.URL = "https://jira.example.test/browse/ENG-42"
		}},
		{name: "URL query", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.URL += "?token=secret"
		}},
		{name: "wrong canonical ref", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.Ref = "mention:v1:jira:issue:spoofed"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ref := reference
			if test.mutate != nil {
				test.mutate(&ref)
			}
			err := authorizer.AuthorizeReference(context.Background(), ReferenceAuthorizationRequest{
				WorkspaceID: test.workspaceID,
				Purpose:     ReferencePurposeSubmission,
				Reference:   ref,
			})
			if !errors.Is(err, ErrReferenceUnauthorized) {
				t.Fatalf("error = %v, want ErrReferenceUnauthorized", err)
			}
		})
	}
	if siteCalls == 0 {
		t.Fatal("configured Jira site was never checked")
	}
}
