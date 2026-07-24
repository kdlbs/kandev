package mentions

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/kandev/kandev/internal/jira"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

const jiraProviderID = "jira"

// JiraMentionService is Jira's workspace-explicit mention boundary.
type JiraMentionService interface {
	SearchMentionTicketsForWorkspace(
		ctx context.Context,
		workspaceID, query string,
		limit int,
	) ([]jira.MentionTicket, error)
	MentionSiteURLForWorkspace(ctx context.Context, workspaceID string) (string, error)
}

type jiraProvider struct {
	service JiraMentionService
}

// NewJiraProvider creates Jira's built-in issue mention source.
func NewJiraProvider(service JiraMentionService) MentionProvider {
	return &jiraProvider{service: service}
}

func (p *jiraProvider) Descriptor() ProviderDescriptor {
	return ProviderDescriptor{
		Source:      "jira_issues",
		Provider:    jiraProviderID,
		Kind:        mentionKindIssue,
		DisplayName: "Jira",
		KindLabel:   mentionLabelIssue,
		Order:       20,
	}
}

func (p *jiraProvider) Search(ctx context.Context, request SearchRequest) ([]Candidate, error) {
	if p.service == nil {
		return nil, NewProviderError(StatusNotConfigured)
	}
	if strings.TrimSpace(request.WorkspaceID) == "" {
		return nil, NewProviderError(StatusUnsupportedScope, jira.ErrMentionWorkspaceRequired)
	}
	tickets, err := p.service.SearchMentionTicketsForWorkspace(
		ctx,
		request.WorkspaceID,
		request.Query,
		request.Limit,
	)
	if err != nil {
		return nil, mapJiraProviderError(err)
	}
	limit := normalizedJiraProviderLimit(request.Limit)
	count := min(len(tickets), limit)
	candidates := make([]Candidate, 0, count)
	for i := 0; i < count; i++ {
		ticket := tickets[i]
		candidates = append(candidates, Candidate{
			ID:    ticket.ID,
			Key:   ticket.Key,
			Title: ticket.Title,
			URL:   ticket.URL,
			Scope: ticket.SiteURL,
		})
	}
	return candidates, nil
}

func normalizedJiraProviderLimit(limit int) int {
	switch {
	case limit == 0:
		return DefaultLimit
	case limit < 1:
		return 1
	case limit > MaxLimit:
		return MaxLimit
	default:
		return limit
	}
}

func mapJiraProviderError(err error) error {
	status := StatusUpstreamError
	switch {
	case errors.Is(err, jira.ErrNotConfigured):
		status = StatusNotConfigured
	case errors.Is(err, jira.ErrMentionWorkspaceRequired), errors.Is(err, jira.ErrMentionInvalidSiteURL):
		status = StatusUnsupportedScope
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled), isJiraTimeoutError(err):
		status = StatusTimeout
	default:
		var apiErr *jira.APIError
		if errors.As(err, &apiErr) {
			status = jiraAPIErrorStatus(apiErr.StatusCode)
		}
	}
	return NewProviderError(status, err)
}

func isJiraTimeoutError(err error) bool {
	var timeoutErr net.Error
	return errors.As(err, &timeoutErr) && timeoutErr.Timeout()
}

func jiraAPIErrorStatus(statusCode int) Status {
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

func (p *jiraProvider) AuthorizeReference(ctx context.Context, request ReferenceAuthorizationRequest) error {
	reference := request.Reference
	workspaceID := strings.TrimSpace(request.WorkspaceID)
	if p.service == nil || workspaceID == "" || workspaceID != request.WorkspaceID ||
		(request.Purpose != ReferencePurposeSearch && request.Purpose != ReferencePurposeSubmission) ||
		!validJiraReferenceShape(reference) {
		return ErrReferenceUnauthorized
	}
	siteURL, err := p.service.MentionSiteURLForWorkspace(ctx, workspaceID)
	if err != nil || !jiraReferenceMatchesSite(reference, siteURL) {
		return ErrReferenceUnauthorized
	}
	return nil
}

func validJiraReferenceShape(reference apiv1.EntityReference) bool {
	return reference.Version == apiv1.EntityReferenceVersion && reference.Provider == jiraProviderID &&
		reference.Kind == mentionKindIssue && reference.ID != "" &&
		strings.TrimSpace(reference.ID) == reference.ID && reference.Key != "" &&
		strings.TrimSpace(reference.Key) == reference.Key
}

func jiraReferenceMatchesSite(reference apiv1.EntityReference, siteURL string) bool {
	return validConfiguredJiraSiteURL(siteURL) && reference.Scope == siteURL &&
		reference.Ref == canonicalRef(jiraProviderID, mentionKindIssue, siteURL, reference.ID) &&
		reference.URL == siteURL+"/browse/"+url.PathEscape(reference.Key)
}

func validConfiguredJiraSiteURL(siteURL string) bool {
	if siteURL == "" || strings.TrimRight(siteURL, "/") != siteURL {
		return false
	}
	parsed, err := url.Parse(siteURL)
	return err == nil && parsed.User == nil && parsed.Host != "" &&
		(parsed.Scheme == "http" || parsed.Scheme == "https") &&
		parsed.RawQuery == "" && parsed.Fragment == "" && parsed.Opaque == ""
}
