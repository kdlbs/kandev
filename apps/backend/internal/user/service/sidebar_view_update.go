package service

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/user/models"
)

func applySidebarViewUpdate(view *models.SidebarView, update *models.SidebarViewUpdate) error {
	if update.Name != nil {
		view.Name = *update.Name
	}
	if len(update.GeneratedPluginGrouping) != 0 {
		var origin *models.SidebarGeneratedPluginGrouping
		if err := json.Unmarshal(update.GeneratedPluginGrouping, &origin); err != nil {
			return err
		}
		view.GeneratedPluginGrouping = origin
	}
	return applySidebarPresentationUpdate(view, &update.SidebarPresentationUpdate)
}

func applySidebarPresentationUpdate(view *models.SidebarView, update *models.SidebarPresentationUpdate) error {
	if update.Filters != nil {
		view.Filters = *update.Filters
	}
	if update.Sort != nil {
		view.Sort = *update.Sort
	}
	if update.Group != nil && view.Group != *update.Group {
		view.Group = *update.Group
		view.CollapsedGroups = nil
	}
	if update.GroupIndent != nil {
		view.GroupIndent = update.GroupIndent
	}
	if len(update.TaskRow) != 0 {
		var row *models.SidebarTaskRowPresentation
		if err := json.Unmarshal(update.TaskRow, &row); err != nil {
			return err
		}
		view.TaskRow = row
	}
	if disclosure := update.Disclosure; disclosure != nil && disclosure.Group == view.Group {
		view.CollapsedGroups = applySidebarDisclosure(view.CollapsedGroups, disclosure)
	}
	return nil
}

func applySidebarDisclosure(current []string, update *models.SidebarDisclosureUpdate) []string {
	if update.Replace != nil {
		return *update.Replace
	}
	if len(update.Add) == 0 && len(update.Remove) == 0 && !update.Canonicalize {
		return current
	}
	seen := make(map[string]bool, len(current)+len(update.Add)+len(update.Remove))
	for _, key := range update.Remove {
		seen[key] = true
	}
	keys := make([]string, 0, len(current)+len(update.Add))
	for _, key := range current {
		if !seen[key] {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	for _, key := range update.Add {
		if !seen[key] {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	return keys
}

func applySidebarViewCreates(
	byID map[string]models.SidebarView, seen map[string]sidebarViewMutation, deleted map[string]bool, order []string, creates []models.SidebarView,
) ([]string, error) {
	for i := range creates {
		view := &creates[i]
		if strings.TrimSpace(view.ID) == "" || seen[view.ID] != 0 || deleted[view.ID] {
			return nil, errors.New("sidebar_views: duplicate or conflicting mutation")
		}
		seen[view.ID] = sidebarCreateMutation
		if _, exists := byID[view.ID]; !exists {
			byID[view.ID] = *view
			order = append(order, view.ID)
		}
	}
	return order, nil
}
