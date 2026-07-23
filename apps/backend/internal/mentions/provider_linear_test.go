package mentions

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/linear"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

type fakeLinearMentionService struct {
	searchWorkspaceID string
	searchQuery       string
	searchLimit       int
	issues            []linear.LinearIssue
	orgSlug           string
	searchErr         error
	configWorkspaceID string
	configCalls       int
	configFn          func(string) (*linear.LinearConfig, error)
}

func (s *fakeLinearMentionService) SearchMentionIssues(
	_ context.Context,
	workspaceID, query string,
	limit int,
) ([]linear.LinearIssue, string, error) {
	s.searchWorkspaceID = workspaceID
	s.searchQuery = query
	s.searchLimit = limit
	return s.issues, s.orgSlug, s.searchErr
}

func (s *fakeLinearMentionService) GetConfigForWorkspace(
	_ context.Context,
	workspaceID string,
) (*linear.LinearConfig, error) {
	s.configWorkspaceID = workspaceID
	s.configCalls++
	if s.configFn != nil {
		return s.configFn(workspaceID)
	}
	return &linear.LinearConfig{WorkspaceID: workspaceID, OrgSlug: s.orgSlug}, nil
}

func TestLinearProviderSearchMapsImmutableIssueIdentityAndScope(t *testing.T) {
	service := &fakeLinearMentionService{
		orgSlug: "acme",
		issues: []linear.LinearIssue{{
			ID:         "issue-uuid",
			Identifier: "ENG-123",
			Title:      "Fix authentication",
			URL:        "https://linear.app/acme/issue/ENG-123/fix-authentication",
		}},
	}
	provider := NewLinearProvider(service)

	descriptor := provider.Descriptor()
	if descriptor.Source != "linear_issues" || descriptor.Provider != "linear" ||
		descriptor.Kind != "issue" || descriptor.DisplayName != "Linear" ||
		descriptor.KindLabel != "Issue" || descriptor.Order != 30 {
		t.Fatalf("descriptor = %#v", descriptor)
	}
	candidates, err := provider.Search(context.Background(), SearchRequest{
		WorkspaceID: "workspace-1",
		Query:       "auth",
		Limit:       7,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if service.searchWorkspaceID != "workspace-1" || service.searchQuery != "auth" || service.searchLimit != 7 {
		t.Fatalf("delegated search = workspace %q query %q limit %d",
			service.searchWorkspaceID, service.searchQuery, service.searchLimit)
	}
	want := Candidate{
		ID:    "issue-uuid",
		Key:   "ENG-123",
		Title: "Fix authentication",
		URL:   "https://linear.app/acme/issue/ENG-123/fix-authentication",
		Scope: "acme",
	}
	if len(candidates) != 1 || candidates[0] != want {
		t.Fatalf("candidates = %#v, want %#v", candidates, want)
	}
}

func TestLinearProviderSearchUsesAuthorizerToDropUnsafeDestinations(t *testing.T) {
	service := &fakeLinearMentionService{
		orgSlug: "acme",
		issues: []linear.LinearIssue{
			{
				ID: "safe-id", Identifier: "ENG-1", Title: "Safe",
				URL: "https://linear.app/acme/issue/ENG-1/safe",
			},
			{
				ID: "evil-id", Identifier: "ENG-2", Title: "Unsafe",
				URL: "https://linear.app.evil.test/acme/issue/ENG-2/unsafe",
			},
		},
	}
	provider := NewLinearProvider(service)
	registry := NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatalf("register: %v", err)
	}

	response, err := NewService(registry).Search(context.Background(), SearchRequest{
		WorkspaceID: "workspace-1",
		Query:       "issue",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(response.Groups) != 1 || len(response.Groups[0].Results) != 1 ||
		response.Groups[0].Results[0].ID != "safe-id" {
		t.Fatalf("groups = %#v, want only canonical Linear destination", response.Groups)
	}
	if service.configCalls != 0 {
		t.Fatalf("search authorization repeated config lookup %d times", service.configCalls)
	}
}

func TestLinearProviderMapsFailuresToSafeStatuses(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status Status
	}{
		{name: "not configured", err: linear.ErrNotConfigured, status: StatusNotConfigured},
		{name: "workspace required", err: linear.ErrMentionWorkspaceRequired, status: StatusUnsupportedScope},
		{name: "organization unavailable", err: linear.ErrMentionScopeUnavailable, status: StatusUnsupportedScope},
		{name: "unauthorized", err: &linear.APIError{StatusCode: 401, Message: "secret upstream body"}, status: StatusUnauthorized},
		{name: "forbidden", err: &linear.APIError{StatusCode: 403, Message: "secret upstream body"}, status: StatusUnauthorized},
		{name: "rate limited", err: &linear.APIError{StatusCode: 429, Message: "secret upstream body"}, status: StatusRateLimited},
		{name: "request timeout", err: &linear.APIError{StatusCode: 408, Message: "secret upstream body"}, status: StatusTimeout},
		{name: "gateway timeout", err: &linear.APIError{StatusCode: 504, Message: "secret upstream body"}, status: StatusTimeout},
		{name: "deadline", err: context.DeadlineExceeded, status: StatusTimeout},
		{name: "canceled", err: context.Canceled, status: StatusTimeout},
		{name: "upstream", err: errors.New("secret upstream body"), status: StatusUpstreamError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := NewRegistry()
			if err := registry.Register(NewLinearProvider(&fakeLinearMentionService{searchErr: test.err})); err != nil {
				t.Fatalf("register: %v", err)
			}
			response, err := NewService(registry).Search(context.Background(), SearchRequest{
				WorkspaceID: "workspace-1",
				Query:       "auth",
			})
			if err != nil {
				t.Fatalf("aggregate search: %v", err)
			}
			if len(response.Groups) != 1 || response.Groups[0].Status != test.status {
				t.Fatalf("groups = %#v, want status %q", response.Groups, test.status)
			}
		})
	}
}

func TestLinearProviderPreservesCancellation(t *testing.T) {
	provider := NewLinearProvider(&fakeLinearMentionService{searchErr: context.Canceled})
	_, err := provider.Search(context.Background(), SearchRequest{
		WorkspaceID: "workspace-1",
		Query:       "auth",
		Limit:       DefaultLimit,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want wrapped cancellation", err)
	}
}

func TestLinearProviderCapsCandidates(t *testing.T) {
	issues := make([]linear.LinearIssue, 12)
	for index := range issues {
		issues[index] = linear.LinearIssue{
			ID:         string(rune('a' + index)),
			Identifier: string(rune('A' + index)),
			Title:      "Issue",
			URL:        "https://linear.app/acme/issue/ENG-1",
		}
	}
	service := &fakeLinearMentionService{issues: issues, orgSlug: "acme"}
	provider := NewLinearProvider(service)

	candidates, err := provider.Search(context.Background(), SearchRequest{
		WorkspaceID: "workspace-1",
		Query:       "auth",
		Limit:       -1,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if service.searchLimit != 1 || len(candidates) != 1 {
		t.Fatalf("service limit = %d, candidates = %d, want minimum cap 1",
			service.searchLimit, len(candidates))
	}
}

func TestLinearProviderAuthorizesConfiguredWorkspaceOrganizationOnSubmission(t *testing.T) {
	service := &fakeLinearMentionService{
		orgSlug: "acme",
		issues: []linear.LinearIssue{{
			ID: "issue-uuid", Identifier: "ENG-123", Title: "Fix authentication",
			URL: "https://linear.app/acme/issue/ENG-123/fix-authentication",
		}},
		configFn: func(workspaceID string) (*linear.LinearConfig, error) {
			orgSlug := "acme"
			if workspaceID == "workspace-2" {
				orgSlug = "other-org"
			}
			return &linear.LinearConfig{WorkspaceID: workspaceID, OrgSlug: orgSlug}, nil
		},
	}
	provider := NewLinearProvider(service)
	authorizer, ok := provider.(ReferenceAuthorizer)
	if !ok {
		t.Fatal("Linear provider must authorize references")
	}
	registry := NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatalf("register: %v", err)
	}
	response, err := NewService(registry).Search(context.Background(), SearchRequest{
		WorkspaceID: "workspace-1",
		Query:       "auth",
	})
	if err != nil || len(response.Groups) != 1 || len(response.Groups[0].Results) != 1 {
		t.Fatalf("search response = %#v, err = %v", response, err)
	}
	ref := response.Groups[0].Results[0]

	if err := registry.AuthorizeReference(context.Background(), ReferenceAuthorizationRequest{
		WorkspaceID: "workspace-1",
		Purpose:     ReferencePurposeSubmission,
		Reference:   ref,
	}); err != nil {
		t.Fatalf("authorize valid submission: %v", err)
	}
	if service.configWorkspaceID != "workspace-1" || service.configCalls != 1 {
		t.Fatalf("config lookup = workspace %q calls %d", service.configWorkspaceID, service.configCalls)
	}

	tests := []struct {
		name        string
		workspaceID string
		mutate      func(*apiv1.EntityReference)
	}{
		{name: "blank workspace", workspaceID: " "},
		{name: "different configured organization", workspaceID: "workspace-2"},
		{name: "wrong scope", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.Scope = "other-org"
			ref.Ref = canonicalRef(ref.Provider, ref.Kind, ref.Scope, ref.ID)
		}},
		{name: "wrong canonical ref", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.Ref = "mention:v1:linear:issue:acme:other-id"
		}},
		{name: "http destination", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.URL = "http://linear.app/acme/issue/ENG-123/fix-authentication"
		}},
		{name: "lookalike host", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.URL = "https://linear.app.evil.test/acme/issue/ENG-123/fix-authentication"
		}},
		{name: "host port", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.URL = "https://linear.app:443/acme/issue/ENG-123/fix-authentication"
		}},
		{name: "wrong organization path", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.URL = "https://linear.app/other-org/issue/ENG-123/fix-authentication"
		}},
		{name: "wrong issue key path", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.URL = "https://linear.app/acme/issue/ENG-999/fix-authentication"
		}},
		{name: "encoded path separator", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.URL = "https://linear.app/acme%2Fevil/issue/ENG-123/fix-authentication"
		}},
		{name: "query string", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.URL += "?token=secret"
		}},
		{name: "fragment", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.URL += "#fragment"
		}},
		{name: "wrong provider", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.Provider = "github"
		}},
		{name: "wrong kind", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.Kind = "pull_request"
		}},
		{name: "wrong version", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.Version = apiv1.EntityReferenceVersion + 1
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := ref
			if test.mutate != nil {
				test.mutate(&candidate)
			}
			if err := authorizer.AuthorizeReference(context.Background(), ReferenceAuthorizationRequest{
				WorkspaceID: test.workspaceID,
				Purpose:     ReferencePurposeSubmission,
				Reference:   candidate,
			}); !errors.Is(err, ErrReferenceUnauthorized) {
				t.Fatalf("error = %v, want unauthorized", err)
			}
		})
	}
}

func TestLinearProviderSubmissionFailsClosedWhenConfigurationUnavailable(t *testing.T) {
	service := &fakeLinearMentionService{
		orgSlug: "acme",
		configFn: func(string) (*linear.LinearConfig, error) {
			return nil, linear.ErrNotConfigured
		},
	}
	provider := NewLinearProvider(service)
	authorizer, ok := provider.(ReferenceAuthorizer)
	if !ok {
		t.Fatal("Linear provider must authorize references")
	}
	ref := apiv1.EntityReference{
		Version:  apiv1.EntityReferenceVersion,
		Ref:      canonicalRef("linear", "issue", "acme", "issue-uuid"),
		Provider: "linear", Kind: "issue", ID: "issue-uuid", Key: "ENG-123",
		Title: "Fix authentication", URL: "https://linear.app/acme/issue/ENG-123", Scope: "acme",
	}
	if err := authorizer.AuthorizeReference(context.Background(), ReferenceAuthorizationRequest{
		WorkspaceID: "workspace-1",
		Purpose:     ReferencePurposeSubmission,
		Reference:   ref,
	}); !errors.Is(err, ErrReferenceUnauthorized) {
		t.Fatalf("error = %v, want fail-closed unauthorized", err)
	}
}

func TestLinearProviderSubmissionRejectsConfigurationFromAnotherWorkspace(t *testing.T) {
	service := &fakeLinearMentionService{
		orgSlug: "acme",
		configFn: func(string) (*linear.LinearConfig, error) {
			return &linear.LinearConfig{WorkspaceID: "workspace-2", OrgSlug: "acme"}, nil
		},
	}
	provider := NewLinearProvider(service)
	authorizer, ok := provider.(ReferenceAuthorizer)
	if !ok {
		t.Fatal("Linear provider must authorize references")
	}
	ref := apiv1.EntityReference{
		Version:  apiv1.EntityReferenceVersion,
		Ref:      canonicalRef("linear", "issue", "acme", "issue-uuid"),
		Provider: "linear", Kind: "issue", ID: "issue-uuid", Key: "ENG-123",
		Title: "Fix authentication", URL: "https://linear.app/acme/issue/ENG-123", Scope: "acme",
	}
	if err := authorizer.AuthorizeReference(context.Background(), ReferenceAuthorizationRequest{
		WorkspaceID: "workspace-1",
		Purpose:     ReferencePurposeSubmission,
		Reference:   ref,
	}); !errors.Is(err, ErrReferenceUnauthorized) {
		t.Fatalf("error = %v, want cross-workspace config rejected", err)
	}
}
