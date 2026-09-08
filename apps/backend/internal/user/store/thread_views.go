package store

import "github.com/kandev/kandev/internal/user/models"

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
	}}
}

func threadViewIDExists(views []models.ThreadView, id string) bool {
	for _, view := range views {
		if view.ID == id {
			return true
		}
	}
	return false
}
