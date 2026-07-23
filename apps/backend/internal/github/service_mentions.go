package github

import (
	"context"
	"errors"
	"strings"
)

const (
	defaultMentionSearchLimit = 5
	maxMentionSearchLimit     = 10
)

var (
	ErrMentionWorkspaceRequired = errors.New("GitHub mention workspace is required")
	ErrMentionQueryRequired     = errors.New("GitHub mention query is required")
)

// SearchMentionIssuesForWorkspace translates plain composer text into a
// server-owned, title-only GitHub issue query within one workspace's scope.
func (s *Service) SearchMentionIssuesForWorkspace(
	ctx context.Context,
	workspaceID, query string,
	limit int,
) ([]*Issue, error) {
	workspaceID, query, limit, err := normalizeMentionSearch(workspaceID, query, limit)
	if err != nil {
		return nil, err
	}
	result, err := s.SearchUserIssuesPagedForWorkspace(
		ctx,
		workspaceID,
		"",
		buildMentionTitleQuery("type:issue", query),
		1,
		limit,
	)
	if err != nil {
		return nil, err
	}
	if result == nil || result.Issues == nil {
		return []*Issue{}, nil
	}
	return result.Issues, nil
}

// SearchMentionPullRequestsForWorkspace is the pull-request counterpart to
// SearchMentionIssuesForWorkspace.
func (s *Service) SearchMentionPullRequestsForWorkspace(
	ctx context.Context,
	workspaceID, query string,
	limit int,
) ([]*PR, error) {
	workspaceID, query, limit, err := normalizeMentionSearch(workspaceID, query, limit)
	if err != nil {
		return nil, err
	}
	result, err := s.SearchUserPRsPagedForWorkspace(
		ctx,
		workspaceID,
		"",
		buildMentionTitleQuery("type:pr", query),
		1,
		limit,
	)
	if err != nil {
		return nil, err
	}
	if result == nil || result.PRs == nil {
		return []*PR{}, nil
	}
	return result.PRs, nil
}

func normalizeMentionSearch(workspaceID, query string, limit int) (string, string, int, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return "", "", 0, ErrMentionWorkspaceRequired
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return "", "", 0, ErrMentionQueryRequired
	}
	switch {
	case limit <= 0:
		limit = defaultMentionSearchLimit
	case limit > maxMentionSearchLimit:
		limit = maxMentionSearchLimit
	}
	return workspaceID, query, limit, nil
}

// buildMentionTitleQuery keeps the server-owned type qualifier outside the
// quoted user input so qualifier-like title text cannot change result types.
func buildMentionTitleQuery(typeQualifier, query string) string {
	query = strings.ReplaceAll(query, `\`, `\\`)
	query = strings.ReplaceAll(query, `"`, `\"`)
	return typeQualifier + ` in:title "` + query + `"`
}
