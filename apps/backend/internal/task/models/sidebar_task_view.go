package models

import (
	"bytes"
	"encoding/json"
	"fmt"
)

const (
	MaxSidebarTaskPageSize      = 100
	MaxSidebarViewClauses       = 20
	MaxSidebarViewListValues    = 1000
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

// SidebarQueryValidationError exposes only safe query correction metadata.
type SidebarQueryValidationError struct {
	Reason      string `json:"reason"`
	FilterIndex *int   `json:"filter_index,omitempty"`
	Limit       int    `json:"limit,omitempty"`
	message     string
}

func (e *SidebarQueryValidationError) Error() string {
	if e.FilterIndex != nil {
		return fmt.Sprintf("filter %d: %s", *e.FilterIndex, e.message)
	}
	return e.message
}

func sidebarValidationError(reason, message string, limit int) *SidebarQueryValidationError {
	return &SidebarQueryValidationError{Reason: reason, message: message, Limit: limit}
}

func (q SidebarTaskViewQuery) Validate() error {
	if q.Page < 1 {
		return sidebarValidationError("page_bounds", "page must be positive", 0)
	}
	if q.PageSize < 1 || q.PageSize > MaxSidebarTaskPageSize {
		return sidebarValidationError("page_bounds", "unsupported page size", MaxSidebarTaskPageSize)
	}
	if !oneOf(q.Sort.Key, "state", "updatedAt", "lastActivityAt", "createdAt", "title", "custom") {
		return sidebarValidationError("sorting", "unsupported sort key", 0)
	}
	if !oneOf(q.Sort.Direction, "asc", "desc") {
		return sidebarValidationError("sorting", "unsupported sort direction", 0)
	}
	if !oneOf(q.Group, "none", "repository", "workflow", "workflowStep", "executorType", "state") {
		return sidebarValidationError("grouping", "unsupported group", 0)
	}
	if len(q.Filters) > MaxSidebarViewClauses {
		return sidebarValidationError("clause_count", "too many filters", MaxSidebarViewClauses)
	}
	if len(q.CollapsedGroupKeys)+len(q.CollapsedTaskIDs) > MaxSidebarViewPreferenceIDs {
		return sidebarValidationError("collapsed_count", "too many collapsed sidebar entries", MaxSidebarViewPreferenceIDs)
	}
	for index, clause := range q.Filters {
		if err := validateSidebarTaskViewClause(clause); err != nil {
			err.FilterIndex = &index
			return err
		}
	}
	if !oneOf(q.Locale, "en", "pt-pt", "zh-cn", "zh-hk", "zh-tw", "ja", "pseudo") {
		return sidebarValidationError("locale", "unsupported locale", 0)
	}
	return nil
}

func validateSidebarTaskViewClause(clause SidebarTaskViewClause) *SidebarQueryValidationError {
	if !oneOf(clause.Dimension, "archived", "state", "workflow", "workflowStep", "executorType", "repository", "hasDiff", "hasPR", "isPRReview", "isIssueWatch", "titleMatch") {
		return sidebarValidationError("invalid_clause", "unsupported dimension", 0)
	}
	if !oneOf(clause.Op, "is", "is_not", "in", "not_in", "matches", "not_matches") {
		return sidebarValidationError("invalid_clause", "unsupported operator", 0)
	}
	if (clause.Op == "matches" || clause.Op == "not_matches") && clause.Dimension != "titleMatch" {
		return sidebarValidationError("invalid_clause", "text matching is supported only for titleMatch", 0)
	}
	return validateSidebarTaskClauseValue(clause)
}

func validateSidebarTaskClauseValue(clause SidebarTaskViewClause) *SidebarQueryValidationError {
	if clause.Op == "in" || clause.Op == "not_in" {
		var values []json.RawMessage
		if err := json.Unmarshal(clause.Value, &values); err != nil || values == nil {
			return sidebarValidationError("invalid_clause", "in and not_in require an array value", 0)
		}
		if len(values) > MaxSidebarViewListValues {
			return sidebarValidationError("list_count", "too many selected values", MaxSidebarViewListValues)
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

func validateSidebarTaskFilterValue(dimension string, raw json.RawMessage) *SidebarQueryValidationError {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return sidebarValidationError("invalid_clause", "value must not be null", 0)
	}
	if oneOf(dimension, "archived", "hasDiff", "hasPR", "isPRReview", "isIssueWatch") {
		var value bool
		if err := json.Unmarshal(raw, &value); err != nil {
			return sidebarValidationError("invalid_clause", "this dimension requires a boolean value", 0)
		}
		return nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return sidebarValidationError("invalid_clause", "this dimension requires a string value", 0)
	}
	if len(value) > MaxSidebarViewValueBytes {
		return sidebarValidationError("scalar_length", "selected value is too long", MaxSidebarViewValueBytes)
	}
	if dimension == "state" && !oneOf(value, "review", "in_progress", "backlog") {
		return sidebarValidationError("invalid_clause", "unsupported state bucket", 0)
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
