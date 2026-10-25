package models

import "time"

// TaskUsageTotals is the read-side aggregate over task_usage_events for one
// scope (a task, or a single session within a task) - docs/specs/task-cost-ledger/spec.md
// AC-18, AC-19, AC-20. TokensTotal is the sum of each row's stored
// tokens_total column, never recomputed from the per-kind sums at read time.
// OutputTokensComplete is false when any contributing row has a nil
// TokensOut (AC-12): the total is then a lower bound, not an exact count. A
// scope with EventCount zero reports every sum as zero,
// OutputTokensComplete true, and both timestamps nil (AC-20).
type TaskUsageTotals struct {
	TokensIn             int64
	TokensCachedRead     int64
	TokensCachedWrite    int64
	TokensOut            int64
	TokensThought        int64
	TokensTotal          int64
	CostSubcents         int64
	EventCount           int64
	EstimatedEventCount  int64
	UnpricedEventCount   int64
	OutputTokensComplete bool
	FirstEventAt         *time.Time
	LastEventAt          *time.Time
}
