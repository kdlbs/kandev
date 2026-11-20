package service

import "github.com/kandev/kandev/internal/user/models"

func applySidebarDraftUpdate(state *models.SidebarWorkspaceState, update *models.SidebarDraftUpdate) error {
	view, current := sidebarDraftIntentView(state, update)
	if !current {
		return nil
	}
	if update.Clear {
		state.Draft = nil
		return nil
	}
	if state.ActiveViewID != update.BaseViewID {
		return nil
	}
	if err := applySidebarPresentationUpdate(&view, &update.SidebarPresentationUpdate); err != nil {
		return err
	}
	if err := validateSidebarPluginGrouping(view.Group); err != nil {
		return err
	}
	state.Draft = &models.SidebarViewDraft{
		BaseViewID: update.BaseViewID, Filters: view.Filters, Sort: view.Sort,
		Group: view.Group, GroupIndent: view.GroupIndent,
		CollapsedGroups: view.CollapsedGroups, TaskRow: view.TaskRow,
	}
	return nil
}

func sidebarDraftIntentView(state *models.SidebarWorkspaceState, update *models.SidebarDraftUpdate) (models.SidebarView, bool) {
	if draft := state.Draft; draft != nil {
		if draft.BaseViewID != update.BaseViewID || draft.Group != update.ExpectedGroup {
			return models.SidebarView{}, false
		}
		return models.SidebarView{
			ID: draft.BaseViewID, Filters: draft.Filters, Sort: draft.Sort,
			Group: draft.Group, GroupIndent: draft.GroupIndent,
			CollapsedGroups: draft.CollapsedGroups, TaskRow: draft.TaskRow,
		}, true
	}
	if update.Clear || update.RequireExisting {
		return models.SidebarView{}, false
	}
	for _, view := range state.Views {
		if view.ID == update.BaseViewID && view.Group == update.ExpectedGroup {
			view.CollapsedGroups = nil
			return view, true
		}
	}
	return models.SidebarView{}, false
}
