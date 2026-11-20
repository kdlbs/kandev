package service

import (
	"errors"
	"slices"
	"strings"

	"github.com/kandev/kandev/internal/user/models"
)

type sidebarViewMutation uint8

const (
	sidebarCreateMutation sidebarViewMutation = 1 << iota
	sidebarUpsertMutation
	sidebarUpdateMutation
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
	byID := make(map[string]models.SidebarView, len(current)+len(patch.ViewsCreate)+len(patch.ViewsUpsert))
	order := make([]string, 0, len(current)+len(patch.ViewsCreate)+len(patch.ViewsUpsert))
	for _, view := range current {
		if !deleted[view.ID] {
			byID[view.ID] = view
			order = append(order, view.ID)
		}
	}
	seen := make(map[string]sidebarViewMutation, min(maxSidebarViews, len(patch.ViewsCreate)+len(patch.ViewsUpsert)+len(patch.ViewsUpdate)))
	var err error
	order, err = applySidebarViewCreates(byID, seen, deleted, order, patch.ViewsCreate)
	if err != nil {
		return nil, err
	}
	order, err = applySidebarViewUpserts(byID, seen, deleted, order, patch.ViewsUpsert)
	if err != nil {
		return nil, err
	}
	if err := applySidebarViewUpdates(byID, seen, deleted, patch.ViewsUpdate); err != nil {
		return nil, err
	}
	if len(seen) > maxSidebarViews {
		return nil, errors.New("sidebar_views: too many mutations")
	}
	if err := validateSidebarOrderBase(current, order, patch); err != nil {
		return nil, err
	}
	if patch.ViewOrder != nil {
		order = *patch.ViewOrder
	}
	views, err := orderSidebarWorkspaceViews(byID, order)
	return &views, err
}

func hasSidebarWorkspaceMutations(patch *models.SidebarWorkspacePatch) bool {
	return patch.ViewsCreate != nil || patch.ViewsUpsert != nil || patch.ViewsUpdate != nil || patch.ViewIDsDelete != nil || patch.ViewOrder != nil || patch.ViewOrderBase != nil
}

func applySidebarViewUpdates(
	byID map[string]models.SidebarView, seen map[string]sidebarViewMutation, deleted map[string]bool, updates []models.SidebarViewUpdate,
) error {
	for i := range updates {
		view := &updates[i]
		if strings.TrimSpace(view.ID) == "" || seen[view.ID]&^sidebarCreateMutation != 0 || deleted[view.ID] {
			return errors.New("sidebar_views: duplicate or conflicting mutation")
		}
		seen[view.ID] |= sidebarUpdateMutation
		if current, exists := byID[view.ID]; exists {
			if err := applySidebarViewUpdate(&current, view); err != nil {
				return err
			}
			byID[view.ID] = current
		}
	}
	return nil
}

func validateSidebarWorkspaceMutations(patch *models.SidebarWorkspacePatch) error {
	if patch.Views != nil {
		return errors.New("sidebar_views: cannot mix replacement and mutations")
	}
	if len(patch.ViewsCreate)+len(patch.ViewsUpsert) > maxSidebarViews || len(patch.ViewsUpdate) > maxSidebarViews || len(patch.ViewIDsDelete) > maxSidebarViews ||
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

func applySidebarViewUpserts(
	byID map[string]models.SidebarView, seen map[string]sidebarViewMutation, deleted map[string]bool, order []string, upserts []models.SidebarView,
) ([]string, error) {
	for i := range upserts {
		view := &upserts[i]
		if seen[view.ID] != 0 || deleted[view.ID] {
			return nil, errors.New("sidebar_views: duplicate or conflicting mutation")
		}
		seen[view.ID] = sidebarUpsertMutation
		if _, exists := byID[view.ID]; !exists {
			order = append(order, view.ID)
		}
		byID[view.ID] = *view
	}
	return order, nil
}

func validateSidebarOrderBase(current []models.SidebarView, naturalOrder []string, patch *models.SidebarWorkspacePatch) error {
	if patch.ViewOrderBase == nil {
		return nil
	}
	if patch.ViewOrder == nil || (!sidebarOrderMatches(current, *patch.ViewOrderBase) &&
		!slices.Equal(naturalOrder, *patch.ViewOrder)) {
		return errors.New("sidebar_views: order changed; refresh and retry")
	}
	return nil
}

func sidebarOrderMatches(current []models.SidebarView, ids []string) bool {
	if len(current) != len(ids) {
		return false
	}
	for i, id := range ids {
		if current[i].ID != id {
			return false
		}
	}
	return true
}
