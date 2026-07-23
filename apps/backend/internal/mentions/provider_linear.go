package mentions

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/kandev/kandev/internal/linear"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	linearSource      = "linear_issues"
	linearProviderID  = "linear"
	linearIssueKind   = "issue"
	linearDisplayName = "Linear"
	linearIssueLabel  = "Issue"
	linearScheme      = "https"
	linearAppHost     = "linear.app"
)

// LinearMentionService is the workspace-scoped Linear surface needed by the
// mention provider and its submission authorizer.
type LinearMentionService interface {
	SearchMentionIssues(
		ctx context.Context,
		workspaceID, query string,
		limit int,
	) ([]linear.LinearIssue, string, error)
	GetConfigForWorkspace(ctx context.Context, workspaceID string) (*linear.LinearConfig, error)
}

type linearProvider struct {
	service LinearMentionService
}

// NewLinearProvider creates the Linear issue mention source.
func NewLinearProvider(service LinearMentionService) MentionProvider {
	return &linearProvider{service: service}
}

func (p *linearProvider) Descriptor() ProviderDescriptor {
	return ProviderDescriptor{
		Source:      linearSource,
		Provider:    linearProviderID,
		Kind:        linearIssueKind,
		DisplayName: linearDisplayName,
		KindLabel:   linearIssueLabel,
		Order:       30,
	}
}

func (p *linearProvider) Search(ctx context.Context, request SearchRequest) ([]Candidate, error) {
	if p == nil || p.service == nil {
		return nil, NewProviderError(StatusNotConfigured)
	}
	request.WorkspaceID = strings.TrimSpace(request.WorkspaceID)
	if request.WorkspaceID == "" {
		return nil, NewProviderError(StatusUnsupportedScope, linear.ErrMentionWorkspaceRequired)
	}
	request.Query = strings.TrimSpace(request.Query)
	if request.Query == "" {
		return nil, NewProviderError(StatusUpstreamError, linear.ErrMentionQueryRequired)
	}

	limit := normalizedLinearProviderLimit(request.Limit)
	issues, orgSlug, err := p.service.SearchMentionIssues(
		ctx,
		request.WorkspaceID,
		request.Query,
		limit,
	)
	if err != nil {
		return nil, classifyLinearError(err)
	}
	orgSlug = strings.TrimSpace(orgSlug)
	if orgSlug == "" {
		return nil, NewProviderError(StatusUnsupportedScope, linear.ErrMentionScopeUnavailable)
	}
	count := min(len(issues), limit)
	candidates := make([]Candidate, 0, count)
	for index := 0; index < count; index++ {
		issue := issues[index]
		candidates = append(candidates, Candidate{
			ID:    issue.ID,
			Key:   issue.Identifier,
			Title: issue.Title,
			URL:   issue.URL,
			Scope: orgSlug,
		})
	}
	return candidates, nil
}

func normalizedLinearProviderLimit(limit int) int {
	if limit == 0 {
		return DefaultLimit
	}
	if limit < 1 {
		return 1
	}
	if limit > MaxLimit {
		return MaxLimit
	}
	return limit
}

func classifyLinearError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return NewProviderError(StatusTimeout, err)
	}
	switch {
	case errors.Is(err, linear.ErrNotConfigured):
		return NewProviderError(StatusNotConfigured, err)
	case errors.Is(err, linear.ErrMentionWorkspaceRequired),
		errors.Is(err, linear.ErrMentionScopeUnavailable):
		return NewProviderError(StatusUnsupportedScope, err)
	}
	var apiErr *linear.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 401, 403:
			return NewProviderError(StatusUnauthorized, err)
		case 408, 504:
			return NewProviderError(StatusTimeout, err)
		case 429:
			return NewProviderError(StatusRateLimited, err)
		}
	}
	return NewProviderError(StatusUpstreamError, err)
}

func (p *linearProvider) AuthorizeReference(ctx context.Context, request ReferenceAuthorizationRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	reference := request.Reference
	workspaceID := strings.TrimSpace(request.WorkspaceID)
	if workspaceID == "" || !validLinearReference(reference) {
		return ErrReferenceUnauthorized
	}
	if request.Purpose == ReferencePurposeSearch {
		return nil
	}
	if request.Purpose != ReferencePurposeSubmission || p == nil || p.service == nil {
		return ErrReferenceUnauthorized
	}

	config, err := p.service.GetConfigForWorkspace(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return ErrReferenceUnauthorized
	}
	if config == nil || strings.TrimSpace(config.WorkspaceID) != workspaceID ||
		strings.TrimSpace(config.OrgSlug) != reference.Scope {
		return ErrReferenceUnauthorized
	}
	return nil
}

func validLinearReference(reference apiv1.EntityReference) bool {
	if reference.Version != apiv1.EntityReferenceVersion ||
		reference.Provider != linearProviderID || reference.Kind != linearIssueKind ||
		reference.ID == "" || reference.ID != strings.TrimSpace(reference.ID) ||
		reference.Key == "" || reference.Key != strings.TrimSpace(reference.Key) ||
		reference.Scope == "" || reference.Scope != strings.TrimSpace(reference.Scope) ||
		reference.Ref != canonicalRef(linearProviderID, linearIssueKind, reference.Scope, reference.ID) {
		return false
	}
	return validLinearDestination(reference.URL, reference.Scope, reference.Key)
}

func validLinearDestination(rawURL, orgSlug, issueKey string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || !validLinearURLBase(parsed) {
		return false
	}
	escapedPath := parsed.EscapedPath()
	if !strings.HasPrefix(escapedPath, "/") {
		return false
	}
	segments := strings.Split(strings.TrimPrefix(escapedPath, "/"), "/")
	if len(segments) != 3 && len(segments) != 4 {
		return false
	}
	decoded := make([]string, len(segments))
	for index, segment := range segments {
		decoded[index], err = decodeCanonicalLinearPathSegment(segment)
		if err != nil {
			return false
		}
	}
	return decoded[0] == orgSlug && decoded[1] == linearIssueKind && decoded[2] == issueKey
}

func validLinearURLBase(parsed *url.URL) bool {
	return parsed.Scheme == linearScheme && parsed.Host == linearAppHost &&
		parsed.User == nil && parsed.Opaque == "" && parsed.RawQuery == "" &&
		!parsed.ForceQuery && parsed.Fragment == ""
}

func decodeCanonicalLinearPathSegment(segment string) (string, error) {
	decoded, err := url.PathUnescape(segment)
	if err != nil || decoded == "" || decoded == "." || decoded == ".." ||
		strings.ContainsAny(decoded, "/\\") || url.PathEscape(decoded) != segment {
		return "", errors.New("non-canonical Linear path segment")
	}
	return decoded, nil
}
