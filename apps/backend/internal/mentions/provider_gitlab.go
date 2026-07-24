package mentions

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/gitlab"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	gitLabProviderID        = "gitlab"
	gitLabIssueKind         = "issue"
	gitLabMergeRequestKind  = "merge_request"
	gitLabDisplayName       = "GitLab"
	gitLabIssueLabel        = "Issue"
	gitLabMergeRequestLabel = "Merge request"
	gitLabHTTPScheme        = "http"
	gitLabHTTPSScheme       = "https"
)

// GitLabMentionService is GitLab's explicit workspace/project mention boundary.
type GitLabMentionService interface {
	SearchMentionIssuesForWorkspace(
		ctx context.Context,
		workspaceID, query string,
		limit int,
	) ([]gitlab.MentionItem, error)
	SearchMentionMRsForWorkspace(
		ctx context.Context,
		workspaceID, query string,
		limit int,
	) ([]gitlab.MentionItem, error)
	MentionScopeForWorkspace(ctx context.Context, workspaceID string) (*gitlab.MentionScope, error)
}

type gitLabProvider struct {
	service GitLabMentionService
	kind    string
}

// NewGitLabIssueProvider creates GitLab's issue mention source.
func NewGitLabIssueProvider(service GitLabMentionService) MentionProvider {
	return &gitLabProvider{service: service, kind: gitLabIssueKind}
}

// NewGitLabMergeRequestProvider creates GitLab's merge-request mention source.
func NewGitLabMergeRequestProvider(service GitLabMentionService) MentionProvider {
	return &gitLabProvider{service: service, kind: gitLabMergeRequestKind}
}

func (p *gitLabProvider) Descriptor() ProviderDescriptor {
	if p.kind == gitLabMergeRequestKind {
		return ProviderDescriptor{
			Source: "gitlab_merge_requests", Provider: gitLabProviderID, Kind: p.kind,
			DisplayName: gitLabDisplayName, KindLabel: gitLabMergeRequestLabel, Order: 51,
		}
	}
	return ProviderDescriptor{
		Source: "gitlab_issues", Provider: gitLabProviderID, Kind: gitLabIssueKind,
		DisplayName: gitLabDisplayName, KindLabel: gitLabIssueLabel, Order: 50,
	}
}

func (p *gitLabProvider) Search(ctx context.Context, request SearchRequest) ([]Candidate, error) {
	if p == nil || p.service == nil {
		return nil, NewProviderError(StatusNotConfigured)
	}
	request.WorkspaceID = strings.TrimSpace(request.WorkspaceID)
	if request.WorkspaceID == "" {
		return nil, NewProviderError(StatusUnsupportedScope, gitlab.ErrMentionWorkspaceRequired)
	}
	request.Query = strings.TrimSpace(request.Query)
	if request.Query == "" {
		return nil, NewProviderError(StatusUpstreamError, gitlab.ErrMentionQueryRequired)
	}
	limit := clampGitLabMentionLimit(request.Limit)
	var (
		items []gitlab.MentionItem
		err   error
	)
	if p.kind == gitLabMergeRequestKind {
		items, err = p.service.SearchMentionMRsForWorkspace(ctx, request.WorkspaceID, request.Query, limit)
	} else {
		items, err = p.service.SearchMentionIssuesForWorkspace(ctx, request.WorkspaceID, request.Query, limit)
	}
	if err != nil {
		return nil, classifyGitLabMentionError(err)
	}
	count := min(len(items), limit)
	candidates := make([]Candidate, 0, count)
	for index := 0; index < count; index++ {
		item := items[index]
		candidates = append(candidates, Candidate{
			ID:    strconv.FormatInt(item.ID, 10),
			Key:   gitLabMentionKey(p.kind, item.ProjectPath, item.IID),
			Title: item.Title,
			URL:   item.URL,
			Scope: gitlab.MentionProjectScopeKey(item.Host, item.ProjectID),
		})
	}
	return candidates, nil
}

func clampGitLabMentionLimit(limit int) int {
	if limit <= 0 {
		return DefaultLimit
	}
	if limit > MaxLimit {
		return MaxLimit
	}
	return limit
}

func gitLabMentionKey(kind, projectPath string, iid int) string {
	separator := "#"
	if kind == gitLabMergeRequestKind {
		separator = "!"
	}
	return projectPath + separator + strconv.Itoa(iid)
}

func classifyGitLabMentionError(err error) error {
	status := StatusUpstreamError
	switch {
	case errors.Is(err, gitlab.ErrNoClient):
		status = StatusNotConfigured
	case errors.Is(err, gitlab.ErrMentionWorkspaceRequired),
		errors.Is(err, gitlab.ErrMentionUnsupportedScope),
		errors.Is(err, gitlab.ErrMentionInvalidScope):
		status = StatusUnsupportedScope
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded), isGitLabTimeoutError(err):
		status = StatusTimeout
	default:
		var apiErr *gitlab.APIError
		if errors.As(err, &apiErr) {
			status = gitLabAPIErrorStatus(apiErr.StatusCode)
		}
	}
	return NewProviderError(status, err)
}

func isGitLabTimeoutError(err error) bool {
	var timeoutErr net.Error
	return errors.As(err, &timeoutErr) && timeoutErr.Timeout()
}

func gitLabAPIErrorStatus(statusCode int) Status {
	switch {
	case statusCode >= http.StatusMultipleChoices && statusCode < http.StatusBadRequest:
		return StatusUnauthorized
	case statusCode == http.StatusUnauthorized, statusCode == http.StatusForbidden:
		return StatusUnauthorized
	case statusCode == http.StatusTooManyRequests:
		return StatusRateLimited
	case statusCode == http.StatusRequestTimeout, statusCode == http.StatusGatewayTimeout:
		return StatusTimeout
	default:
		return StatusUpstreamError
	}
}

func (p *gitLabProvider) AuthorizeReference(ctx context.Context, request ReferenceAuthorizationRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p == nil || p.service == nil {
		return ErrReferenceUnauthorized
	}
	workspaceID, ok := gitLabAuthorizationWorkspace(request)
	if !ok {
		return ErrReferenceUnauthorized
	}
	projectPath, iid, ok := validGitLabReference(request.Reference, p.kind)
	if !ok {
		return ErrReferenceUnauthorized
	}
	scope, err := p.service.MentionScopeForWorkspace(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return ErrReferenceUnauthorized
	}
	if !gitLabScopeAuthorizesReference(scope, workspaceID, projectPath, iid, p.kind, request.Reference) {
		return ErrReferenceUnauthorized
	}
	return nil
}

func gitLabAuthorizationWorkspace(request ReferenceAuthorizationRequest) (string, bool) {
	workspaceID := strings.TrimSpace(request.WorkspaceID)
	validPurpose := request.Purpose == ReferencePurposeSearch || request.Purpose == ReferencePurposeSubmission
	return workspaceID, workspaceID != "" && workspaceID == request.WorkspaceID && validPurpose
}

func validGitLabReference(reference apiv1.EntityReference, kind string) (string, int, bool) {
	projectPath, iid, validKey := parseGitLabMentionKey(kind, reference.Key)
	if !validKey || reference.Version != apiv1.EntityReferenceVersion ||
		reference.Provider != gitLabProviderID || reference.Kind != kind ||
		!validPositiveCanonicalInt(reference.ID) ||
		reference.Ref != canonicalRef(gitLabProviderID, kind, reference.Scope, reference.ID) {
		return "", 0, false
	}
	return projectPath, iid, true
}

func gitLabScopeAuthorizesReference(
	scope *gitlab.MentionScope,
	workspaceID, projectPath string,
	iid int,
	kind string,
	reference apiv1.EntityReference,
) bool {
	if scope == nil || scope.WorkspaceID != workspaceID || !validGitLabMentionHost(scope.Host) {
		return false
	}
	for _, project := range scope.Projects {
		if project.ID <= 0 || project.Path != projectPath {
			continue
		}
		if reference.Scope != gitlab.MentionProjectScopeKey(scope.Host, project.ID) {
			continue
		}
		expectedURL := gitlab.MentionIssueURL(scope.Host, project.Path, iid)
		if kind == gitLabMergeRequestKind {
			expectedURL = gitlab.MentionMRURL(scope.Host, project.Path, iid)
		}
		if reference.URL == expectedURL {
			return true
		}
	}
	return false
}

func parseGitLabMentionKey(kind, key string) (string, int, bool) {
	separator := "#"
	if kind == gitLabMergeRequestKind {
		separator = "!"
	}
	index := strings.LastIndex(key, separator)
	if index <= 0 || index == len(key)-1 {
		return "", 0, false
	}
	projectPath := key[:index]
	rawIID := key[index+1:]
	iid, err := strconv.Atoi(rawIID)
	if err != nil || iid <= 0 || strconv.Itoa(iid) != rawIID || strings.TrimSpace(projectPath) != projectPath {
		return "", 0, false
	}
	return projectPath, iid, true
}

func validPositiveCanonicalInt(raw string) bool {
	value, err := strconv.ParseInt(raw, 10, 64)
	return err == nil && value > 0 && strconv.FormatInt(value, 10) == raw
}

func validGitLabMentionHost(host string) bool {
	if host == "" || strings.TrimRight(host, "/") != host || strings.Contains(host, "|") {
		return false
	}
	parsed, err := url.Parse(host)
	if err != nil || parsed.User != nil || parsed.Opaque != "" || parsed.Host == "" ||
		(parsed.Scheme != gitLabHTTPScheme && parsed.Scheme != gitLabHTTPSScheme) || parsed.RawQuery != "" ||
		parsed.ForceQuery || parsed.Fragment != "" {
		return false
	}
	return parsed.Path == "" || path.Clean(parsed.Path) == parsed.Path
}
