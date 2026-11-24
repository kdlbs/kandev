package service

import (
	"errors"
	"slices"
	"strings"

	"github.com/kandev/kandev/internal/user/models"
)

func sidebarDraftSaveOwnerMatches(state *models.SidebarWorkspaceState, save *models.SidebarDraftSave) bool {
	return state.Draft != nil && state.ActiveViewID == save.BaseViewID &&
		state.Draft.BaseViewID == save.BaseViewID && state.Draft.Group == save.ExpectedGroup
}

func saveSidebarDraft(state *models.SidebarWorkspaceState, save *models.SidebarDraftSave) error {
	draft := state.Draft
	if !sidebarDraftSaveOwnerMatches(state, save) || strings.TrimSpace(save.TargetViewID) == "" {
		return errors.New("sidebar_views: the draft owner changed before save")
	}
	index := -1
	for i := range state.Views {
		if state.Views[i].ID == save.BaseViewID {
			index = i
			break
		}
	}
	if index < 0 {
		return errors.New("sidebar_views: draft base is missing")
	}
	copyView := save.TargetViewID != save.BaseViewID
	if copyView && (len(state.Views) >= maxSidebarViews || sidebarViewIDExists(state.Views, save.TargetViewID)) {
		return errors.New("sidebar_views: cannot create draft copy")
	}
	base := &state.Views[index]
	next := savedSidebarDraftView(base, draft)
	if copyView {
		next.ID, next.Name = save.TargetViewID, strings.TrimSpace(save.Name)
		next.GeneratedPluginGrouping = nil
		if next.Name == "" {
			return errors.New("sidebar_views: copy name must not be empty")
		}
	}
	if err := validateSidebarPluginView(&next); err != nil {
		return err
	}
	length := len(state.Views)
	if copyView {
		length++
	}
	views := make([]models.SidebarView, length)
	copy(views, state.Views)
	if copyView {
		views[len(state.Views)] = next
	} else {
		views[index] = next
	}
	state.Views, state.ActiveViewID, state.Draft = views, next.ID, nil
	return nil
}

func savedSidebarDraftView(base *models.SidebarView, draft *models.SidebarViewDraft) models.SidebarView {
	next := *base
	disclosure := base.CollapsedGroups
	if draft.Group != base.Group {
		disclosure = draft.CollapsedGroups
	}
	next.Filters, next.Sort, next.Group = draft.Filters, draft.Sort, draft.Group
	next.GroupIndent, next.CollapsedGroups = draft.GroupIndent, slices.Clone(disclosure)
	if draft.TaskRow != nil {
		next.TaskRow = draft.TaskRow
	}
	return next
}
