package models

import "time"

// StepOrderLess reports whether left sorts before right under
// REQ-TASKS-KANBAN-TASK-REORDERING-001.1: position ascending, then priority
// rank (critical, high, medium, low, then any other or absent value), then
// queued_at ascending — an absent queued_at reads as the task's created_at
// (.36's alignment with the SQL promotion query's
// COALESCE(queued_at, created_at), rather than skipping the key) — then
// created_at ascending, then id ascending. Every key is a named column and id
// is unique, so this is a total order for any two distinct tasks.
//
// The single source of truth for this ordering: the WIP promotion
// comparators (task/service, orchestrator) and the reorder repository both
// delegate here so the three cannot drift apart again.
func StepOrderLess(left, right *Task) bool {
	if left.Position != right.Position {
		return left.Position < right.Position
	}
	if lp, rp := stepOrderPriorityRank(left.Priority), stepOrderPriorityRank(right.Priority); lp != rp {
		return lp < rp
	}
	leftQueuedAt, rightQueuedAt := stepOrderEffectiveQueuedAt(left), stepOrderEffectiveQueuedAt(right)
	if !leftQueuedAt.Equal(rightQueuedAt) {
		return leftQueuedAt.Before(rightQueuedAt)
	}
	if !left.CreatedAt.Equal(right.CreatedAt) {
		return left.CreatedAt.Before(right.CreatedAt)
	}
	return left.ID < right.ID
}

func stepOrderPriorityRank(priority string) int {
	switch priority {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	default:
		return 4
	}
}

// stepOrderEffectiveQueuedAt is the queued_at key StepOrderLess and
// REQ-TASKS-KANBAN-TASK-REORDERING-001.36 name: a task's own queued_at, or
// its created_at when it was never queued.
func stepOrderEffectiveQueuedAt(task *Task) time.Time {
	if task.QueuedAt != nil {
		return *task.QueuedAt
	}
	return task.CreatedAt
}
