package models

import "time"

const (
	TreeHoldModePause  = "pause"
	TreeHoldModeCancel = "cancel"
)

type TreeHold struct {
	ID             string     `json:"id" db:"id"`
	WorkspaceID    string     `json:"workspace_id" db:"workspace_id"`
	RootTaskID     string     `json:"root_task_id" db:"root_task_id"`
	Mode           string     `json:"mode" db:"mode"`
	ReleasePolicy  string     `json:"release_policy" db:"release_policy"`
	ReleasedAt     *time.Time `json:"released_at,omitempty" db:"released_at"`
	ReleasedBy     string     `json:"released_by,omitempty" db:"released_by"`
	ReleasedReason string     `json:"released_reason,omitempty" db:"released_reason"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
}

type TreeHoldMember struct {
	HoldID     string `json:"hold_id" db:"hold_id"`
	TaskID     string `json:"task_id" db:"task_id"`
	Depth      int    `json:"depth" db:"depth"`
	TaskStatus string `json:"task_status" db:"task_status"`
	SkipReason string `json:"skip_reason,omitempty" db:"skip_reason"`
}

type SubtreeMember struct {
	TaskID string `json:"task_id" db:"id"`
	Depth  int    `json:"depth" db:"depth"`
}

type TreePreviewTask struct {
	ID     string `json:"id" db:"id"`
	Title  string `json:"title" db:"title"`
	Status string `json:"status" db:"status"`
	Depth  int    `json:"depth" db:"depth"`
}

type TreePreview struct {
	TaskCount      int                `json:"task_count"`
	Tasks          []*TreePreviewTask `json:"tasks"`
	ActiveRunCount int                `json:"active_run_count"`
	ActiveHold     *TreeHold          `json:"active_hold,omitempty"`
}

type SubtreeCostSummary struct {
	RootTaskID         string `json:"task_id" db:"-"`
	TaskCount          int    `json:"task_count" db:"task_count"`
	IncludeDescendants bool   `json:"include_descendants" db:"-"`
	CostSubcents       int64  `json:"cost_subcents" db:"cost_subcents"`
	TokensIn           int64  `json:"tokens_in" db:"tokens_in"`
	TokensCachedIn     int64  `json:"tokens_cached_in" db:"tokens_cached_in"`
	TokensOut          int64  `json:"tokens_out" db:"tokens_out"`
}
