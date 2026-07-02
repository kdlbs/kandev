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
// All repos clears org/repo selections.
func (s *Service) UpdateWorkspaceSettings(ctx context.Context, req *UpdateWorkspaceSettingsRequest) (*WorkspaceSettings, error) {
	if req == nil || strings.TrimSpace(req.WorkspaceID) == "" {
		return nil, fmt.Errorf("%w: workspace_id is required", ErrWorkspaceSettingsValidation)
	}
	if req.RepoScopeMode != nil && !isValidRepoScopeMode(*req.RepoScopeMode) {
		return nil, fmt.Errorf("%w: invalid repo_scope_mode %q", ErrWorkspaceSettingsValidation, *req.RepoScopeMode)
	}
	if req.SavedPresets != nil {
		req.SavedPresetsSet = true
	}
	if req.DefaultQueryPresets != nil {
		req.DefaultQueriesSet = true
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
	case RepoScopeModeOrgs:
		return len(settings.RepoScopeOrgs) > 0
	case RepoScopeModeRepos:
		return len(settings.RepoScopeRepos) > 0
	default:
		return false
	}
}

func isValidRepoScopeMode(mode string) bool {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case RepoScopeModeAll, RepoScopeModeOrgs, RepoScopeModeRepos:
		return true
	default:
		return false
	}
}

func appendWorkspaceScopeQuery(customQuery string, settings *WorkspaceSettings) string {
	qualifiers := workspaceScopeQualifiers(settings)
	if len(qualifiers) == 0 {
		return customQuery
	}
	return strings.TrimSpace(strings.Join(append([]string{customQuery}, qualifiers...), " "))
}

func appendWorkspaceScopeToSearch(filter, customQuery string, settings *WorkspaceSettings) (string, string) {
	if strings.TrimSpace(customQuery) != "" {
		return filter, appendWorkspaceScopeQuery(customQuery, settings)
	}
	return appendWorkspaceScopeQuery(filter, settings), customQuery
}

func workspaceScopeQualifiers(settings *WorkspaceSettings) []string {
	settings = normalizeWorkspaceSettings(settings)
	if settings == nil {
		return nil
	}
	switch settings.RepoScopeMode {
	case RepoScopeModeOrgs:
		qualifiers := make([]string, 0, len(settings.RepoScopeOrgs))
		for _, org := range settings.RepoScopeOrgs {
			if org = strings.TrimSpace(org); org != "" {
				qualifiers = append(qualifiers, "org:"+org)
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
