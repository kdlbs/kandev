package mentions

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/github"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	githubProviderID      = "github"
	githubIssueKind       = mentionKindIssue
	githubPullRequestKind = "pull_request"
)

// GitHubMentionService is GitHub's workspace-explicit mention boundary.
type GitHubMentionService interface {
	SearchMentionIssuesForWorkspace(
		ctx context.Context,
		workspaceID, query string,
		limit int,
	) ([]*github.Issue, error)
	SearchMentionPullRequestsForWorkspace(
		ctx context.Context,
		workspaceID, query string,
		limit int,
	) ([]*github.PR, error)
	GetWorkspaceSettings(ctx context.Context, workspaceID string) (*github.WorkspaceSettings, error)
}

type githubProvider struct {
	service GitHubMentionService
	kind    string
}

// NewGitHubIssueProvider creates GitHub's issue mention source.
func NewGitHubIssueProvider(service GitHubMentionService) MentionProvider {
	return &githubProvider{service: service, kind: githubIssueKind}
}

// NewGitHubPullRequestProvider creates GitHub's pull-request mention source.
func NewGitHubPullRequestProvider(service GitHubMentionService) MentionProvider {
	return &githubProvider{service: service, kind: githubPullRequestKind}
}

func (p *githubProvider) Descriptor() ProviderDescriptor {
	if p.kind == githubPullRequestKind {
		return ProviderDescriptor{
			Source: "github_pull_requests", Provider: githubProviderID, Kind: githubPullRequestKind,
			DisplayName: "GitHub", KindLabel: "Pull request", Order: 41,
		}
	}
	return ProviderDescriptor{
		Source: "github_issues", Provider: githubProviderID, Kind: githubIssueKind,
		DisplayName: "GitHub", KindLabel: mentionLabelIssue, Order: 40,
	}
}

func (p *githubProvider) Search(ctx context.Context, request SearchRequest) ([]Candidate, error) {
	if p == nil || p.service == nil {
		return nil, NewProviderError(StatusNotConfigured)
	}
	if strings.TrimSpace(request.WorkspaceID) == "" {
		return nil, NewProviderError(StatusUnsupportedScope, github.ErrMentionWorkspaceRequired)
	}
	limit := clampGitHubMentionLimit(request.Limit)
	if p.kind == githubPullRequestKind {
		prs, err := p.service.SearchMentionPullRequestsForWorkspace(
			ctx, request.WorkspaceID, request.Query, limit,
		)
		if err != nil {
			return nil, classifyGitHubMentionError(err)
		}
		return githubPullRequestCandidates(prs, limit), nil
	}
	issues, err := p.service.SearchMentionIssuesForWorkspace(
		ctx, request.WorkspaceID, request.Query, limit,
	)
	if err != nil {
		return nil, classifyGitHubMentionError(err)
	}
	return githubIssueCandidates(issues, limit), nil
}

func githubIssueCandidates(issues []*github.Issue, limit int) []Candidate {
	candidates := make([]Candidate, 0, min(len(issues), limit))
	for _, issue := range issues {
		if issue == nil || issue.Number <= 0 {
			continue
		}
		candidate, ok := githubCandidate(
			issue.ID, issue.NodeID, issue.Number, issue.Title, issue.RepoOwner, issue.RepoName, githubIssueKind,
		)
		if ok {
			candidates = append(candidates, candidate)
		}
		if len(candidates) == limit {
			break
		}
	}
	return candidates
}

func githubPullRequestCandidates(prs []*github.PR, limit int) []Candidate {
	candidates := make([]Candidate, 0, min(len(prs), limit))
	for _, pr := range prs {
		if pr == nil || pr.Number <= 0 {
			continue
		}
		candidate, ok := githubCandidate(
			pr.ID, pr.NodeID, pr.Number, pr.Title, pr.RepoOwner, pr.RepoName, githubPullRequestKind,
		)
		if ok {
			candidates = append(candidates, candidate)
		}
		if len(candidates) == limit {
			break
		}
	}
	return candidates
}

func githubCandidate(
	restID int64,
	nodeID string,
	number int,
	title, owner, repo, kind string,
) (Candidate, bool) {
	owner, repo, ok := normalizeGitHubRepository(owner, repo)
	if !ok {
		return Candidate{}, false
	}
	id := strings.TrimSpace(nodeID)
	if id == "" && restID > 0 {
		id = strconv.FormatInt(restID, 10)
	}
	if id == "" || id != strings.TrimSpace(id) {
		return Candidate{}, false
	}
	route := mentionIssuesPathSegment
	if kind == githubPullRequestKind {
		route = "pull"
	}
	repository := owner + "/" + repo
	return Candidate{
		ID:    id,
		Key:   repository + "#" + strconv.Itoa(number),
		Title: title,
		URL:   "https://github.com/" + repository + "/" + route + "/" + strconv.Itoa(number),
		Scope: "github.com/" + repository,
	}, true
}

func clampGitHubMentionLimit(limit int) int {
	if limit <= 0 {
		return DefaultLimit
	}
	if limit > MaxLimit {
		return MaxLimit
	}
	return limit
}

func classifyGitHubMentionError(err error) error {
	status := StatusUpstreamError
	switch {
	case errors.Is(err, github.ErrNoClient):
		status = StatusNotConfigured
	case errors.Is(err, github.ErrMentionWorkspaceRequired):
		status = StatusUnsupportedScope
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		status = StatusTimeout
	default:
		var apiErr *github.GitHubAPIError
		var timeoutErr net.Error
		switch {
		case errors.As(err, &apiErr):
			status = githubAPIStatus(apiErr.StatusCode)
		case errors.As(err, &timeoutErr) && timeoutErr.Timeout():
			status = StatusTimeout
		case strings.Contains(strings.ToLower(err.Error()), "rate limit"):
			status = StatusRateLimited
		}
	}
	return NewProviderError(status, err)
}

func githubAPIStatus(statusCode int) Status {
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return StatusUnauthorized
	case http.StatusTooManyRequests:
		return StatusRateLimited
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return StatusTimeout
	default:
		return StatusUpstreamError
	}
}

func (p *githubProvider) AuthorizeReference(ctx context.Context, request ReferenceAuthorizationRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p == nil || p.service == nil || request.WorkspaceID == "" ||
		(request.Purpose != ReferencePurposeSearch && request.Purpose != ReferencePurposeSubmission) ||
		!validGitHubReference(request.Reference, p.kind) {
		return ErrReferenceUnauthorized
	}
	settings, err := p.service.GetWorkspaceSettings(ctx, request.WorkspaceID)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return ErrReferenceUnauthorized
	}
	owner, repo, ok := repositoryFromGitHubScope(request.Reference.Scope)
	if !ok || !githubSettingsAllowRepository(settings, request.WorkspaceID, owner, repo) {
		return ErrReferenceUnauthorized
	}
	return nil
}

func validGitHubReference(reference apiv1.EntityReference, kind string) bool {
	if reference.Version != apiv1.EntityReferenceVersion || reference.Provider != githubProviderID ||
		reference.Kind != kind || reference.ID == "" || reference.ID != strings.TrimSpace(reference.ID) ||
		reference.Ref != canonicalRef(githubProviderID, kind, reference.Scope, reference.ID) {
		return false
	}
	owner, repo, ok := repositoryFromGitHubScope(reference.Scope)
	if !ok {
		return false
	}
	repository := owner + "/" + repo
	keyPrefix := repository + "#"
	if !strings.HasPrefix(reference.Key, keyPrefix) {
		return false
	}
	numberText := strings.TrimPrefix(reference.Key, keyPrefix)
	number, err := strconv.Atoi(numberText)
	if err != nil || number <= 0 || strconv.Itoa(number) != numberText {
		return false
	}
	route := mentionIssuesPathSegment
	if kind == githubPullRequestKind {
		route = "pull"
	}
	wantURL := fmt.Sprintf("https://github.com/%s/%s/%d", repository, route, number)
	return reference.URL == wantURL
}

func repositoryFromGitHubScope(scope string) (string, string, bool) {
	const prefix = "github.com/"
	if !strings.HasPrefix(scope, prefix) || strings.ToLower(scope) != scope {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(scope, prefix), "/")
	if len(parts) != 2 {
		return "", "", false
	}
	owner, repo, ok := normalizeGitHubRepository(parts[0], parts[1])
	if !ok || scope != prefix+owner+"/"+repo {
		return "", "", false
	}
	return owner, repo, true
}

func normalizeGitHubRepository(owner, repo string) (string, string, bool) {
	owner = strings.ToLower(strings.TrimSpace(owner))
	repo = strings.ToLower(strings.TrimSpace(repo))
	if !validGitHubPathSegment(owner) || !validGitHubPathSegment(repo) {
		return "", "", false
	}
	return owner, repo, true
}

func validGitHubPathSegment(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') ||
			char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func githubSettingsAllowRepository(
	settings *github.WorkspaceSettings,
	workspaceID, owner, repo string,
) bool {
	if settings == nil || strings.TrimSpace(settings.WorkspaceID) != workspaceID {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(settings.RepoScopeMode)) {
	case github.RepoScopeModeAll:
		return true
	case github.RepoScopeModeOrgs:
		for _, org := range settings.RepoScopeOrgs {
			if strings.EqualFold(strings.TrimSpace(org), owner) {
				return true
			}
		}
	case github.RepoScopeModeRepos:
		for _, candidate := range settings.RepoScopeRepos {
			if strings.EqualFold(strings.TrimSpace(candidate.Owner), owner) &&
				strings.EqualFold(strings.TrimSpace(candidate.Name), repo) {
				return true
			}
		}
	}
	return false
}
