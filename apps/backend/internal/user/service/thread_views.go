package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/internal/user/store"
)

const (
	maxThreadViews       = 50
	maxThreadViewClauses = 20
	maxThreadViewTaskIDs = 200
	maxThreadViewColumns = 30
	minThreadViewColumns = 1
)

// applyThreadViews validates and applies a complete Threads saved-view list.
func applyThreadViews(settings *models.UserSettings, req *UpdateUserSettingsRequest) error {
	if req.ThreadViews == nil {
		return nil
	}
	views := *req.ThreadViews
	if len(views) == 0 && req.ThreadActiveViewID == nil {
		views = store.DefaultThreadViews()
		settings.ThreadActiveViewID = store.DefaultThreadViewID
	}
	if err := validateThreadViews(views); err != nil {
		return err
	}
	settings.ThreadViews = views
	if req.ThreadActiveViewID == nil && !threadViewIDExists(views, settings.ThreadActiveViewID) {
		return fmt.Errorf("thread_active_view_id %q does not match any saved view", settings.ThreadActiveViewID)
	}
	return nil
}

// applyThreadViewState validates and applies the active view and draft.
func applyThreadViewState(settings *models.UserSettings, req *UpdateUserSettingsRequest) error {
	if req.ThreadActiveViewID != nil {
		activeViewID := strings.TrimSpace(*req.ThreadActiveViewID)
		if activeViewID == "" {
			return errors.New("thread_active_view_id must not be empty")
		}
		if !threadViewIDExists(settings.ThreadViews, activeViewID) {
			return fmt.Errorf("thread_active_view_id %q does not match any saved view", activeViewID)
		}
		settings.ThreadActiveViewID = activeViewID
	}
	if req.ThreadViewDraft == nil {
		return nil
	}
	if *req.ThreadViewDraft == nil {
		settings.ThreadViewDraft = nil
		return nil
	}
	draft := **req.ThreadViewDraft
	if err := validateThreadViewDraft(draft, settings.ThreadViews); err != nil {
		return err
	}
	draft.BaseViewID = strings.TrimSpace(draft.BaseViewID)
	settings.ThreadViewDraft = &draft
	return nil
}

func validateThreadViews(views []models.ThreadView) error {
	if len(views) > maxThreadViews {
		return fmt.Errorf("thread_views: max %d views allowed", maxThreadViews)
	}
	seen := make(map[string]struct{}, len(views))
	for i := range views {
		view := views[i]
		if strings.TrimSpace(view.ID) == "" {
			return errors.New("thread_views: view id must not be empty")
		}
		if strings.TrimSpace(view.Name) == "" {
			return errors.New("thread_views: view name must not be empty")
		}
		if _, exists := seen[view.ID]; exists {
			return fmt.Errorf("thread_views: duplicate view id %q", view.ID)
		}
		seen[view.ID] = struct{}{}
		if err := validateThreadViewBody(view.TaskScope, view.Filters, view.MaxColumns, false); err != nil {
			return fmt.Errorf("thread_views.%s: %w", view.ID, err)
		}
	}
	return nil
}

func validateThreadViewDraft(draft models.ThreadViewDraft, views []models.ThreadView) error {
	if !threadViewIDExists(views, strings.TrimSpace(draft.BaseViewID)) {
		return fmt.Errorf("thread_view_draft.base_view_id %q does not match any saved view", draft.BaseViewID)
	}
	return validateThreadViewBody(draft.TaskScope, draft.Filters, draft.MaxColumns, true)
}

func validateThreadViewBody(
	scope models.ThreadTaskScope,
	filters []models.ThreadViewClause,
	maxColumns *int,
	allowEmptySelected bool,
) error {
	switch scope.Mode {
	case models.ThreadTaskScopeAll:
		if len(scope.TaskIDs) != 0 {
			return errors.New("all task scope must not contain task ids")
		}
	case models.ThreadTaskScopeSelected:
		if !allowEmptySelected && len(scope.TaskIDs) == 0 {
			return errors.New("selected task scope must contain at least one task id")
		}
		if err := validateThreadTaskIDs(scope.TaskIDs); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported task scope mode %q", scope.Mode)
	}
	if len(filters) > maxThreadViewClauses {
		return fmt.Errorf("max %d filter clauses allowed", maxThreadViewClauses)
	}
	if maxColumns != nil && (*maxColumns < minThreadViewColumns || *maxColumns > maxThreadViewColumns) {
		return fmt.Errorf("max_columns must be between %d and %d", minThreadViewColumns, maxThreadViewColumns)
	}
	return nil
}

func validateThreadTaskIDs(taskIDs []string) error {
	if len(taskIDs) > maxThreadViewTaskIDs {
		return fmt.Errorf("max %d selected task ids allowed", maxThreadViewTaskIDs)
	}
	seen := make(map[string]struct{}, len(taskIDs))
	for _, taskID := range taskIDs {
		if strings.TrimSpace(taskID) == "" {
			return errors.New("selected task ids must not be empty")
		}
		if _, exists := seen[taskID]; exists {
			return fmt.Errorf("duplicate selected task id %q", taskID)
		}
		seen[taskID] = struct{}{}
	}
	return nil
}

func threadViewIDExists(views []models.ThreadView, id string) bool {
	for _, view := range views {
		if view.ID == id {
			return true
		}
	}
	return false
}
