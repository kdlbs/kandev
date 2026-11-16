package service

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

var ErrSidebarTaskViewUnavailable = errors.New("sidebar task view query is unavailable")

type sidebarGroupingResolver interface {
	ResolveSidebarGrouping(context.Context, string, string) (*models.SidebarPluginGrouping, error)
}

// SetSidebarGroupingResolver connects the supervised plugin classification owner.
func (s *Service) SetSidebarGroupingResolver(resolver sidebarGroupingResolver) {
	s.sidebarGroupingResolver = resolver
}

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
	if err := query.Validate(); err != nil {
		return nil, err
	}
	prefs.PluginGrouping = nil
	if _, _, plugin := pluginsdk.ParseSidebarGroupingReference(query.Group); plugin {
		if s.sidebarGroupingResolver == nil {
			return nil, models.ErrSidebarGroupingUnavailable
		}
		grouping, err := s.sidebarGroupingResolver.ResolveSidebarGrouping(ctx, workspaceID, query.Group)
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		if err != nil || grouping == nil || grouping.Snapshot == nil {
			return nil, models.ErrSidebarGroupingUnavailable
		}
		if query.Page > 1 && (query.GroupingRevision != grouping.Snapshot.Revision || query.GroupingGeneration != grouping.Generation) {
			query.Page = 1
		}
		query.GroupingRevision = grouping.Snapshot.Revision
		query.GroupingGeneration = grouping.Generation
		prefs.PluginGrouping = grouping
	}
	repo, ok := s.tasks.(sidebarTaskViewQueryRepository)
	if !ok {
		return nil, ErrSidebarTaskViewUnavailable
	}
	return repo.QuerySidebarTaskPage(ctx, workspaceID, query, prefs)
}
