package linear

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	defaultMentionSearchLimit = 5
	maxMentionSearchLimit     = 10
)

var (
	// ErrMentionWorkspaceRequired prevents mention search from falling back to
	// whichever workspace happens to be active for the current process.
	ErrMentionWorkspaceRequired = errors.New("linear mention search: workspace ID is required")
	// ErrMentionQueryRequired prevents an empty typeahead query from becoming an
	// unbounded issue browse request.
	ErrMentionQueryRequired = errors.New("linear mention search: query is required")
	// ErrMentionScopeUnavailable means the configured connection has not yet
	// captured the non-secret organization slug needed to scope references.
	ErrMentionScopeUnavailable = errors.New("linear mention search: organization scope is unavailable")
)

// SearchMentionIssues searches the first bounded issue page for one explicit
// workspace and returns the configured non-secret organization scope.
func (s *Service) SearchMentionIssues(
	ctx context.Context,
	workspaceID, query string,
	limit int,
) ([]LinearIssue, string, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, "", ErrMentionWorkspaceRequired
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, "", ErrMentionQueryRequired
	}
	if s == nil || s.store == nil {
		return nil, "", ErrNotConfigured
	}

	cfg, err := s.store.GetConfigForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, "", fmt.Errorf("load Linear mention configuration: %w", err)
	}
	if cfg == nil {
		return nil, "", ErrNotConfigured
	}
	orgSlug := strings.TrimSpace(cfg.OrgSlug)
	if orgSlug == "" {
		return nil, "", ErrMentionScopeUnavailable
	}

	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return nil, "", fmt.Errorf("create Linear mention client: %w", err)
	}
	limit = normalizeMentionSearchLimit(limit)
	result, err := client.SearchIssues(
		ctx,
		SearchFilter{Query: query},
		"",
		limit,
	)
	if err != nil {
		return nil, "", fmt.Errorf("search Linear mention issues: %w", err)
	}
	if result == nil {
		return nil, "", errors.New("search Linear mention issues: empty response")
	}
	return result.Issues[:min(len(result.Issues), limit)], orgSlug, nil
}

func normalizeMentionSearchLimit(limit int) int {
	if limit == 0 {
		return defaultMentionSearchLimit
	}
	if limit < 1 {
		return 1
	}
	if limit > maxMentionSearchLimit {
		return maxMentionSearchLimit
	}
	return limit
}
