package service

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
)

var ErrSidebarTaskViewUnavailable = errors.New("sidebar task view query is unavailable")

type sidebarTaskViewQueryRepository interface {
	QuerySidebarTaskPage(
		context.Context,
		string,
		models.SidebarTaskViewQuery,
		models.SidebarTaskViewPreferences,
	) (*models.SidebarTaskPageResult, error)
}

// QuerySidebarTaskPage returns one authorized page from the complete sidebar view.
func (s *Service) QuerySidebarTaskPage(
	ctx context.Context,
	workspaceID string,
	query models.SidebarTaskViewQuery,
	prefs models.SidebarTaskViewPreferences,
) (*models.SidebarTaskPageResult, error) {
	if err := s.authorizeWorkspaceID(ctx, workspaceID); err != nil {
		return nil, err
	}
	repo, ok := s.tasks.(sidebarTaskViewQueryRepository)
	if !ok {
		return nil, ErrSidebarTaskViewUnavailable
	}
	return repo.QuerySidebarTaskPage(ctx, workspaceID, query, prefs)
}
