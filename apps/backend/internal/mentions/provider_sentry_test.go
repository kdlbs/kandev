package mentions

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/sentry"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

type fakeSentryMentionService struct {
	searchWorkspaceID string
	searchQuery       string
	searchLimit       int
	issues            []sentry.MentionIssue
	searchErr         error
	lookupWorkspaceID string
	lookupInstanceID  string
	lookupCalls       int
	lookupFn          func(string, string) (*sentry.MentionInstance, error)
}

func (s *fakeSentryMentionService) SearchMentionIssues(
	_ context.Context,
	workspaceID, query string,
	limit int,
) ([]sentry.MentionIssue, error) {
	s.searchWorkspaceID = workspaceID
	s.searchQuery = query
	s.searchLimit = limit
	return s.issues, s.searchErr
}

func (s *fakeSentryMentionService) MentionInstanceForWorkspace(
	_ context.Context,
	workspaceID, instanceID string,
) (*sentry.MentionInstance, error) {
	s.lookupWorkspaceID = workspaceID
	s.lookupInstanceID = instanceID
	s.lookupCalls++
	if s.lookupFn != nil {
		return s.lookupFn(workspaceID, instanceID)
	}
	return nil, sentry.ErrInstanceNotFound
}

func TestSentryIssueProviderMapsStableInstanceOriginScope(t *testing.T) {
	service := &fakeSentryMentionService{issues: []sentry.MentionIssue{
		{
			InstanceID: "instance-a", InstanceOrigin: "https://sentry.example", OrganizationSlug: "acme",
			ID: "42", ShortID: "APP-1", Title: "Panic",
			URL: "https://sentry.example/organizations/acme/issues/42/",
		},
		{
			InstanceID: "instance-b", InstanceOrigin: "http://self-hosted.example:9000", OrganizationSlug: "internal",
			ID: "42", ShortID: "OPS-1", Title: "Panic",
			URL: "http://self-hosted.example:9000/issues/42/",
		},
	}}
	provider := NewSentryIssueProvider(service)

	descriptor := provider.Descriptor()
	if descriptor.Source != "sentry_issues" || descriptor.Provider != "sentry" ||
		descriptor.Kind != "issue" || descriptor.DisplayName != "Sentry" ||
		descriptor.KindLabel != "Issue" || descriptor.Order != 70 {
		t.Fatalf("descriptor = %#v", descriptor)
	}
	candidates, err := provider.Search(context.Background(), SearchRequest{
		WorkspaceID: "workspace-1",
		Query:       "panic",
		Limit:       2,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if service.searchWorkspaceID != "workspace-1" || service.searchQuery != "panic" || service.searchLimit != 2 {
		t.Fatalf("delegated search = workspace %q query %q limit %d",
			service.searchWorkspaceID, service.searchQuery, service.searchLimit)
	}
	if len(candidates) != 2 || candidates[0].ID != "42" || candidates[1].ID != "42" ||
		candidates[0].Scope != "instance-a|https://sentry.example" ||
		candidates[1].Scope != "instance-b|http://self-hosted.example:9000" ||
		candidates[0].Scope == candidates[1].Scope {
		t.Fatalf("candidates = %#v, want collision-proof instance+origin scopes", candidates)
	}
}

func TestSentryIssueProviderSearchDropsUnsafeDestinationsThroughAuthorizer(t *testing.T) {
	service := &fakeSentryMentionService{issues: []sentry.MentionIssue{
		{
			InstanceID: "instance-a", InstanceOrigin: "https://sentry.example", OrganizationSlug: "acme",
			ID: "42", ShortID: "APP-1", Title: "Safe",
			URL: "https://sentry.example/organizations/acme/issues/42/",
		},
		{
			InstanceID: "instance-a", InstanceOrigin: "https://sentry.example", OrganizationSlug: "acme",
			ID: "43", ShortID: "APP-2", Title: "Evil host",
			URL: "https://sentry.example.evil.test/issues/43/",
		},
		{
			InstanceID: "instance-a", InstanceOrigin: "https://sentry.example", OrganizationSlug: "acme",
			ID: "44", ShortID: "APP-3", Title: "Wrong path",
			URL: "https://sentry.example/projects/acme/issues/44/",
		},
	}}
	registry := NewRegistry()
	if err := registry.Register(NewSentryIssueProvider(service)); err != nil {
		t.Fatalf("register: %v", err)
	}

	response, err := NewService(registry).Search(context.Background(), SearchRequest{
		WorkspaceID: "workspace-1",
		Query:       "panic",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(response.Groups) != 1 || len(response.Groups[0].Results) != 1 || response.Groups[0].Results[0].ID != "42" {
		t.Fatalf("groups = %#v, want one exact Sentry destination", response.Groups)
	}
	if service.lookupCalls != 0 {
		t.Fatalf("search authorization repeated instance lookup %d times", service.lookupCalls)
	}
}

func TestSentryIssueProviderMapsFailuresToSafeStatuses(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status Status
	}{
		{name: "not configured", err: sentry.ErrNotConfigured, status: StatusNotConfigured},
		{name: "workspace required", err: sentry.ErrMentionWorkspaceRequired, status: StatusUnsupportedScope},
		{name: "scope overflow", err: sentry.ErrMentionScopeLimit, status: StatusUnsupportedScope},
		{name: "invalid origin", err: sentry.ErrMentionInvalidOrigin, status: StatusUnsupportedScope},
		{name: "instance missing", err: sentry.ErrInstanceNotFound, status: StatusUnsupportedScope},
		{name: "unauthorized", err: &sentry.APIError{StatusCode: 401, Message: "secret body"}, status: StatusUnauthorized},
		{name: "forbidden", err: &sentry.APIError{StatusCode: 403, Message: "secret body"}, status: StatusUnauthorized},
		{name: "rate limited", err: &sentry.APIError{StatusCode: 429, Message: "secret body"}, status: StatusRateLimited},
		{name: "request timeout", err: &sentry.APIError{StatusCode: 408, Message: "secret body"}, status: StatusTimeout},
		{name: "gateway timeout", err: &sentry.APIError{StatusCode: 504, Message: "secret body"}, status: StatusTimeout},
		{name: "deadline", err: context.DeadlineExceeded, status: StatusTimeout},
		{name: "canceled", err: context.Canceled, status: StatusTimeout},
		{name: "upstream", err: errors.New("secret body"), status: StatusUpstreamError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := NewRegistry()
			if err := registry.Register(NewSentryIssueProvider(&fakeSentryMentionService{searchErr: test.err})); err != nil {
				t.Fatalf("register: %v", err)
			}
			response, err := NewService(registry).Search(context.Background(), SearchRequest{
				WorkspaceID: "workspace-1",
				Query:       "panic",
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

func TestSentryIssueProviderPreservesCancellationAndCapsCandidates(t *testing.T) {
	issues := make([]sentry.MentionIssue, 12)
	for index := range issues {
		issues[index] = sentry.MentionIssue{
			InstanceID: "instance-a", InstanceOrigin: "https://sentry.example",
			ID: string(rune('a' + index)), ShortID: "APP-1", Title: "Issue",
			URL: "https://sentry.example/issues/APP-1/",
		}
	}
	service := &fakeSentryMentionService{issues: issues}
	provider := NewSentryIssueProvider(service)
	candidates, err := provider.Search(context.Background(), SearchRequest{
		WorkspaceID: "workspace-1", Query: "panic", Limit: -1,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if service.searchLimit != 1 || len(candidates) != 1 {
		t.Fatalf("service limit = %d, candidates = %d, want minimum cap 1", service.searchLimit, len(candidates))
	}

	service.searchErr = context.Canceled
	if _, err := provider.Search(context.Background(), SearchRequest{
		WorkspaceID: "workspace-1", Query: "panic", Limit: 5,
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v, want preserved", err)
	}
}

func TestSentryIssueProviderAuthorizesOwnedInstanceAndExactDestination(t *testing.T) {
	service := &fakeSentryMentionService{
		issues: []sentry.MentionIssue{{
			InstanceID: "instance-a", InstanceOrigin: "https://sentry.example", OrganizationSlug: "acme",
			ID: "42", ShortID: "APP-1", Title: "Panic",
			URL: "https://sentry.example/organizations/acme/issues/42/",
		}},
		lookupFn: func(workspaceID, instanceID string) (*sentry.MentionInstance, error) {
			if workspaceID == "workspace-2" {
				return &sentry.MentionInstance{ID: instanceID, WorkspaceID: workspaceID, Origin: "https://other.example"}, nil
			}
			return &sentry.MentionInstance{ID: instanceID, WorkspaceID: workspaceID, Origin: "https://sentry.example"}, nil
		},
	}
	provider := NewSentryIssueProvider(service)
	authorizer, ok := provider.(ReferenceAuthorizer)
	if !ok {
		t.Fatal("Sentry provider must authorize references")
	}
	registry := NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatalf("register: %v", err)
	}
	response, err := NewService(registry).Search(context.Background(), SearchRequest{
		WorkspaceID: "workspace-1", Query: "panic",
	})
	if err != nil || len(response.Groups) != 1 || len(response.Groups[0].Results) != 1 {
		t.Fatalf("search response = %#v, err = %v", response, err)
	}
	ref := response.Groups[0].Results[0]
	if err := authorizer.AuthorizeReference(context.Background(), ReferenceAuthorizationRequest{
		WorkspaceID: "workspace-1", Purpose: ReferencePurposeSubmission, Reference: ref,
	}); err != nil {
		t.Fatalf("authorize valid submission: %v", err)
	}
	if service.lookupWorkspaceID != "workspace-1" || service.lookupInstanceID != "instance-a" || service.lookupCalls != 1 {
		t.Fatalf("lookup = workspace %q instance %q calls %d",
			service.lookupWorkspaceID, service.lookupInstanceID, service.lookupCalls)
	}

	tests := []struct {
		name        string
		workspaceID string
		mutate      func(*apiv1.EntityReference)
	}{
		{name: "blank workspace", workspaceID: " "},
		{name: "different workspace origin", workspaceID: "workspace-2"},
		{name: "wrong scope", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.Scope = "instance-a|https://other.example"
			ref.Ref = canonicalRef(ref.Provider, ref.Kind, ref.Scope, ref.ID)
		}},
		{name: "wrong canonical ref", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.Ref = "mention:v1:sentry:issue:spoofed"
		}},
		{name: "wrong scheme", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.URL = "http://sentry.example/organizations/acme/issues/42/"
		}},
		{name: "lookalike host", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.URL = "https://sentry.example.evil.test/organizations/acme/issues/42/"
		}},
		{name: "userinfo", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.URL = "https://user@sentry.example/organizations/acme/issues/42/"
		}},
		{name: "port", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.URL = "https://sentry.example:443/organizations/acme/issues/42/"
		}},
		{name: "wrong path", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.URL = "https://sentry.example/projects/acme/issues/42/"
		}},
		{name: "wrong issue identity", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.URL = "https://sentry.example/organizations/acme/issues/99/"
		}},
		{name: "encoded path separator", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.URL = "https://sentry.example/organizations/acme%2Fevil/issues/42/"
		}},
		{name: "query", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.URL += "?project=1"
		}},
		{name: "fragment", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.URL += "#event"
		}},
		{name: "wrong provider", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.Provider = "linear"
		}},
		{name: "wrong kind", workspaceID: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
			ref.Kind = "event"
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

func TestSentryIssueProviderSubmissionFailsClosedWhenInstanceUnavailable(t *testing.T) {
	service := &fakeSentryMentionService{lookupFn: func(string, string) (*sentry.MentionInstance, error) {
		return nil, sentry.ErrInstanceNotFound
	}}
	provider := NewSentryIssueProvider(service)
	authorizer, ok := provider.(ReferenceAuthorizer)
	if !ok {
		t.Fatal("Sentry provider must authorize references")
	}
	ref := apiv1.EntityReference{
		Version:  apiv1.EntityReferenceVersion,
		Ref:      canonicalRef("sentry", "issue", "instance-a|https://sentry.example", "42"),
		Provider: "sentry", Kind: "issue", ID: "42", Key: "APP-1", Title: "Panic",
		URL: "https://sentry.example/issues/42/", Scope: "instance-a|https://sentry.example",
	}
	if err := authorizer.AuthorizeReference(context.Background(), ReferenceAuthorizationRequest{
		WorkspaceID: "workspace-1", Purpose: ReferencePurposeSubmission, Reference: ref,
	}); !errors.Is(err, ErrReferenceUnauthorized) {
		t.Fatalf("error = %v, want fail-closed unauthorized", err)
	}
}
