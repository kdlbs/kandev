package github

import (
	"context"
	"fmt"
	"strings"
)

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
		return nil, fmt.Errorf("workspace_id is required")
	}
	current, err := s.GetWorkspaceSettings(ctx, req.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if req.RepoScopeMode != nil {
		current.RepoScopeMode = *req.RepoScopeMode
	}
	if req.RepoScopeOrgs != nil {
		current.RepoScopeOrgs = *req.RepoScopeOrgs
	}
	if req.RepoScopeRepos != nil {
		current.RepoScopeRepos = *req.RepoScopeRepos
	}
	if req.SavedPresets != nil {
		current.SavedPresets = cloneRawMessage(*req.SavedPresets)
	}
	if req.DefaultQueryPresets != nil {
		if string(*req.DefaultQueryPresets) == jsonNullLiteral {
			current.DefaultQueryPresets = nil
		} else {
			current.DefaultQueryPresets = cloneRawMessage(*req.DefaultQueryPresets)
		}
	}
	if err := s.UpsertWorkspaceSettings(ctx, current); err != nil {
		return nil, err
	}
	return s.GetWorkspaceSettings(ctx, req.WorkspaceID)
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
	result.TotalCount = len(result.PRs)
	result.Page = page
	result.PerPage = perPage
	return result
}

func scopedIssueSearchPage(result *IssueSearchPage, settings *WorkspaceSettings, page int, perPage int) *IssueSearchPage {
	if result == nil {
		result = &IssueSearchPage{Page: page, PerPage: perPage}
	}
	result.Issues = filterIssuesByWorkspaceScope(result.Issues, settings)
	result.TotalCount = len(result.Issues)
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
