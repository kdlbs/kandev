package models

// KanbanTaskQuery selects visible workspace delivery tasks. Hidden workflows,
// native conversations, configuration and Office tasks never enter this view.
type KanbanTaskQuery struct {
	WorkflowID, RepositoryID, Query, Sort string
	Page, PageSize                        int
}
