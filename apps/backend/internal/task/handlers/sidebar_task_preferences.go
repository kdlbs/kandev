package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
	usermodels "github.com/kandev/kandev/internal/user/models"
)

var errSidebarDisclosureView = errors.New("saved sidebar view not found")

func (h *TaskHandlers) sidebarTaskPreferences(ctx context.Context, workspaceID string, query models.SidebarTaskViewQuery) (models.SidebarTaskViewPreferences, error) {
	var prefs models.SidebarTaskViewPreferences
	if h.sidebarSettingsReader == nil {
		if query.DisclosureViewID != "" {
			return prefs, errSidebarDisclosureView
		}
		return prefs, nil
	}
	settings, err := h.sidebarSettingsReader.GetUserSettings(ctx)
	if err != nil {
		return prefs, err
	}
	if settings == nil {
		if query.DisclosureViewID != "" {
			return prefs, errSidebarDisclosureView
		}
		return prefs, nil
	}
	if query.DisclosureViewID != "" {
		prefs.SavedCollapsedGroupKeys, err = sidebarSavedDisclosure(settings, workspaceID, query)
		if err != nil {
			return prefs, err
		}
	}
	automation, err := json.Marshal(settings.SidebarTaskColorAutomation)
	if err != nil {
		return prefs, err
	}
	prefs.PinnedTaskIDs = settings.SidebarTaskPrefs.PinnedTaskIDs
	prefs.OrderedTaskIDs = settings.SidebarTaskPrefs.OrderedTaskIDs
	prefs.SubtaskOrderByParentID = settings.SidebarTaskPrefs.SubtaskOrderByParentID
	prefs.ColorSettings = &models.SidebarTaskColorSettings{ManualColors: settings.SidebarTaskColors, Automation: automation}
	return prefs, nil
}

func sidebarSavedDisclosure(settings *usermodels.UserSettings, workspaceID string, query models.SidebarTaskViewQuery) ([]string, error) {
	state := settings.SidebarViewsByWorkspace[workspaceID]
	for _, view := range state.Views {
		if view.ID != query.DisclosureViewID {
			continue
		}
		if draft := state.Draft; draft != nil && draft.BaseViewID == view.ID &&
			draft.Group != view.Group && draft.Group == query.Group {
			return draft.CollapsedGroups, nil
		}
		if view.Group == query.Group {
			return view.CollapsedGroups, nil
		}
		return nil, nil
	}
	return nil, errSidebarDisclosureView
}
