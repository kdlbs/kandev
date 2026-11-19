package service

import (
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/user/models"
)

func mergeSidebarWorkspaceViews(current []models.SidebarView, patch *models.SidebarWorkspacePatch) (*[]models.SidebarView, error) {
	if !hasSidebarWorkspaceMutations(patch) {
		return patch.Views, nil
	}
	if err := validateSidebarWorkspaceMutations(patch); err != nil {
		return nil, err
	}
	deleted := make(map[string]bool, len(patch.ViewIDsDelete))
	for _, id := range patch.ViewIDsDelete {
		if id == "" || deleted[id] {
			return nil, errors.New("sidebar_views: invalid duplicate deletion")
		}
		deleted[id] = true
	}
	byID := make(map[string]models.SidebarView, len(current)+len(patch.ViewsUpsert))
	order := make([]string, 0, len(current)+len(patch.ViewsUpsert))
	for _, view := range current {
		if !deleted[view.ID] {
			byID[view.ID] = view
			order = append(order, view.ID)
		}
	}
	seen := make(map[string]bool, len(patch.ViewsUpsert)+len(patch.ViewsUpdate))
	for _, view := range patch.ViewsUpsert {
		if seen[view.ID] || deleted[view.ID] {
			return nil, errors.New("sidebar_views: duplicate or conflicting mutation")
		}
		seen[view.ID] = true
		if _, exists := byID[view.ID]; !exists {
			order = append(order, view.ID)
		}
		byID[view.ID] = view
	}
	if err := applySidebarViewUpdates(byID, seen, deleted, patch.ViewsUpdate); err != nil {
		return nil, err
	}
	if patch.ViewOrder != nil {
		order = *patch.ViewOrder
	}
	views, err := orderSidebarWorkspaceViews(byID, order)
	return &views, err
}

func hasSidebarWorkspaceMutations(patch *models.SidebarWorkspacePatch) bool {
	return patch.ViewsUpsert != nil || patch.ViewsUpdate != nil || patch.ViewIDsDelete != nil || patch.ViewOrder != nil
}

func applySidebarViewUpdates(
	byID map[string]models.SidebarView, seen, deleted map[string]bool, updates []models.SidebarView,
) error {
	for _, view := range updates {
		if strings.TrimSpace(view.ID) == "" || seen[view.ID] || deleted[view.ID] {
			return errors.New("sidebar_views: duplicate or conflicting mutation")
		}
		seen[view.ID] = true
		if _, exists := byID[view.ID]; exists {
			byID[view.ID] = view
		}
	}
	return nil
}

func validateSidebarWorkspaceMutations(patch *models.SidebarWorkspacePatch) error {
	if patch.Views != nil {
		return errors.New("sidebar_views: cannot mix replacement and mutations")
	}
	if len(patch.ViewsUpsert)+len(patch.ViewsUpdate) > maxSidebarViews || len(patch.ViewIDsDelete) > maxSidebarViews ||
		(patch.ViewOrder != nil && len(*patch.ViewOrder) > maxSidebarViews) {
		return errors.New("sidebar_views: too many mutations")
	}
	return nil
}

func orderSidebarWorkspaceViews(byID map[string]models.SidebarView, order []string) ([]models.SidebarView, error) {
	if len(order) != len(byID) {
		return nil, errors.New("sidebar_views: order must contain every view")
	}
	views := make([]models.SidebarView, 0, len(order))
	seen := make(map[string]bool, len(order))
	for _, id := range order {
		view, exists := byID[id]
		if !exists || seen[id] {
			return nil, errors.New("sidebar_views: invalid order")
		}
		seen[id] = true
		views = append(views, view)
	}
	return views, nil
}
