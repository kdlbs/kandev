package store

import (
	"encoding/json"

	"github.com/kandev/kandev/internal/user/models"
)

const (
	DefaultThreadViewID         = "view-all-threads"
	DefaultThreadViewMaxColumns = 5
)

// DefaultThreadViews returns the canonical Threads view used for new users.
func DefaultThreadViews() []models.ThreadView {
	maxColumns := DefaultThreadViewMaxColumns
	return []models.ThreadView{{
		ID:         DefaultThreadViewID,
		Name:       "All threads",
		TaskScope:  models.ThreadTaskScope{Mode: models.ThreadTaskScopeAll, TaskIDs: []string{}},
		Filters:    []models.ThreadViewClause{},
		Sort:       models.ThreadViewSort{Key: "attention", Direction: "asc"},
		MaxColumns: &maxColumns,
		Layout:     models.ThreadLayoutColumns,
	}}
}

// Stored presentation is decoded independently so an unknown display value
// cannot discard the query or prevent the rest of user settings from loading.
func normalizeThreadPresentation(layoutRaw, autoHideRaw json.RawMessage) (string, bool) {
	var layout string
	if err := json.Unmarshal(layoutRaw, &layout); err != nil || layout != models.ThreadLayoutGrid {
		layout = models.ThreadLayoutColumns
	}
	var autoHide bool
	if err := json.Unmarshal(autoHideRaw, &autoHide); err != nil {
		autoHide = false
	}
	return layout, autoHide
}

func decodeStoredThreadViews(raw json.RawMessage) ([]models.ThreadView, error) {
	type storedView models.ThreadView
	var stored []struct {
		storedView
		Layout           json.RawMessage `json:"layout"`
		AutoHideComposer json.RawMessage `json:"auto_hide_composer"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, err
	}
	views := make([]models.ThreadView, len(stored))
	for i, value := range stored {
		views[i] = models.ThreadView(value.storedView)
		views[i].Layout, views[i].AutoHideComposer = normalizeThreadPresentation(value.Layout, value.AutoHideComposer)
	}
	return views, nil
}

func decodeStoredThreadDraft(raw json.RawMessage) (*models.ThreadViewDraft, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	type storedDraft models.ThreadViewDraft
	var stored *struct {
		storedDraft
		Layout           json.RawMessage `json:"layout"`
		AutoHideComposer json.RawMessage `json:"auto_hide_composer"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, err
	}
	if stored == nil {
		return nil, nil
	}
	draft := models.ThreadViewDraft(stored.storedDraft)
	draft.Layout, draft.AutoHideComposer = normalizeThreadPresentation(stored.Layout, stored.AutoHideComposer)
	return &draft, nil
}

func threadViewIDExists(views []models.ThreadView, id string) bool {
	for _, view := range views {
		if view.ID == id {
			return true
		}
	}
	return false
}
