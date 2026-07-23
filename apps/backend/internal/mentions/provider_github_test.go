package mentions

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/github"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

type fakeGitHubMentionService struct {
	issues      []*github.Issue
	prs         []*github.PR
	settings    *github.WorkspaceSettings
	err         error
	workspaceID string
	query       string
	limit       int
}

func (s *fakeGitHubMentionService) SearchMentionIssuesForWorkspace(
	_ context.Context,
	workspaceID, query string,
	limit int,
) ([]*github.Issue, error) {
	s.workspaceID, s.query, s.limit = workspaceID, query, limit
	return s.issues, s.err
}

func (s *fakeGitHubMentionService) SearchMentionPullRequestsForWorkspace(
	_ context.Context,
	workspaceID, query string,
	limit int,
) ([]*github.PR, error) {
	s.workspaceID, s.query, s.limit = workspaceID, query, limit
	return s.prs, s.err
}

func (s *fakeGitHubMentionService) GetWorkspaceSettings(
	_ context.Context,
	workspaceID string,
) (*github.WorkspaceSettings, error) {
	s.workspaceID = workspaceID
	if s.err != nil {
		return nil, s.err
	}
	return s.settings, nil
}

func TestGitHubProvidersMapIssueAndPullRequestIdentity(t *testing.T) {
	service := &fakeGitHubMentionService{
		issues: []*github.Issue{{ID: 101, NodeID: "I_kwDOA", Number: 17, Title: "Fix auth", RepoOwner: "Acme", RepoName: "Widgets"}},
		prs:    []*github.PR{{ID: 202, NodeID: "PR_kwDOB", Number: 18, Title: "Land auth fix", RepoOwner: "Acme", RepoName: "Widgets"}},
	}

	issueProvider := NewGitHubIssueProvider(service)
	prProvider := NewGitHubPullRequestProvider(service)
	issueDescriptor := issueProvider.Descriptor()
	prDescriptor := prProvider.Descriptor()
	if issueDescriptor.Source != "github_issues" || issueDescriptor.Kind != "issue" || issueDescriptor.Order != 40 {
		t.Fatalf("issue descriptor = %#v", issueDescriptor)
	}
	if prDescriptor.Source != "github_pull_requests" || prDescriptor.Kind != "pull_request" || prDescriptor.Order != 41 {
		t.Fatalf("PR descriptor = %#v", prDescriptor)
	}

	issues, err := issueProvider.Search(context.Background(), SearchRequest{WorkspaceID: "workspace-1", Query: "auth", Limit: 5})
	if err != nil {
		t.Fatalf("search issues: %v", err)
	}
	prs, err := prProvider.Search(context.Background(), SearchRequest{WorkspaceID: "workspace-1", Query: "auth", Limit: 5})
	if err != nil {
		t.Fatalf("search PRs: %v", err)
	}
	wantIssue := Candidate{
		ID: "I_kwDOA", Key: "acme/widgets#17", Title: "Fix auth",
		URL: "https://github.com/acme/widgets/issues/17", Scope: "github.com/acme/widgets",
	}
	wantPR := Candidate{
		ID: "PR_kwDOB", Key: "acme/widgets#18", Title: "Land auth fix",
		URL: "https://github.com/acme/widgets/pull/18", Scope: "github.com/acme/widgets",
	}
	if len(issues) != 1 || issues[0] != wantIssue {
		t.Fatalf("issue candidates = %#v, want %#v", issues, wantIssue)
	}
	if len(prs) != 1 || prs[0] != wantPR {
		t.Fatalf("PR candidates = %#v, want %#v", prs, wantPR)
	}
}

func TestGitHubProviderAuthorizesOnlyWorkspaceScopedCanonicalDestination(t *testing.T) {
	service := &fakeGitHubMentionService{settings: &github.WorkspaceSettings{
		WorkspaceID:    "workspace-1",
		RepoScopeMode:  github.RepoScopeModeRepos,
		RepoScopeRepos: []github.RepoFilter{{Owner: "acme", Name: "widgets"}},
	}}
	provider := NewGitHubIssueProvider(service)
	authorizer, ok := provider.(ReferenceAuthorizer)
	if !ok {
		t.Fatal("GitHub issue provider must authorize references")
	}
	reference := apiv1.EntityReference{
		Version:  apiv1.EntityReferenceVersion,
		Ref:      canonicalRef("github", "issue", "github.com/acme/widgets", "I_kwDOA"),
		Provider: "github", Kind: "issue", ID: "I_kwDOA", Key: "acme/widgets#17",
		Title: "Fix auth", URL: "https://github.com/acme/widgets/issues/17", Scope: "github.com/acme/widgets",
	}
	for _, purpose := range []ReferencePurpose{ReferencePurposeSearch, ReferencePurposeSubmission} {
		if err := authorizer.AuthorizeReference(context.Background(), ReferenceAuthorizationRequest{
			WorkspaceID: "workspace-1", Purpose: purpose, Reference: reference,
		}); err != nil {
			t.Fatalf("authorize %s: %v", purpose, err)
		}
	}

	tests := []struct {
		name   string
		mutate func(*apiv1.EntityReference)
	}{
		{"foreign repo", func(ref *apiv1.EntityReference) {
			ref.Scope = "github.com/foreign/private"
			ref.URL = "https://github.com/foreign/private/issues/17"
			ref.Key = "foreign/private#17"
			ref.Ref = canonicalRef("github", "issue", ref.Scope, ref.ID)
		}},
		{"foreign host", func(ref *apiv1.EntityReference) { ref.URL = "https://github.example.com/acme/widgets/issues/17" }},
		{"query", func(ref *apiv1.EntityReference) { ref.URL += "?redirect=1" }},
		{"fragment", func(ref *apiv1.EntityReference) { ref.URL += "#comment" }},
		{"wrong route", func(ref *apiv1.EntityReference) { ref.URL = "https://github.com/acme/widgets/pull/17" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invalid := reference
			test.mutate(&invalid)
			if err := authorizer.AuthorizeReference(context.Background(), ReferenceAuthorizationRequest{
				WorkspaceID: "workspace-1", Purpose: ReferencePurposeSubmission, Reference: invalid,
			}); err == nil {
				t.Fatal("invalid reference was authorized")
			}
		})
	}
}

func TestGitHubProviderClassifiesUnavailableClient(t *testing.T) {
	provider := NewGitHubIssueProvider(&fakeGitHubMentionService{err: github.ErrNoClient})
	_, err := provider.Search(context.Background(), SearchRequest{WorkspaceID: "workspace-1", Query: "auth", Limit: 5})
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Status != StatusNotConfigured {
		t.Fatalf("error = %v, want not configured", err)
	}
}
