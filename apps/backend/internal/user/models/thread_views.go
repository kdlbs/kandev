package models

import "encoding/json"

const (
	ThreadTaskScopeAll      = "all"
	ThreadTaskScopeSelected = "selected"
)

// ThreadTaskScope limits a Threads saved view to all eligible tasks or to an
// explicit task-id list.
type ThreadTaskScope struct {
	Mode    string   `json:"mode"`
	TaskIDs []string `json:"task_ids"`
}

// ThreadView stores one persisted Threads task query.
type ThreadView struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	TaskScope  ThreadTaskScope    `json:"task_scope"`
	Filters    []ThreadViewClause `json:"filters"`
	Sort       ThreadViewSort     `json:"sort"`
	MaxColumns *int               `json:"max_columns"`
}

// ThreadViewClause stores an opaque filter clause. The frontend owns the
// dimension and value vocabulary; the backend enforces only bounded shape.
type ThreadViewClause struct {
	ID        string          `json:"id"`
	Dimension string          `json:"dimension"`
	Op        string          `json:"op"`
	Value     json.RawMessage `json:"value"`
}

type ThreadViewSort struct {
	Key       string `json:"key"`
	Direction string `json:"direction"`
}

// ThreadViewDraft stores an unsaved edit against one saved Threads view.
type ThreadViewDraft struct {
	BaseViewID string             `json:"base_view_id"`
	TaskScope  ThreadTaskScope    `json:"task_scope"`
	Filters    []ThreadViewClause `json:"filters"`
	Sort       ThreadViewSort     `json:"sort"`
	MaxColumns *int               `json:"max_columns"`
}
