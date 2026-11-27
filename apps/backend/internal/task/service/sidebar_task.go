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
	var groupingKeys []string
	if _, _, plugin := pluginsdk.ParseSidebarGroupingReference(query.Group); plugin {
		grouping, err := s.resolveSidebarTaskGrouping(ctx, workspaceID, query.Group)
		if err != nil {
			return nil, err
		}
		if query.Page > 1 && (query.GroupingRevision != grouping.Snapshot.Revision || query.GroupingGeneration != grouping.Generation) {
			query.Page = 1
		}
		query.GroupingRevision = grouping.Snapshot.Revision
		query.GroupingGeneration = grouping.Generation
		prefs.PluginGrouping = grouping
		collapsed := query.CollapsedGroupKeys
		if query.DisclosureViewID != "" {
			collapsed = prefs.SavedCollapsedGroupKeys
		}
		groupingKeys, query.CollapsedGroupKeys = models.SidebarPluginDisclosure(grouping, collapsed)
		query.DisclosureViewID = ""
		if err := s.authorizeWorkspaceID(ctx, workspaceID); err != nil {
			return nil, err
		}
	}
	repo, ok := s.tasks.(sidebarTaskViewQueryRepository)
	if !ok {
		return nil, ErrSidebarTaskViewUnavailable
	}
	page, err := repo.QuerySidebarTaskPage(ctx, workspaceID, query, prefs)
	if page != nil {
		page.GroupingKeys = groupingKeys
	}
	return page, err
}

func (s *Service) resolveSidebarTaskGrouping(ctx context.Context, workspaceID, reference string) (*models.SidebarPluginGrouping, error) {
	if s.sidebarGroupingResolver == nil {
		return nil, models.ErrSidebarGroupingUnavailable
	}
	grouping, err := s.sidebarGroupingResolver.ResolveSidebarGrouping(ctx, workspaceID, reference)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if err != nil || grouping == nil || grouping.Snapshot == nil {
		if accessErr := s.authorizeWorkspaceID(ctx, workspaceID); accessErr != nil {
			return nil, accessErr
		}
		return nil, models.ErrSidebarGroupingUnavailable
	}
	return grouping, nil
}
