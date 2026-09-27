package models

import (
	"encoding/json"
	"errors"
	"fmt"
)

const (
	MaxSidebarTaskPageSize      = 100
	MaxSidebarViewClauses       = 20
	MaxSidebarViewValueBytes    = 256
	MaxSidebarViewPreferenceIDs = 10000
)

// SidebarTaskViewQuery is the complete, untrusted view description sent by a
// sidebar client. Workspace authorization is performed by the service and is
// never inferred from this value.
type SidebarTaskViewQuery struct {
	Filters            []SidebarTaskViewClause `json:"filters"`
	Sort               SidebarTaskViewSort     `json:"sort"`
	Group              string                  `json:"group"`
	CollapsedGroupKeys []string                `json:"collapsed_group_keys"`
	CollapsedTaskIDs   []string                `json:"collapsed_task_ids"`
	Page               int                     `json:"page"`
	PageSize           int                     `json:"page_size"`
	Locale             string                  `json:"locale"`
}

type SidebarTaskViewSort struct {
	Key       string `json:"key"`
	Direction string `json:"direction"`
}

type SidebarTaskViewClause struct {
	Dimension string          `json:"dimension"`
	Op        string          `json:"op"`
	Value     json.RawMessage `json:"value"`
}

// SidebarTaskViewPreferences are authenticated user settings. They are
// supplied by the settings service, not accepted from the query body.
type SidebarTaskViewPreferences struct {
	PinnedTaskIDs          []string            `json:"pinned_task_ids"`
	OrderedTaskIDs         []string            `json:"ordered_task_ids"`
	SubtaskOrderByParentID map[string][]string `json:"subtask_order_by_parent_id"`
}

type SidebarTaskPageEntry struct {
	Kind             string `json:"kind"`
	TaskID           string `json:"task_id,omitempty"`
	GroupKey         string `json:"group_key,omitempty"`
	GroupLabel       string `json:"group_label,omitempty"`
	WorkflowName     string `json:"workflow_name,omitempty"`
	StepName         string `json:"workflow_step_name,omitempty"`
	StepColor        string `json:"workflow_step_color,omitempty"`
	Depth            int    `json:"depth,omitempty"`
	ParentID         string `json:"parent_id,omitempty"`
	ParentTitle      string `json:"parent_title,omitempty"`
	Continuation     bool   `json:"continuation,omitempty"`
	MatchingCount    int    `json:"matching_count,omitempty"`
	WIPQueuePosition int    `json:"wip_queue_position,omitempty"`
	WIPQueueTotal    int    `json:"wip_queue_total,omitempty"`
	SubtaskCount     int    `json:"subtask_count,omitempty"`
}

type SidebarTaskPageResult struct {
	QueryKey          string                 `json:"query_key"`
	Page              int                    `json:"page"`
	PageSize          int                    `json:"page_size"`
	TotalEntries      int                    `json:"total_entries"`
	TotalTasks        int                    `json:"total_tasks"`
	TotalVisibleTasks int                    `json:"total_visible_tasks"`
	HasPrevious       bool                   `json:"has_previous"`
	HasNext           bool                   `json:"has_next"`
	Entries           []SidebarTaskPageEntry `json:"entries"`
	Tasks             []*Task                `json:"-"`
}

func (q SidebarTaskViewQuery) Validate() error {
	if q.Page < 1 {
		return errors.New("page must be positive")
	}
	if q.PageSize < 1 || q.PageSize > MaxSidebarTaskPageSize {
		return fmt.Errorf("page_size must be between 1 and %d", MaxSidebarTaskPageSize)
	}
	if !oneOf(q.Sort.Key, "state", "updatedAt", "lastActivityAt", "createdAt", "title", "custom") {
		return errors.New("unsupported sort key")
	}
	if !oneOf(q.Sort.Direction, "asc", "desc") {
		return errors.New("unsupported sort direction")
	}
	if !oneOf(q.Group, "none", "repository", "workflow", "workflowStep", "executorType", "state") {
		return errors.New("unsupported group")
	}
	if len(q.Filters) > MaxSidebarViewClauses {
		return fmt.Errorf("at most %d filters are allowed", MaxSidebarViewClauses)
	}
	if len(q.CollapsedGroupKeys)+len(q.CollapsedTaskIDs) > MaxSidebarViewPreferenceIDs {
		return errors.New("too many collapsed sidebar entries")
	}
	for index, clause := range q.Filters {
		if err := validateSidebarTaskViewClause(clause); err != nil {
			return fmt.Errorf("filter %d: %w", index, err)
		}
	}
	if !oneOf(q.Locale, "en", "pt-pt", "zh-cn", "zh-hk", "zh-tw", "ja", "pseudo") {
		return errors.New("unsupported locale")
	}
	return nil
}

func validateSidebarTaskViewClause(clause SidebarTaskViewClause) error {
	if !oneOf(clause.Dimension, "archived", "state", "workflow", "workflowStep", "executorType", "repository", "hasDiff", "hasPR", "isPRReview", "isIssueWatch", "titleMatch") {
		return errors.New("unsupported dimension")
	}
	if !oneOf(clause.Op, "is", "is_not", "in", "not_in", "matches", "not_matches") {
		return errors.New("unsupported operator")
	}
	if (clause.Op == "matches" || clause.Op == "not_matches") && clause.Dimension != "titleMatch" {
		return errors.New("text matching is supported only for titleMatch")
	}
	if len(clause.Value) == 0 || len(clause.Value) > MaxSidebarViewValueBytes {
		return fmt.Errorf("value must contain 1 to %d bytes", MaxSidebarViewValueBytes)
	}
	return validateSidebarTaskClauseValue(clause)
}

func validateSidebarTaskClauseValue(clause SidebarTaskViewClause) error {
	if clause.Op == "matches" || clause.Op == "not_matches" {
		var value string
		if err := json.Unmarshal(clause.Value, &value); err != nil {
			return errors.New("text matching requires a string value")
		}
		return nil
	}
	if clause.Op == "in" || clause.Op == "not_in" {
		var values []json.RawMessage
		if err := json.Unmarshal(clause.Value, &values); err != nil || values == nil {
			return errors.New("in and not_in require an array value")
		}
		for _, value := range values {
			if err := validateSidebarTaskFilterValue(clause.Dimension, value); err != nil {
				return err
			}
		}
		return nil
	}
	return validateSidebarTaskFilterValue(clause.Dimension, clause.Value)
}

func validateSidebarTaskFilterValue(dimension string, raw json.RawMessage) error {
	if dimension == "archived" || dimension == "hasDiff" || dimension == "hasPR" || dimension == "isPRReview" || dimension == "isIssueWatch" {
		var value bool
		if err := json.Unmarshal(raw, &value); err != nil {
			return errors.New("this dimension requires a boolean value")
		}
		return nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return errors.New("this dimension requires a string value")
	}
	if dimension == "state" && !oneOf(value, "review", "in_progress", "backlog") {
		return errors.New("unsupported state bucket")
	}
	return nil
}

func oneOf(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
