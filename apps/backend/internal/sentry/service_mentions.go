package sentry

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/sync/errgroup"
)

const (
	defaultMentionLimit         = 5
	maxMentionLimit             = 10
	maxMentionInstances         = 8
	maxMentionOrganizations     = 8
	maxMentionSearchConcurrency = 4
)

var (
	ErrMentionWorkspaceRequired = errors.New("sentry mention search: workspace ID is required")
	ErrMentionQueryRequired     = errors.New("sentry mention search: query is required")
	ErrMentionScopeLimit        = errors.New("sentry mention search: scope limit exceeded")
	ErrMentionInvalidOrigin     = errors.New("sentry mention search: invalid instance origin")
	ErrMentionMalformedResponse = errors.New("sentry mention search: malformed upstream response")
)

// MentionIssue is the provider-local immutable projection used by mention search.
type MentionIssue struct {
	InstanceID       string
	InstanceOrigin   string
	OrganizationSlug string
	ID               string
	ShortID          string
	Title            string
	URL              string
}

// MentionInstance is the non-secret ownership and origin data needed to
// authorize a persisted Sentry reference.
type MentionInstance struct {
	ID          string
	WorkspaceID string
	Origin      string
}

type mentionOrganizationLister interface {
	ListOrganizationsLimited(context.Context, int) ([]SentryOrganization, bool, error)
}

type mentionIssueSearcher interface {
	SearchIssuesLimited(context.Context, SearchFilter, string, int) (*SearchResult, error)
}

type mentionInstanceScope struct {
	instance      MentionInstance
	client        Client
	organizations []SentryOrganization
}

type mentionSearchJob struct {
	instance     MentionInstance
	client       Client
	organization SentryOrganization
}

// SearchMentionIssues searches every bounded Sentry scope in one workspace.
func (s *Service) SearchMentionIssues(
	ctx context.Context,
	workspaceID, query string,
	limit int,
) ([]MentionIssue, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, ErrMentionWorkspaceRequired
	}
	literalQuery, err := quoteMentionQuery(query)
	if err != nil {
		return nil, err
	}
	if s == nil || s.store == nil {
		return nil, ErrNotConfigured
	}

	instances, err := s.store.ListInstances(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list Sentry mention instances: %w", err)
	}
	if len(instances) == 0 {
		return nil, ErrNotConfigured
	}
	if len(instances) > maxMentionInstances {
		return nil, fmt.Errorf("%w: workspace has more than %d instances", ErrMentionScopeLimit, maxMentionInstances)
	}

	scopes, err := s.discoverMentionScopes(ctx, workspaceID, instances)
	if err != nil {
		return nil, err
	}
	return searchMentionScopes(ctx, scopes, literalQuery, normalizeMentionLimit(limit))
}

func (s *Service) discoverMentionScopes(
	ctx context.Context,
	workspaceID string,
	configs []*SentryConfig,
) ([]mentionInstanceScope, error) {
	scopes := make([]mentionInstanceScope, len(configs))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(maxMentionSearchConcurrency)
	for index, config := range configs {
		index, config := index, config
		group.Go(func() error {
			scope, err := s.discoverMentionScope(groupCtx, workspaceID, config)
			if err != nil {
				return err
			}
			scopes[index] = scope
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, fmt.Errorf("discover Sentry mention scope: %w", err)
	}
	return scopes, nil
}

func (s *Service) discoverMentionScope(
	ctx context.Context,
	workspaceID string,
	config *SentryConfig,
) (mentionInstanceScope, error) {
	instance, err := mentionInstanceFromConfig(config)
	if err != nil {
		return mentionInstanceScope{}, err
	}
	client, err := s.browseClient(ctx, workspaceID, config.ID)
	if err != nil {
		return mentionInstanceScope{}, err
	}
	if client == nil {
		return mentionInstanceScope{}, ErrNotConfigured
	}
	organizations, err := listMentionOrganizations(ctx, client)
	if err != nil {
		return mentionInstanceScope{}, err
	}
	return mentionInstanceScope{
		instance:      *instance,
		client:        client,
		organizations: organizations,
	}, nil
}

func listMentionOrganizations(ctx context.Context, client Client) ([]SentryOrganization, error) {
	var (
		organizations []SentryOrganization
		more          bool
		err           error
	)
	if limited, ok := client.(mentionOrganizationLister); ok {
		organizations, more, err = limited.ListOrganizationsLimited(ctx, maxMentionOrganizations)
	} else {
		organizations, err = client.ListOrganizations(ctx)
		more = len(organizations) > maxMentionOrganizations
	}
	if err != nil {
		return nil, err
	}
	if more || len(organizations) > maxMentionOrganizations {
		return nil, fmt.Errorf("%w: instance has more than %d organizations", ErrMentionScopeLimit, maxMentionOrganizations)
	}

	out := make([]SentryOrganization, 0, len(organizations))
	seen := make(map[string]struct{}, len(organizations))
	for _, organization := range organizations {
		organization.Slug = strings.TrimSpace(organization.Slug)
		if organization.Slug == "" {
			continue
		}
		if _, duplicate := seen[organization.Slug]; duplicate {
			continue
		}
		seen[organization.Slug] = struct{}{}
		out = append(out, organization)
	}
	return out, nil
}

func searchMentionScopes(
	ctx context.Context,
	scopes []mentionInstanceScope,
	literalQuery string,
	limit int,
) ([]MentionIssue, error) {
	jobs := buildMentionSearchJobs(scopes)
	results := make([][]MentionIssue, len(jobs))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(maxMentionSearchConcurrency)
	for index, job := range jobs {
		index, job := index, job
		group.Go(func() error {
			issues, err := searchMentionJob(groupCtx, job, literalQuery, limit)
			if err != nil {
				return err
			}
			results[index] = issues
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, fmt.Errorf("search Sentry mention scope: %w", err)
	}
	return flattenMentionResults(results, limit), nil
}

func buildMentionSearchJobs(scopes []mentionInstanceScope) []mentionSearchJob {
	jobs := make([]mentionSearchJob, 0)
	for _, scope := range scopes {
		for _, organization := range scope.organizations {
			jobs = append(jobs, mentionSearchJob{
				instance:     scope.instance,
				client:       scope.client,
				organization: organization,
			})
		}
	}
	return jobs
}

func searchMentionJob(
	ctx context.Context,
	job mentionSearchJob,
	literalQuery string,
	limit int,
) ([]MentionIssue, error) {
	filter := SearchFilter{OrgSlug: job.organization.Slug, Query: literalQuery}
	var (
		result *SearchResult
		err    error
	)
	if limited, ok := job.client.(mentionIssueSearcher); ok {
		result, err = limited.SearchIssuesLimited(ctx, filter, "", limit)
	} else {
		result, err = job.client.SearchIssues(ctx, filter, "")
	}
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, ErrMentionMalformedResponse
	}
	count := min(len(result.Issues), limit)
	issues := make([]MentionIssue, 0, count)
	for index := 0; index < count; index++ {
		issue := result.Issues[index]
		issues = append(issues, MentionIssue{
			InstanceID:       job.instance.ID,
			InstanceOrigin:   job.instance.Origin,
			OrganizationSlug: job.organization.Slug,
			ID:               issue.ID,
			ShortID:          issue.ShortID,
			Title:            issue.Title,
			URL:              issue.Permalink,
		})
	}
	return issues, nil
}

func flattenMentionResults(results [][]MentionIssue, limit int) []MentionIssue {
	out := make([]MentionIssue, 0, limit)
	seen := make(map[string]struct{}, limit)
	for _, group := range results {
		for _, issue := range group {
			key := issue.InstanceID + "\x00" + issue.InstanceOrigin + "\x00" + issue.ID
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, issue)
			if len(out) == limit {
				return out
			}
		}
	}
	return out
}

// MentionInstanceForWorkspace reloads one workspace-owned instance.
func (s *Service) MentionInstanceForWorkspace(
	ctx context.Context,
	workspaceID, instanceID string,
) (*MentionInstance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspaceID = strings.TrimSpace(workspaceID)
	instanceID = strings.TrimSpace(instanceID)
	if workspaceID == "" {
		return nil, ErrMentionWorkspaceRequired
	}
	if instanceID == "" || s == nil || s.store == nil {
		return nil, ErrInstanceNotFound
	}
	config, err := s.store.GetInstance(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	if config == nil || config.WorkspaceID != workspaceID {
		return nil, ErrInstanceNotFound
	}
	return mentionInstanceFromConfig(config)
}

func mentionInstanceFromConfig(config *SentryConfig) (*MentionInstance, error) {
	if config == nil || strings.TrimSpace(config.ID) == "" || strings.TrimSpace(config.WorkspaceID) == "" {
		return nil, ErrInstanceNotFound
	}
	origin, err := normalizeMentionOrigin(config.URL)
	if err != nil {
		return nil, err
	}
	return &MentionInstance{
		ID:          config.ID,
		WorkspaceID: config.WorkspaceID,
		Origin:      origin,
	}, nil
}

func normalizeMentionOrigin(raw string) (string, error) {
	parsed, err := url.Parse(resolveBaseURL(raw))
	if err != nil || parsed.User != nil || parsed.Opaque != "" || parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", ErrMentionInvalidOrigin
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), nil
}

func quoteMentionQuery(raw string) (string, error) {
	query := strings.TrimSpace(raw)
	if query == "" {
		return "", ErrMentionQueryRequired
	}
	if !utf8.ValidString(query) || utf8.RuneCountInString(query) > 200 {
		return "", ErrMentionQueryRequired
	}
	for _, character := range query {
		if unicode.IsControl(character) {
			return "", ErrMentionQueryRequired
		}
	}
	query = strings.ReplaceAll(query, `\`, `\\`)
	query = strings.ReplaceAll(query, `"`, `\"`)
	return `"` + query + `"`, nil
}

func normalizeMentionLimit(limit int) int {
	switch {
	case limit == 0:
		return defaultMentionLimit
	case limit < 1:
		return 1
	case limit > maxMentionLimit:
		return maxMentionLimit
	default:
		return limit
	}
}
