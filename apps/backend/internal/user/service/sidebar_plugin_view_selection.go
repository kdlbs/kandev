package service

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/internal/user/store"
)

var sidebarPluginViewDefaults = store.DefaultSidebarViews()[0]

func selectSidebarPluginView(state *models.SidebarWorkspaceState, candidate *models.SidebarView) error {
	if state.Draft != nil {
		return errors.New("sidebar_views: save or discard the draft before selecting a grouping")
	}
	if strings.TrimSpace(candidate.ID) == "" || strings.TrimSpace(candidate.Name) == "" ||
		len(candidate.CollapsedGroups) != 0 || !unchangedSidebarPluginView(candidate, candidate.Group) {
		return errors.New("sidebar_views: invalid generated grouping candidate")
	}
	if err := validateSidebarPluginView(candidate); err != nil {
		return err
	}
	for i := range state.Views {
		if unchangedSidebarPluginView(&state.Views[i], candidate.Group) {
			state.ActiveViewID = state.Views[i].ID
			return nil
		}
	}
	if len(state.Views) >= maxSidebarViews || sidebarViewIDExists(state.Views, candidate.ID) {
		return errors.New("sidebar_views: cannot create generated grouping view")
	}
	names := make(map[string]bool, len(state.Views))
	for i := range state.Views {
		names[state.Views[i].Name] = true
	}
	selected := *candidate
	for suffix := 2; names[selected.Name]; suffix++ {
		selected.Name = fmt.Sprintf("%s %d", candidate.Name, suffix)
	}
	origin := *candidate.GeneratedPluginGrouping
	origin.Name = selected.Name
	selected.GeneratedPluginGrouping = &origin
	state.Views = append(state.Views, selected)
	state.ActiveViewID = selected.ID
	return nil
}

func unchangedSidebarPluginView(view *models.SidebarView, reference string) bool {
	origin := view.GeneratedPluginGrouping
	if origin == nil || origin.Reference != reference || origin.Name != view.Name ||
		view.Group != reference || len(view.Filters) != 0 ||
		view.Sort.Key != sidebarPluginViewDefaults.Sort.Key || view.Sort.Direction != sidebarPluginViewDefaults.Sort.Direction ||
		view.Sort.Color != "" || len(view.Sort.ThenBy) != 0 || (view.GroupIndent != nil && !*view.GroupIndent) {
		return false
	}
	return defaultSidebarPluginTaskRow(view.TaskRow)
}

func defaultSidebarPluginTaskRow(row *models.SidebarTaskRowPresentation) bool {
	defaults := sidebarPluginViewDefaults.TaskRow
	return row == nil || (row.DetailsEnabled == defaults.DetailsEnabled && row.Trailing == defaults.Trailing &&
		slices.Equal(row.DetailOrder, defaults.DetailOrder) && slices.Equal(row.VisibleDetails, defaults.VisibleDetails))
}
