package mentions

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/kandev/kandev/internal/sentry"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	sentrySource         = "sentry_issues"
	sentryProviderID     = "sentry"
	sentryIssueKind      = "issue"
	sentryDisplayName    = "Sentry"
	sentryIssueLabel     = "Issue"
	sentryScopeDelimiter = "|"
	sentrySchemeHTTP     = "http"
	sentrySchemeHTTPS    = "https"
	sentryIssuesSegment  = "issues"
	sentryOrgsSegment    = "organizations"
)

// SentryMentionService is Sentry's workspace-explicit mention boundary.
type SentryMentionService interface {
	SearchMentionIssues(
		ctx context.Context,
		workspaceID, query string,
		limit int,
	) ([]sentry.MentionIssue, error)
	MentionInstanceForWorkspace(
		ctx context.Context,
		workspaceID, instanceID string,
	) (*sentry.MentionInstance, error)
}

type sentryIssueProvider struct {
	service SentryMentionService
}

// NewSentryIssueProvider creates Sentry's built-in issue mention source.
func NewSentryIssueProvider(service SentryMentionService) MentionProvider {
	return &sentryIssueProvider{service: service}
}

func (p *sentryIssueProvider) Descriptor() ProviderDescriptor {
	return ProviderDescriptor{
		Source:      sentrySource,
		Provider:    sentryProviderID,
		Kind:        sentryIssueKind,
		DisplayName: sentryDisplayName,
		KindLabel:   sentryIssueLabel,
		Order:       70,
	}
}

func (p *sentryIssueProvider) Search(ctx context.Context, request SearchRequest) ([]Candidate, error) {
	if p == nil || p.service == nil {
		return nil, NewProviderError(StatusNotConfigured)
	}
	request.WorkspaceID = strings.TrimSpace(request.WorkspaceID)
	if request.WorkspaceID == "" {
		return nil, NewProviderError(StatusUnsupportedScope, sentry.ErrMentionWorkspaceRequired)
	}
	request.Query = strings.TrimSpace(request.Query)
	if request.Query == "" {
		return nil, NewProviderError(StatusUpstreamError, sentry.ErrMentionQueryRequired)
	}
	limit := normalizedSentryProviderLimit(request.Limit)
	issues, err := p.service.SearchMentionIssues(ctx, request.WorkspaceID, request.Query, limit)
	if err != nil {
		return nil, mapSentryProviderError(err)
	}
	count := min(len(issues), limit)
	candidates := make([]Candidate, 0, count)
	for index := 0; index < count; index++ {
		issue := issues[index]
		candidates = append(candidates, Candidate{
			ID:    issue.ID,
			Key:   issue.ShortID,
			Title: issue.Title,
			URL:   issue.URL,
			Scope: sentryReferenceScope(issue.InstanceID, issue.InstanceOrigin),
		})
	}
	return candidates, nil
}

func normalizedSentryProviderLimit(limit int) int {
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

func mapSentryProviderError(err error) error {
	status := StatusUpstreamError
	switch {
	case errors.Is(err, sentry.ErrNotConfigured):
		status = StatusNotConfigured
	case errors.Is(err, sentry.ErrMentionWorkspaceRequired),
		errors.Is(err, sentry.ErrMentionScopeLimit),
		errors.Is(err, sentry.ErrMentionInvalidOrigin),
		errors.Is(err, sentry.ErrInstanceRequired),
		errors.Is(err, sentry.ErrInstanceNotFound):
		status = StatusUnsupportedScope
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded), isSentryTimeout(err):
		status = StatusTimeout
	default:
		var apiErr *sentry.APIError
		if errors.As(err, &apiErr) {
			status = sentryAPIErrorStatus(apiErr.StatusCode)
		}
	}
	return NewProviderError(status, err)
}

func isSentryTimeout(err error) bool {
	var timeoutErr net.Error
	return errors.As(err, &timeoutErr) && timeoutErr.Timeout()
}

func sentryAPIErrorStatus(statusCode int) Status {
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

func (p *sentryIssueProvider) AuthorizeReference(ctx context.Context, request ReferenceAuthorizationRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	workspaceID := strings.TrimSpace(request.WorkspaceID)
	instanceID, origin, ok := validSentryReference(request.Reference)
	if workspaceID == "" || workspaceID != request.WorkspaceID || !ok {
		return ErrReferenceUnauthorized
	}
	if request.Purpose == ReferencePurposeSearch {
		return nil
	}
	if request.Purpose != ReferencePurposeSubmission || p == nil || p.service == nil {
		return ErrReferenceUnauthorized
	}

	instance, err := p.service.MentionInstanceForWorkspace(ctx, workspaceID, instanceID)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return ErrReferenceUnauthorized
	}
	if !sentryInstanceMatchesReference(instance, workspaceID, instanceID, origin, request.Reference.Scope) {
		return ErrReferenceUnauthorized
	}
	return nil
}

func sentryInstanceMatchesReference(
	instance *sentry.MentionInstance,
	workspaceID, instanceID, origin, scope string,
) bool {
	return instance != nil && instance.ID == instanceID && instance.WorkspaceID == workspaceID &&
		instance.Origin == origin && scope == sentryReferenceScope(instance.ID, instance.Origin)
}

func validSentryReference(reference apiv1.EntityReference) (string, string, bool) {
	instanceID, origin, scopeOK := parseSentryReferenceScope(reference.Scope)
	if !scopeOK || reference.Version != apiv1.EntityReferenceVersion ||
		reference.Provider != sentryProviderID || reference.Kind != sentryIssueKind ||
		reference.ID == "" || reference.ID != strings.TrimSpace(reference.ID) ||
		reference.Key != strings.TrimSpace(reference.Key) ||
		reference.Ref != canonicalRef(sentryProviderID, sentryIssueKind, reference.Scope, reference.ID) ||
		!validSentryDestination(reference.URL, origin, reference.ID, reference.Key) {
		return "", "", false
	}
	return instanceID, origin, true
}

func sentryReferenceScope(instanceID, origin string) string {
	return instanceID + sentryScopeDelimiter + origin
}

func parseSentryReferenceScope(scope string) (string, string, bool) {
	instanceID, origin, found := strings.Cut(scope, sentryScopeDelimiter)
	if !found || instanceID == "" || instanceID != strings.TrimSpace(instanceID) ||
		strings.Contains(origin, sentryScopeDelimiter) || !validSentryOrigin(origin) ||
		scope != sentryReferenceScope(instanceID, origin) {
		return "", "", false
	}
	return instanceID, origin, true
}

func validSentryOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.User != nil || parsed.Opaque != "" || parsed.Host == "" ||
		(parsed.Scheme != sentrySchemeHTTP && parsed.Scheme != sentrySchemeHTTPS) || parsed.Path != "" ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	canonical := strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host)
	return origin == canonical
}

func validSentryDestination(rawURL, origin, issueID, issueKey string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.User != nil || parsed.Opaque != "" || parsed.RawQuery != "" ||
		parsed.ForceQuery || parsed.Fragment != "" ||
		strings.ToLower(parsed.Scheme)+"://"+strings.ToLower(parsed.Host) != origin {
		return false
	}
	escapedPath := strings.TrimSuffix(parsed.EscapedPath(), "/")
	if !strings.HasPrefix(escapedPath, "/") {
		return false
	}
	rawSegments := strings.Split(strings.TrimPrefix(escapedPath, "/"), "/")
	segments := make([]string, len(rawSegments))
	for index, segment := range rawSegments {
		segments[index], err = decodeCanonicalSentryPathSegment(segment)
		if err != nil {
			return false
		}
	}
	if !validSentryIssuePath(segments) {
		return false
	}
	target := segments[len(segments)-1]
	return target == issueID || issueKey != "" && target == issueKey
}

func validSentryIssuePath(segments []string) bool {
	switch len(segments) {
	case 2:
		return segments[0] == sentryIssuesSegment
	case 4:
		return segments[0] == sentryOrgsSegment && segments[1] != "" && segments[2] == sentryIssuesSegment
	default:
		return false
	}
}

func decodeCanonicalSentryPathSegment(segment string) (string, error) {
	decoded, err := url.PathUnescape(segment)
	if err != nil || decoded == "" || decoded == "." || decoded == ".." ||
		strings.ContainsAny(decoded, "/\\") || url.PathEscape(decoded) != segment {
		return "", errors.New("non-canonical Sentry path segment")
	}
	return decoded, nil
}
