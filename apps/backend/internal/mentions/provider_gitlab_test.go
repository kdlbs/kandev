package mentions

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/gitlab"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

type fakeGitLabMentionService struct {
	issues func(context.Context, string, string, int) ([]gitlab.MentionItem, error)
	mrs    func(context.Context, string, string, int) ([]gitlab.MentionItem, error)
	scope  func(context.Context, string) (*gitlab.MentionScope, error)
}

func (s *fakeGitLabMentionService) SearchMentionIssuesForWorkspace(
	ctx context.Context,
	workspaceID, query string,
	limit int,
) ([]gitlab.MentionItem, error) {
	return s.issues(ctx, workspaceID, query, limit)
}

func (s *fakeGitLabMentionService) SearchMentionMRsForWorkspace(
	ctx context.Context,
	workspaceID, query string,
	limit int,
) ([]gitlab.MentionItem, error) {
	return s.mrs(ctx, workspaceID, query, limit)
}

func (s *fakeGitLabMentionService) MentionScopeForWorkspace(
	ctx context.Context,
	workspaceID string,
) (*gitlab.MentionScope, error) {
	return s.scope(ctx, workspaceID)
}

func gitLabTestService() *fakeGitLabMentionService {
	const host = "https://gitlab.example.test/base"
	return &fakeGitLabMentionService{
		issues: func(_ context.Context, workspaceID, query string, limit int) ([]gitlab.MentionItem, error) {
			if workspaceID != "workspace-1" || query != "auth" || limit != 4 {
				return nil, errors.New("unexpected issue search request")
			}
			return []gitlab.MentionItem{{
				ID: 1001, IID: 42, ProjectID: 101, ProjectPath: "group/api",
				Title: "Fix auth", URL: host + "/group/api/-/issues/42", Host: host,
			}}, nil
		},
		mrs: func(_ context.Context, workspaceID, query string, limit int) ([]gitlab.MentionItem, error) {
			if workspaceID != "workspace-1" || query != "auth" || limit != 4 {
				return nil, errors.New("unexpected MR search request")
			}
			return []gitlab.MentionItem{{
				ID: 2001, IID: 7, ProjectID: 202, ProjectPath: "group/web",
				Title: "Auth MR", URL: host + "/group/web/-/merge_requests/7", Host: host,
			}}, nil
		},
		scope: func(_ context.Context, workspaceID string) (*gitlab.MentionScope, error) {
			if workspaceID != "workspace-1" {
				return nil, gitlab.ErrMentionUnsupportedScope
			}
			return &gitlab.MentionScope{
				WorkspaceID: workspaceID,
				Host:        host,
				Projects: []gitlab.MentionProjectScope{
					{ID: 101, Path: "group/api"},
					{ID: 202, Path: "group/web"},
				},
			}, nil
		},
	}
}

func TestGitLabProvidersMapImmutableIssueAndMRIdentity(t *testing.T) {
	service := gitLabTestService()
	tests := []struct {
		name       string
		provider   MentionProvider
		wantSource string
		wantKind   string
		wantOrder  int
		wantID     string
		wantKey    string
		wantTitle  string
		wantURL    string
		wantScope  string
	}{
		{
			name: "issue", provider: NewGitLabIssueProvider(service),
			wantSource: "gitlab_issues", wantKind: "issue", wantOrder: 50,
			wantID: "1001", wantKey: "group/api#42", wantTitle: "Fix auth",
			wantURL:   "https://gitlab.example.test/base/group/api/-/issues/42",
			wantScope: gitlab.MentionProjectScopeKey("https://gitlab.example.test/base", 101),
		},
		{
			name: "merge request", provider: NewGitLabMergeRequestProvider(service),
			wantSource: "gitlab_merge_requests", wantKind: "merge_request", wantOrder: 51,
			wantID: "2001", wantKey: "group/web!7", wantTitle: "Auth MR",
			wantURL:   "https://gitlab.example.test/base/group/web/-/merge_requests/7",
			wantScope: gitlab.MentionProjectScopeKey("https://gitlab.example.test/base", 202),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptor := test.provider.Descriptor()
			if descriptor.Source != test.wantSource || descriptor.Provider != "gitlab" ||
				descriptor.Kind != test.wantKind || descriptor.Order != test.wantOrder ||
				descriptor.DisplayName != "GitLab" {
				t.Fatalf("descriptor = %#v", descriptor)
			}
			registry := NewRegistry()
			if err := registry.Register(test.provider); err != nil {
				t.Fatalf("register: %v", err)
			}
			response, err := NewService(registry).Search(context.Background(), SearchRequest{
				WorkspaceID: "workspace-1", Query: "auth", Limit: 4,
			})
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if len(response.Groups) != 1 || len(response.Groups[0].Results) != 1 {
				t.Fatalf("groups = %#v", response.Groups)
			}
			result := response.Groups[0].Results[0]
			if result.ID != test.wantID || result.Key != test.wantKey || result.Title != test.wantTitle ||
				result.URL != test.wantURL || result.Scope != test.wantScope ||
				result.Ref != canonicalRef("gitlab", test.wantKind, test.wantScope, test.wantID) {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestGitLabProviderSearchRejectsUnboundProjectHit(t *testing.T) {
	const host = "https://gitlab.example.test"
	service := &fakeGitLabMentionService{
		issues: func(context.Context, string, string, int) ([]gitlab.MentionItem, error) {
			return []gitlab.MentionItem{{
				ID: 9001, IID: 9, ProjectID: 999, ProjectPath: "secret/project",
				Title: "Leak", URL: host + "/secret/project/-/issues/9", Host: host,
			}}, nil
		},
		mrs: func(context.Context, string, string, int) ([]gitlab.MentionItem, error) { return nil, nil },
		scope: func(context.Context, string) (*gitlab.MentionScope, error) {
			return &gitlab.MentionScope{
				WorkspaceID: "workspace-1", Host: host,
				Projects: []gitlab.MentionProjectScope{{ID: 101, Path: "group/api"}},
			}, nil
		},
	}
	registry := NewRegistry()
	if err := registry.Register(NewGitLabIssueProvider(service)); err != nil {
		t.Fatalf("register: %v", err)
	}
	response, err := NewService(registry).Search(context.Background(), SearchRequest{
		WorkspaceID: "workspace-1", Query: "auth",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if got := len(response.Groups[0].Results); got != 0 {
		t.Fatalf("unbound project results = %d, want 0", got)
	}
}

func TestGitLabProviderMapsSafePartialStatuses(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want Status
	}{
		{name: "not configured", err: gitlab.ErrNoClient, want: StatusNotConfigured},
		{name: "unsupported workspace", err: gitlab.ErrMentionUnsupportedScope, want: StatusUnsupportedScope},
		{name: "unauthorized", err: &gitlab.APIError{StatusCode: http.StatusUnauthorized, Body: "secret"}, want: StatusUnauthorized},
		{name: "login redirect", err: &gitlab.APIError{StatusCode: http.StatusFound, Body: "secret"}, want: StatusUnauthorized},
		{name: "rate limited", err: &gitlab.APIError{StatusCode: http.StatusTooManyRequests, Body: "secret"}, want: StatusRateLimited},
		{name: "timeout", err: &gitlab.APIError{StatusCode: http.StatusGatewayTimeout, Body: "secret"}, want: StatusTimeout},
		{name: "context timeout", err: context.DeadlineExceeded, want: StatusTimeout},
		{name: "upstream", err: errors.New("secret upstream response"), want: StatusUpstreamError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &fakeGitLabMentionService{
				issues: func(context.Context, string, string, int) ([]gitlab.MentionItem, error) { return nil, test.err },
				mrs:    func(context.Context, string, string, int) ([]gitlab.MentionItem, error) { return nil, test.err },
				scope:  func(context.Context, string) (*gitlab.MentionScope, error) { return nil, test.err },
			}
			registry := NewRegistry()
			if err := registry.Register(NewGitLabIssueProvider(service)); err != nil {
				t.Fatalf("register: %v", err)
			}
			response, err := NewService(registry).Search(context.Background(), SearchRequest{
				WorkspaceID: "workspace-1", Query: "auth",
			})
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if response.Groups[0].Status != test.want {
				t.Fatalf("status = %q, want %q", response.Groups[0].Status, test.want)
			}
		})
	}
}

func TestGitLabProvidersWithoutServiceAreNotConfigured(t *testing.T) {
	for _, provider := range []MentionProvider{NewGitLabIssueProvider(nil), NewGitLabMergeRequestProvider(nil)} {
		registry := NewRegistry()
		if err := registry.Register(provider); err != nil {
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
}

func TestGitLabProviderAuthorizesExactWorkspaceHostAndProjectForBothPurposes(t *testing.T) {
	const host = "https://gitlab.example.test/base"
	scopeCalls := 0
	service := &fakeGitLabMentionService{
		issues: func(context.Context, string, string, int) ([]gitlab.MentionItem, error) { return nil, nil },
		mrs:    func(context.Context, string, string, int) ([]gitlab.MentionItem, error) { return nil, nil },
		scope: func(_ context.Context, workspaceID string) (*gitlab.MentionScope, error) {
			scopeCalls++
			if workspaceID == "" {
				t.Fatal("blank workspace fell back to global GitLab config")
			}
			if workspaceID != "workspace-1" {
				return nil, gitlab.ErrMentionUnsupportedScope
			}
			return &gitlab.MentionScope{
				WorkspaceID: workspaceID, Host: host,
				Projects: []gitlab.MentionProjectScope{
					{ID: 101, Path: "group/api"},
					{ID: 202, Path: "group/web"},
				},
			}, nil
		},
	}
	tests := []struct {
		name      string
		provider  MentionProvider
		reference apiv1.EntityReference
	}{
		{
			name: "issue", provider: NewGitLabIssueProvider(service),
			reference: gitLabReference("issue", "1001", "group/api#42", host, 101,
				host+"/group/api/-/issues/42"),
		},
		{
			name: "merge request", provider: NewGitLabMergeRequestProvider(service),
			reference: gitLabReference("merge_request", "2001", "group/web!7", host, 202,
				host+"/group/web/-/merge_requests/7"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authorizer, ok := test.provider.(ReferenceAuthorizer)
			if !ok {
				t.Fatal("GitLab provider must implement ReferenceAuthorizer")
			}
			for _, purpose := range []ReferencePurpose{ReferencePurposeSearch, ReferencePurposeSubmission} {
				if err := authorizer.AuthorizeReference(context.Background(), ReferenceAuthorizationRequest{
					WorkspaceID: "workspace-1", Purpose: purpose, Reference: test.reference,
				}); err != nil {
					t.Fatalf("authorize %s: %v", purpose, err)
				}
			}

			badCases := []struct {
				name      string
				workspace string
				mutate    func(*apiv1.EntityReference)
			}{
				{name: "blank workspace", workspace: ""},
				{name: "other workspace", workspace: "workspace-2"},
				{name: "wrong host scope", workspace: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
					ref.Scope = gitlab.MentionProjectScopeKey("https://evil.example.test", 101)
				}},
				{name: "wrong project scope", workspace: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
					ref.Scope = gitlab.MentionProjectScopeKey(host, 999)
				}},
				{name: "wrong URL", workspace: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
					ref.URL = "https://evil.example.test/secret"
				}},
				{name: "URL query", workspace: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
					ref.URL += "?token=secret"
				}},
				{name: "wrong key project", workspace: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
					if ref.Kind == "issue" {
						ref.Key = "other/api#42"
					} else {
						ref.Key = "other/web!7"
					}
				}},
				{name: "wrong canonical ref", workspace: "workspace-1", mutate: func(ref *apiv1.EntityReference) {
					ref.Ref = "mention:v1:gitlab:spoofed"
				}},
			}
			for _, bad := range badCases {
				t.Run(bad.name, func(t *testing.T) {
					ref := test.reference
					if bad.mutate != nil {
						bad.mutate(&ref)
					}
					err := authorizer.AuthorizeReference(context.Background(), ReferenceAuthorizationRequest{
						WorkspaceID: bad.workspace, Purpose: ReferencePurposeSubmission, Reference: ref,
					})
					if !errors.Is(err, ErrReferenceUnauthorized) {
						t.Fatalf("error = %v, want ErrReferenceUnauthorized", err)
					}
				})
			}
		})
	}
	if scopeCalls == 0 {
		t.Fatal("workspace GitLab scope was never rechecked")
	}
}

func gitLabReference(kind, id, key, host string, projectID int64, destination string) apiv1.EntityReference {
	scope := gitlab.MentionProjectScopeKey(host, projectID)
	return apiv1.EntityReference{
		Version:  apiv1.EntityReferenceVersion,
		Ref:      canonicalRef("gitlab", kind, scope, id),
		Provider: "gitlab", Kind: kind, ID: id, Key: key, Title: "Title",
		URL: destination, Scope: scope,
	}
}
