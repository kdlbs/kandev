package models

import (
	"slices"
	"strings"
)

const (
	KanbanSortCreatedDesc  = "created_desc"
	KanbanSortPriorityDesc = "priority_desc"
	KanbanSortDefault      = KanbanSortCreatedDesc

	KanbanPriorityFilterTokenCritical = "critical"
	KanbanPriorityFilterTokenHigh     = "high"
	KanbanPriorityFilterTokenMedium   = "medium"
	KanbanPriorityFilterTokenLow      = "low"
)

var (
	kanbanSortValues = []string{
		KanbanSortCreatedDesc,
		KanbanSortPriorityDesc,
	}

	// kanbanPriorityFilterTokenRank orders the four priority tokens for
	// AC-004.9's "store it in priority rank order" requirement.
	kanbanPriorityFilterTokenRank = map[string]int{
		KanbanPriorityFilterTokenCritical: 0,
		KanbanPriorityFilterTokenHigh:     1,
		KanbanPriorityFilterTokenMedium:   2,
		KanbanPriorityFilterTokenLow:      3,
	}
)

func KanbanSortValues() []string {
	return append([]string(nil), kanbanSortValues...)
}

func IsValidKanbanSort(value string) bool {
	return slices.Contains(kanbanSortValues, strings.TrimSpace(value))
}

func NormalizeKanbanSort(value string) string {
	value = strings.TrimSpace(value)
	if IsValidKanbanSort(value) {
		return value
	}
	return KanbanSortDefault
}

func IsValidKanbanPriorityFilterToken(value string) bool {
	_, ok := kanbanPriorityFilterTokenRank[strings.TrimSpace(value)]
	return ok
}

// KanbanPriorityFilterTokenRank returns the priority rank position of a token
// (critical=0 .. low=3) for ordering a stored selection, and false if value is
// not one of the four tokens.
func KanbanPriorityFilterTokenRank(value string) (int, bool) {
	rank, ok := kanbanPriorityFilterTokenRank[strings.TrimSpace(value)]
	return rank, ok
}
