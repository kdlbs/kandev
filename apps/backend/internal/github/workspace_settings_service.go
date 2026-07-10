package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrWorkspaceSettingsValidation = errors.New("invalid workspace settings")

// GetWorkspaceSettings returns the GitHub operational settings for a workspace.
func (s *Service) GetWorkspaceSettings(ctx context.Context, workspaceID string) (*WorkspaceSettings, error) {
	if s.store == nil {
		return defaultWorkspaceSettings(workspaceID), nil
	}
	return s.store.GetWorkspaceSettings(ctx, workspaceID)
}

// UpsertWorkspaceSettings stores the GitHub operational settings for a workspace.
func (s *Service) UpsertWorkspaceSettings(ctx context.Context, settings *WorkspaceSettings) error {
	if s.store == nil {
		return fmt.Errorf("github store not configured")
	}
	return s.store.UpsertWorkspaceSettings(ctx, settings)
}

// UpdateWorkspaceSettings applies a partial update over the existing workspace
// settings. Scope fields are intentionally updated as a set so switching to
// All repos clears org/repo selections. Direct struct callers must set
// SavedPresetsSet/DefaultQueriesSet when they intend to write those blobs;
// JSON-bound requests set those flags in UpdateWorkspaceSettingsRequest.UnmarshalJSON.
func (s *Service) UpdateWorkspaceSettings(ctx context.Context, req *UpdateWorkspaceSettingsRequest) (*WorkspaceSettings, error) {
	if req == nil || strings.TrimSpace(req.WorkspaceID) == "" {
		return nil, fmt.Errorf("%w: workspace_id is required", ErrWorkspaceSettingsValidation)
	}
	if req.RepoScopeMode != nil && !isValidRepoScopeMode(*req.RepoScopeMode) {
		return nil, fmt.Errorf("%w: invalid repo_scope_mode %q", ErrWorkspaceSettingsValidation, *req.RepoScopeMode)
	}
	if s.store == nil {
		return nil, fmt.Errorf("github store not configured")
	}
	return s.store.PatchWorkspaceSettings(ctx, req)
}

func (s *Service) SearchUserPRsPagedForWorkspace(
	ctx context.Context,
	workspaceID string,
	filter string,
	customQuery string,
	page int,
	perPage int,
) (*PRSearchPage, error) {
	settings, err := s.GetWorkspaceSettings(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if !workspaceSettingsHasScope(settings) {
		return s.SearchUserPRsPaged(ctx, filter, customQuery, page, perPage)
	}
	return s.searchUserPRsPagedScoped(ctx, settings, filter, customQuery, page, perPage)
}

func (s *Service) SearchUserIssuesPagedForWorkspace(
	ctx context.Context,
	workspaceID string,
	filter string,
	customQuery string,
	page int,
	perPage int,
) (*IssueSearchPage, error) {
	settings, err := s.GetWorkspaceSettings(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if !workspaceSettingsHasScope(settings) {
		return s.SearchUserIssuesPaged(ctx, filter, customQuery, page, perPage)
	}
	return s.searchUserIssuesPagedScoped(ctx, settings, filter, customQuery, page, perPage)
}

func (s *Service) searchUserPRsPagedScoped(
	ctx context.Context,
	settings *WorkspaceSettings,
	filter string,
	customQuery string,
	page int,
	perPage int,
) (*PRSearchPage, error) {
	if workspaceSettingsHasEmptyScope(settings) {
		return &PRSearchPage{PRs: []*PR{}, TotalCount: 0, Page: page, PerPage: perPage}, nil
	}
	qualifiers := workspaceScopeQualifiers(settings)
	if len(qualifiers) > 1 {
		return s.searchUserPRsPagedScopedAcrossQualifiers(ctx, settings, filter, customQuery, page, perPage, qualifiers)
	}
	filter, customQuery = appendWorkspaceScopeToSearch(filter, customQuery, settings)
	v, err := s.searchUserPagedScoped("pr", settings, filter, customQuery, page, perPage, func(page, perPage int) (any, error) {
		result, err := s.client.SearchPRsPaged(ctx, filter, customQuery, page, perPage)
		if err != nil {
			return nil, err
		}
		return scopedPRSearchPage(result, settings, page, perPage), nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*PRSearchPage), nil
}

func (s *Service) searchUserIssuesPagedScoped(
	ctx context.Context,
	settings *WorkspaceSettings,
	filter string,
	customQuery string,
	page int,
	perPage int,
) (*IssueSearchPage, error) {
	if workspaceSettingsHasEmptyScope(settings) {
		return &IssueSearchPage{Issues: []*Issue{}, TotalCount: 0, Page: page, PerPage: perPage}, nil
	}
	qualifiers := workspaceScopeQualifiers(settings)
	if len(qualifiers) > 1 {
		return s.searchUserIssuesPagedScopedAcrossQualifiers(ctx, settings, filter, customQuery, page, perPage, qualifiers)
	}
	filter, customQuery = appendWorkspaceScopeToSearch(filter, customQuery, settings)
	v, err := s.searchUserPagedScoped("issue", settings, filter, customQuery, page, perPage, func(page, perPage int) (any, error) {
		result, err := s.client.ListIssuesPaged(ctx, filter, customQuery, page, perPage)
		if err != nil {
			return nil, err
		}
		return scopedIssueSearchPage(result, settings, page, perPage), nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*IssueSearchPage), nil
}

func (s *Service) searchUserPagedScoped(
	kind string,
	settings *WorkspaceSettings,
	filter string,
	customQuery string,
	page int,
	perPage int,
	fetch func(page int, perPage int) (any, error),
) (any, error) {
	if s.client == nil {
		return nil, fmt.Errorf("github client not available")
	}
	page, perPage = clampSearchPage(page, perPage)
	scopeKey := workspaceSearchScopeKey(settings)
	key := searchCacheKey(kind+":"+scopeKey, filter, customQuery, page, perPage)
	return s.searchCache.doOrFetch(key, func() (any, error) {
		return fetch(page, perPage)
	})
}

func (s *Service) searchUserPRsPagedScopedAcrossQualifiers(
	ctx context.Context,
	settings *WorkspaceSettings,
	filter string,
	customQuery string,
	page int,
	perPage int,
	qualifiers []string,
) (*PRSearchPage, error) {
	result, err := cachedScopedSearchAcrossQualifiers(s.client, s.searchCache, "pr", settings, filter, customQuery, page, perPage, func(page, perPage int) ([]*PR, int, error) {
		return s.fetchScopedPRsAcrossQualifiers(ctx, settings, filter, customQuery, page, perPage, qualifiers)
	})
	if err != nil {
		return nil, err
	}
	return &PRSearchPage{PRs: result.items, TotalCount: result.total, Page: result.page, PerPage: result.perPage}, nil
}

func (s *Service) searchUserIssuesPagedScopedAcrossQualifiers(
	ctx context.Context,
	settings *WorkspaceSettings,
	filter string,
	customQuery string,
	page int,
	perPage int,
	qualifiers []string,
) (*IssueSearchPage, error) {
	result, err := cachedScopedSearchAcrossQualifiers(s.client, s.searchCache, "issue", settings, filter, customQuery, page, perPage, func(page, perPage int) ([]*Issue, int, error) {
		return s.fetchScopedIssuesAcrossQualifiers(ctx, settings, filter, customQuery, page, perPage, qualifiers)
	})
	if err != nil {
		return nil, err
	}
	return &IssueSearchPage{Issues: result.items, TotalCount: result.total, Page: result.page, PerPage: result.perPage}, nil
}

type scopedFanoutResult[T any] struct {
	items   []T
	total   int
	page    int
	perPage int
}

func cachedScopedSearchAcrossQualifiers[T any](
	client Client,
	cache *ttlCache,
	kind string,
	settings *WorkspaceSettings,
	filter string,
	customQuery string,
	page int,
	perPage int,
	fetch func(page int, perPage int) ([]T, int, error),
) (scopedFanoutResult[T], error) {
	if client == nil {
		return scopedFanoutResult[T]{}, fmt.Errorf("github client not available")
	}
	page, perPage = clampSearchPage(page, perPage)
	scopeKey := workspaceSearchScopeKey(settings)
	key := searchCacheKey(kind+":"+scopeKey+":fanout", filter, customQuery, page, perPage)
	v, err := cache.doOrFetch(key, func() (any, error) {
		items, total, err := fetch(page, perPage)
		if err != nil {
			return nil, err
		}
		return scopedFanoutResult[T]{
			items:   paginateSearchResults(items, page, perPage),
			total:   total,
			page:    page,
			perPage: perPage,
		}, nil
	})
	if err != nil {
		return scopedFanoutResult[T]{}, err
	}
	return v.(scopedFanoutResult[T]), nil
}

func (s *Service) fetchScopedPRsAcrossQualifiers(
	ctx context.Context,
	settings *WorkspaceSettings,
	filter string,
	customQuery string,
	page int,
	perPage int,
	qualifiers []string,
) ([]*PR, int, error) {
	return fetchScopedResultsAcrossQualifiers(settings, filter, customQuery, page, perPage, qualifiers, prScopeKey, func(scopedFilter, scopedCustomQuery string, providerPage, fetchPerPage int) ([]*PR, int, error) {
		result, err := s.client.SearchPRsPaged(ctx, scopedFilter, scopedCustomQuery, providerPage, fetchPerPage)
		if err != nil {
			return nil, 0, err
		}
		if result == nil {
			return []*PR{}, 0, nil
		}
		return filterPRsByWorkspaceScope(result.PRs, settings), result.TotalCount, nil
	})
}

func (s *Service) fetchScopedIssuesAcrossQualifiers(
	ctx context.Context,
	settings *WorkspaceSettings,
	filter string,
	customQuery string,
	page int,
	perPage int,
	qualifiers []string,
) ([]*Issue, int, error) {
	return fetchScopedResultsAcrossQualifiers(settings, filter, customQuery, page, perPage, qualifiers, issueScopeKey, func(scopedFilter, scopedCustomQuery string, providerPage, fetchPerPage int) ([]*Issue, int, error) {
		result, err := s.client.ListIssuesPaged(ctx, scopedFilter, scopedCustomQuery, providerPage, fetchPerPage)
		if err != nil {
			return nil, 0, err
		}
		if result == nil {
			return []*Issue{}, 0, nil
		}
		return filterIssuesByWorkspaceScope(result.Issues, settings), result.TotalCount, nil
	})
}

func fetchScopedResultsAcrossQualifiers[T any](
	settings *WorkspaceSettings,
	filter string,
	customQuery string,
	page int,
	perPage int,
	qualifiers []string,
	resultKey func(T) string,
	fetch func(scopedFilter string, scopedCustomQuery string, providerPage int, fetchPerPage int) ([]T, int, error),
) ([]T, int, error) {
	seen := make(map[string]struct{})
	all := make([]T, 0, perPage)
	fetchPerPage := workspaceScopeFanoutFetchPerPage(page, perPage)
	pagesToFetch := workspaceScopeFanoutPagesToFetch(page, perPage, fetchPerPage)
	counts := make([]int, 0, len(qualifiers))
	for _, qualifier := range qualifiers {
		scopedFilter, scopedCustomQuery := appendWorkspaceScopeQualifierToSearch(filter, customQuery, qualifier)
		qualifierCount := 0
		for providerPage := 1; providerPage <= pagesToFetch; providerPage++ {
			items, count, err := fetch(scopedFilter, scopedCustomQuery, providerPage, fetchPerPage)
			if err != nil {
				return nil, 0, err
			}
			if providerPage == 1 {
				qualifierCount = count
			}
			for _, item := range items {
				key := resultKey(item)
				if key == "" {
					continue
				}
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				all = append(all, item)
			}
			if workspaceScopeFanoutFetchedAll(providerPage, fetchPerPage, count, len(items)) {
				break
			}
		}
		counts = append(counts, qualifierCount)
	}
	return all, workspaceScopeFanoutTotal(settings, counts, len(all)), nil
}

func workspaceScopeFanoutFetchPerPage(page, perPage int) int {
	fetchPerPage := page * perPage
	if fetchPerPage < perPage {
		return perPage
	}
	if fetchPerPage > 100 {
		return 100
	}
	return fetchPerPage
}

func workspaceScopeFanoutPagesToFetch(page, perPage, fetchPerPage int) int {
	if fetchPerPage <= 0 {
		return 1
	}
	target := page * perPage
	if target < perPage {
		target = perPage
	}
	pages := target / fetchPerPage
	if target%fetchPerPage != 0 {
		pages++
	}
	if pages < 1 {
		return 1
	}
	return pages
}

func workspaceScopeFanoutFetchedAll(providerPage, fetchPerPage, totalCount, itemCount int) bool {
	if itemCount == 0 {
		return true
	}
	if itemCount < fetchPerPage {
		return true
	}
	return totalCount > 0 && providerPage*fetchPerPage >= totalCount
}

func workspaceScopeFanoutTotal(settings *WorkspaceSettings, counts []int, uniqueFetched int) int {
	settings = normalizeWorkspaceSettings(settings)
	if settings == nil || settings.RepoScopeMode != RepoScopeModeOrgs {
		return sumInts(counts)
	}
	total := 0
	for i := 0; i < len(counts); i += 2 {
		if i+1 >= len(counts) {
			total += counts[i]
			continue
		}
		total += max(counts[i], counts[i+1])
	}
	if uniqueFetched > total {
		return uniqueFetched
	}
	return total
}

func sumInts(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func paginateSearchResults[T any](items []T, page, perPage int) []T {
	if len(items) == 0 {
		return []T{}
	}
	start := (page - 1) * perPage
	if start >= len(items) {
		return []T{}
	}
	end := start + perPage
	if end > len(items) {
		end = len(items)
	}
	return append([]T(nil), items[start:end]...)
}

func prScopeKey(pr *PR) string {
	if pr == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(fmt.Sprintf("%s/%s#%d", pr.RepoOwner, pr.RepoName, pr.Number)))
}

func issueScopeKey(issue *Issue) string {
	if issue == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(fmt.Sprintf("%s/%s#%d", issue.RepoOwner, issue.RepoName, issue.Number)))
}

func scopedPRSearchPage(result *PRSearchPage, settings *WorkspaceSettings, page int, perPage int) *PRSearchPage {
	if result == nil {
		result = &PRSearchPage{Page: page, PerPage: perPage}
	}
	result.PRs = filterPRsByWorkspaceScope(result.PRs, settings)
	result.Page = page
	result.PerPage = perPage
	return result
}

func scopedIssueSearchPage(result *IssueSearchPage, settings *WorkspaceSettings, page int, perPage int) *IssueSearchPage {
	if result == nil {
		result = &IssueSearchPage{Page: page, PerPage: perPage}
	}
	result.Issues = filterIssuesByWorkspaceScope(result.Issues, settings)
	result.Page = page
	result.PerPage = perPage
	return result
}

func workspaceSettingsHasScope(settings *WorkspaceSettings) bool {
	settings = normalizeWorkspaceSettings(settings)
	if settings == nil {
		return false
	}
	switch settings.RepoScopeMode {
	case RepoScopeModeOrgs, RepoScopeModeRepos:
		return true
	default:
		return false
	}
}

func workspaceSettingsHasEmptyScope(settings *WorkspaceSettings) bool {
	return workspaceSettingsHasScope(settings) && len(workspaceScopeQualifiers(settings)) == 0
}

func isValidRepoScopeMode(mode string) bool {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case RepoScopeModeAll, RepoScopeModeOrgs, RepoScopeModeRepos:
		return true
	default:
		return false
	}
}

func appendWorkspaceScopeToSearch(filter, customQuery string, settings *WorkspaceSettings) (string, string) {
	qualifiers := workspaceScopeQualifiers(settings)
	if len(qualifiers) == 0 {
		return filter, customQuery
	}
	return appendWorkspaceScopeQualifierToSearch(filter, customQuery, qualifiers[0])
}

func appendWorkspaceScopeQualifierToSearch(filter, customQuery, qualifier string) (string, string) {
	if strings.TrimSpace(customQuery) != "" {
		return filter, appendWorkspaceScopeQualifier(customQuery, qualifier)
	}
	return appendWorkspaceScopeQualifier(filter, qualifier), customQuery
}

func appendWorkspaceScopeQualifier(query, qualifier string) string {
	return strings.TrimSpace(strings.Join([]string{query, qualifier}, " "))
}

func workspaceScopeQualifiers(settings *WorkspaceSettings) []string {
	settings = normalizeWorkspaceSettings(settings)
	if settings == nil {
		return nil
	}
	switch settings.RepoScopeMode {
	case RepoScopeModeOrgs:
		qualifiers := make([]string, 0, len(settings.RepoScopeOrgs)*2)
		for _, org := range settings.RepoScopeOrgs {
			if org = strings.TrimSpace(org); org != "" {
				qualifiers = append(qualifiers, "org:"+org)
				qualifiers = append(qualifiers, "user:"+org)
			}
		}
		return qualifiers
	case RepoScopeModeRepos:
		qualifiers := make([]string, 0, len(settings.RepoScopeRepos))
		for _, repo := range settings.RepoScopeRepos {
			if repo.Owner != "" && repo.Name != "" {
				qualifiers = append(qualifiers, repoFilterToQualifier(repo))
			}
		}
		return qualifiers
	default:
		return nil
	}
}

func workspaceScopeRepoFilters(settings *WorkspaceSettings) []RepoFilter {
	settings = normalizeWorkspaceSettings(settings)
	if settings == nil {
		return nil
	}
	switch settings.RepoScopeMode {
	case RepoScopeModeOrgs:
		repos := make([]RepoFilter, 0, len(settings.RepoScopeOrgs))
		for _, org := range settings.RepoScopeOrgs {
			if org = strings.TrimSpace(org); org != "" {
				repos = append(repos, RepoFilter{Owner: org})
			}
		}
		return repos
	case RepoScopeModeRepos:
		return append([]RepoFilter(nil), settings.RepoScopeRepos...)
	default:
		return nil
	}
}

func workspaceSearchScopeKey(settings *WorkspaceSettings) string {
	settings = normalizeWorkspaceSettings(settings)
	if settings == nil {
		return RepoScopeModeAll
	}
	var parts []string
	switch settings.RepoScopeMode {
	case RepoScopeModeOrgs:
		parts = append(parts, settings.RepoScopeOrgs...)
	case RepoScopeModeRepos:
		for _, repo := range settings.RepoScopeRepos {
			parts = append(parts, repo.Owner+"/"+repo.Name)
		}
	}
	return settings.RepoScopeMode + ":" + strings.Join(parts, ",")
}

func filterPRsByWorkspaceScope(prs []*PR, settings *WorkspaceSettings) []*PR {
	if !workspaceSettingsHasScope(settings) {
		if prs == nil {
			return []*PR{}
		}
		return prs
	}
	out := make([]*PR, 0, len(prs))
	for _, pr := range prs {
		if pr != nil && repoAllowedByWorkspaceScope(pr.RepoOwner, pr.RepoName, settings) {
			out = append(out, pr)
		}
	}
	return out
}

func filterIssuesByWorkspaceScope(issues []*Issue, settings *WorkspaceSettings) []*Issue {
	if !workspaceSettingsHasScope(settings) {
		if issues == nil {
			return []*Issue{}
		}
		return issues
	}
	out := make([]*Issue, 0, len(issues))
	for _, issue := range issues {
		if issue != nil && repoAllowedByWorkspaceScope(issue.RepoOwner, issue.RepoName, settings) {
			out = append(out, issue)
		}
	}
	return out
}

func repoAllowedByWorkspaceScope(owner, name string, settings *WorkspaceSettings) bool {
	settings = normalizeWorkspaceSettings(settings)
	if settings == nil || settings.RepoScopeMode == RepoScopeModeAll {
		return true
	}
	owner = strings.ToLower(strings.TrimSpace(owner))
	name = strings.ToLower(strings.TrimSpace(name))
	switch settings.RepoScopeMode {
	case RepoScopeModeOrgs:
		for _, org := range settings.RepoScopeOrgs {
			if strings.ToLower(org) == owner {
				return true
			}
		}
	case RepoScopeModeRepos:
		for _, repo := range settings.RepoScopeRepos {
			if strings.ToLower(repo.Owner) == owner && strings.ToLower(repo.Name) == name {
				return true
			}
		}
	}
	return false
}
