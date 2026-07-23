package mentions

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/azuredevops"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	azureProviderID      = "azure_devops"
	azureWorkItemKind    = "work_item"
	azurePullRequestKind = "pull_request"
)

// AzureMentionService is Azure DevOps' workspace-explicit mention boundary.
type AzureMentionService interface {
	SearchMentionWorkItemsForWorkspace(
		context.Context, string, string, int,
	) ([]azuredevops.MentionWorkItem, error)
	SearchMentionPullRequestsForWorkspace(
		context.Context, string, string, int,
	) ([]azuredevops.MentionPullRequest, error)
	ResolveMentionProjectForWorkspace(
		context.Context, string, string,
	) (*azuredevops.MentionProject, error)
	ResolveMentionRepositoryForWorkspace(
		context.Context, string, string, string,
	) (*azuredevops.MentionRepository, error)
}

type azureProvider struct {
	service AzureMentionService
	kind    string
}

// NewAzureWorkItemProvider creates Azure Boards' work-item mention source.
func NewAzureWorkItemProvider(service AzureMentionService) MentionProvider {
	return &azureProvider{service: service, kind: azureWorkItemKind}
}

// NewAzurePullRequestProvider creates Azure Repos' pull-request mention source.
func NewAzurePullRequestProvider(service AzureMentionService) MentionProvider {
	return &azureProvider{service: service, kind: azurePullRequestKind}
}

func (p *azureProvider) Descriptor() ProviderDescriptor {
	if p.kind == azurePullRequestKind {
		return ProviderDescriptor{
			Source: "azure_pull_requests", Provider: azureProviderID, Kind: azurePullRequestKind,
			DisplayName: "Azure DevOps", KindLabel: "Pull request", Order: 61,
		}
	}
	return ProviderDescriptor{
		Source: "azure_work_items", Provider: azureProviderID, Kind: azureWorkItemKind,
		DisplayName: "Azure DevOps", KindLabel: mentionLabelWorkItem, Order: 60,
	}
}

func (p *azureProvider) Search(ctx context.Context, request SearchRequest) ([]Candidate, error) {
	if p == nil || p.service == nil {
		return nil, NewProviderError(StatusNotConfigured)
	}
	if strings.TrimSpace(request.WorkspaceID) == "" {
		return nil, NewProviderError(StatusUnsupportedScope, azuredevops.ErrInvalidWorkspaceID)
	}
	limit := clampAzureMentionLimit(request.Limit)
	if p.kind == azurePullRequestKind {
		items, err := p.service.SearchMentionPullRequestsForWorkspace(
			ctx, request.WorkspaceID, request.Query, limit,
		)
		if err != nil {
			return nil, classifyAzureMentionError(err)
		}
		return azurePullRequestCandidates(items, limit), nil
	}
	items, err := p.service.SearchMentionWorkItemsForWorkspace(
		ctx, request.WorkspaceID, request.Query, limit,
	)
	if err != nil {
		return nil, classifyAzureMentionError(err)
	}
	return azureWorkItemCandidates(items, limit), nil
}

func azureWorkItemCandidates(items []azuredevops.MentionWorkItem, limit int) []Candidate {
	candidates := make([]Candidate, 0, min(len(items), limit))
	for _, item := range items {
		if item.ID <= 0 || strings.TrimSpace(item.Title) == "" ||
			strings.TrimSpace(item.ProjectName) == "" {
			continue
		}
		scope, ok := azureScope(item.OrganizationURL, item.ProjectID)
		if !ok {
			continue
		}
		id := strconv.Itoa(item.ID)
		candidates = append(candidates, Candidate{
			ID: id, Key: item.ProjectName + "#" + id, Title: item.Title,
			URL: strings.TrimRight(item.OrganizationURL, "/") + "/" +
				url.PathEscape(item.ProjectName) + "/_workitems/edit/" + id,
			Scope: scope,
		})
		if len(candidates) == limit {
			break
		}
	}
	return candidates
}

func azurePullRequestCandidates(items []azuredevops.MentionPullRequest, limit int) []Candidate {
	candidates := make([]Candidate, 0, min(len(items), limit))
	for _, item := range items {
		if item.ID <= 0 || strings.TrimSpace(item.Title) == "" ||
			strings.TrimSpace(item.ProjectName) == "" || strings.TrimSpace(item.RepositoryName) == "" {
			continue
		}
		scope, ok := azureScope(item.OrganizationURL, item.ProjectID, item.RepositoryID)
		if !ok {
			continue
		}
		id := strconv.Itoa(item.ID)
		candidates = append(candidates, Candidate{
			ID: id, Key: item.ProjectName + "/" + item.RepositoryName + "!" + id, Title: item.Title,
			URL: strings.TrimRight(item.OrganizationURL, "/") + "/" +
				url.PathEscape(item.ProjectName) + "/_git/" + url.PathEscape(item.RepositoryName) +
				"/pullrequest/" + id,
			Scope: scope,
		})
		if len(candidates) == limit {
			break
		}
	}
	return candidates
}

func clampAzureMentionLimit(limit int) int {
	if limit <= 0 {
		return DefaultLimit
	}
	if limit > MaxLimit {
		return MaxLimit
	}
	return limit
}

func classifyAzureMentionError(err error) error {
	status := StatusUpstreamError
	switch {
	case errors.Is(err, azuredevops.ErrNotConfigured):
		status = StatusNotConfigured
	case errors.Is(err, azuredevops.ErrInvalidWorkspaceID), errors.Is(err, azuredevops.ErrInvalidConfig):
		status = StatusUnsupportedScope
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		status = StatusTimeout
	default:
		var apiErr *azuredevops.APIError
		var timeoutErr net.Error
		switch {
		case errors.As(err, &apiErr):
			status = azureAPIStatus(apiErr.StatusCode)
		case errors.As(err, &timeoutErr) && timeoutErr.Timeout():
			status = StatusTimeout
		}
	}
	return NewProviderError(status, err)
}

func azureAPIStatus(statusCode int) Status {
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

func (p *azureProvider) AuthorizeReference(ctx context.Context, request ReferenceAuthorizationRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p == nil || p.service == nil || strings.TrimSpace(request.WorkspaceID) == "" ||
		(request.Purpose != ReferencePurposeSearch && request.Purpose != ReferencePurposeSubmission) {
		return ErrReferenceUnauthorized
	}
	parts, ok := validAzureReference(request.Reference, p.kind)
	if !ok {
		return ErrReferenceUnauthorized
	}
	if request.Purpose == ReferencePurposeSearch {
		return nil
	}
	if p.kind == azurePullRequestKind {
		return p.authorizePullRequest(ctx, request.WorkspaceID, request.Reference, parts)
	}
	return p.authorizeWorkItem(ctx, request.WorkspaceID, request.Reference, parts)
}

type azureScopeParts struct {
	organization string
	projectID    string
	repositoryID string
}

func validAzureReference(reference apiv1.EntityReference, kind string) (azureScopeParts, bool) {
	if !validAzureReferenceIdentity(reference, kind) {
		return azureScopeParts{}, false
	}
	return parseAzureReferenceScope(reference.Scope, kind)
}

func validAzureReferenceIdentity(reference apiv1.EntityReference, kind string) bool {
	if reference.Version != apiv1.EntityReferenceVersion || reference.Provider != azureProviderID ||
		reference.Kind != kind || reference.ID == "" || reference.ID != strings.TrimSpace(reference.ID) ||
		reference.Ref != canonicalRef(azureProviderID, kind, reference.Scope, reference.ID) {
		return false
	}
	id, err := strconv.Atoi(reference.ID)
	return err == nil && id > 0 && strconv.Itoa(id) == reference.ID
}

func parseAzureReferenceScope(scope, kind string) (azureScopeParts, bool) {
	wantParts := 3
	if kind == azurePullRequestKind {
		wantParts = 4
	}
	parts := strings.Split(scope, "/")
	if len(parts) != wantParts || parts[0] != "dev.azure.com" || strings.ToLower(scope) != scope {
		return azureScopeParts{}, false
	}
	for _, part := range parts[1:] {
		if !validAzureScopeSegment(part) {
			return azureScopeParts{}, false
		}
	}
	result := azureScopeParts{organization: parts[1], projectID: parts[2]}
	if len(parts) == 4 {
		result.repositoryID = parts[3]
	}
	return result, true
}

func (p *azureProvider) authorizeWorkItem(
	ctx context.Context,
	workspaceID string,
	reference apiv1.EntityReference,
	parts azureScopeParts,
) error {
	project, err := p.service.ResolveMentionProjectForWorkspace(ctx, workspaceID, parts.projectID)
	if err != nil || project == nil || !azureResolvedScopeMatches(
		project.OrganizationURL, project.ProjectID, "", parts,
	) {
		return ErrReferenceUnauthorized
	}
	wantKey := project.ProjectName + "#" + reference.ID
	wantURL := strings.TrimRight(project.OrganizationURL, "/") + "/" +
		url.PathEscape(project.ProjectName) + "/_workitems/edit/" + reference.ID
	if reference.Key != wantKey || reference.URL != wantURL {
		return ErrReferenceUnauthorized
	}
	return nil
}

func (p *azureProvider) authorizePullRequest(
	ctx context.Context,
	workspaceID string,
	reference apiv1.EntityReference,
	parts azureScopeParts,
) error {
	repository, err := p.service.ResolveMentionRepositoryForWorkspace(
		ctx, workspaceID, parts.projectID, parts.repositoryID,
	)
	if err != nil || repository == nil || !azureResolvedScopeMatches(
		repository.OrganizationURL, repository.ProjectID, repository.RepositoryID, parts,
	) {
		return ErrReferenceUnauthorized
	}
	wantKey := repository.ProjectName + "/" + repository.RepositoryName + "!" + reference.ID
	wantURL := strings.TrimRight(repository.OrganizationURL, "/") + "/" +
		url.PathEscape(repository.ProjectName) + "/_git/" + url.PathEscape(repository.RepositoryName) +
		"/pullrequest/" + reference.ID
	if reference.Key != wantKey || reference.URL != wantURL {
		return ErrReferenceUnauthorized
	}
	return nil
}

func azureResolvedScopeMatches(
	organizationURL, projectID, repositoryID string,
	parts azureScopeParts,
) bool {
	scope, ok := azureScope(organizationURL, projectID, repositoryID)
	if !ok {
		return false
	}
	want := "dev.azure.com/" + parts.organization + "/" + parts.projectID
	if parts.repositoryID != "" {
		want += "/" + parts.repositoryID
	}
	return scope == want
}

func azureScope(organizationURL, projectID string, repositoryIDs ...string) (string, bool) {
	canonicalURL, err := azuredevops.ValidateOrganizationURL(organizationURL)
	if err != nil || canonicalURL != strings.TrimRight(organizationURL, "/") {
		return "", false
	}
	organization := strings.TrimPrefix(strings.ToLower(canonicalURL), "https://dev.azure.com/")
	projectID = strings.ToLower(strings.TrimSpace(projectID))
	if !validAzureScopeSegment(organization) || !validAzureScopeSegment(projectID) {
		return "", false
	}
	scope := "dev.azure.com/" + organization + "/" + projectID
	if len(repositoryIDs) > 0 && strings.TrimSpace(repositoryIDs[0]) != "" {
		repositoryID := strings.ToLower(strings.TrimSpace(repositoryIDs[0]))
		if !validAzureScopeSegment(repositoryID) {
			return "", false
		}
		scope += "/" + repositoryID
	}
	return scope, true
}

func validAzureScopeSegment(value string) bool {
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
