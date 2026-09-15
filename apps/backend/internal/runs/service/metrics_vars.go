package service

import (
	"expvar"
	"strings"
)

const metricReasonCustom = "custom"

// metricReasons is the bounded set of run reasons that receive their own
// metric series. Agent-supplied or future reasons use metricReasonCustom so
// process-global expvar maps cannot grow once per distinct input.
var metricReasons = map[string]struct{}{
	"task_assigned":               {},
	"task_comment":                {},
	"task_blockers_resolved":      {},
	"task_children_completed":     {},
	"approval_resolved":           {},
	"task_review_requested":       {},
	"task_changes_requested":      {},
	"routine_trigger":             {},
	"heartbeat":                   {},
	"budget_alert":                {},
	"agent_error":                 {},
	"task_unblocked":              {},
	"task_reopened":               {},
	"task_reopened_via_comment":   {},
	"task_mentioned":              {},
	"stage_pending":               {},
	"stage_changes_requested":     {},
	"task_ready_to_close":         {},
	"routine_dispatch":            {},
	"routine_dispatch_cron":       {},
	"routine_dispatch_event":      {},
	"manual_resume_after_failure": {},
	"review_started":              {},
	"approval_started":            {},
	"blockers_resolved":           {},
	"children_completed":          {},
}

// expvar maps published at package init, exposed via stdlib's /debug/vars
// handler. Mirrors internal/office/scheduler/metrics_vars.go's "k=v;k=v"
// label model and counters-only rule.
var (
	runDedupTotal        = expvar.NewMap("office_run_dedup_total")
	runDedupKeylessTotal = expvar.NewMap("office_run_dedup_keyless_total")
)

// ParentWakeDedupedTotal counts a task_children_completed insert rejected by
// idx_run_wake_wave — the only direct evidence the completion-wave identity
// constraint is doing work, since a run that never exists leaves no other
// trace. Incremented at both classification sites (runs/service and
// office/scheduler, which insert through the same CreateRun but classify
// its error independently) so a producer-side dedupe is as visible as an
// engine-routed one.
//
// Declared here rather than in internal/office/shared: that package started
// importing internal/runs/service for RunQueuer's QueueOutcome return type,
// so the reverse edge this counter used to need would be an import cycle.
// office/scheduler already imports this package directly (for QueueOutcome),
// so it reaches the counter the same way.
var ParentWakeDedupedTotal = expvar.NewInt("parent_wake_deduped_total")

// metricLabel builds a "k1=v1;k2=v2;..." label string for an expvar map key.
func metricLabel(pairs ...string) string {
	if len(pairs)%2 != 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		parts = append(parts, pairs[i]+"="+pairs[i+1])
	}
	return strings.Join(parts, ";")
}

func incRunDedup(q QueueSource, reason, kind string) {
	runDedupTotal.Add(metricLabel("reason", metricReason(reason), "kind", kind, "queue", string(q)), 1)
}

func incRunDedupKeyless(reason string, cause KeylessCause) {
	runDedupKeylessTotal.Add(metricLabel("reason", metricReason(reason), "cause", string(cause)), 1)
}

func metricReason(reason string) string {
	if _, ok := metricReasons[reason]; ok {
		return reason
	}
	return metricReasonCustom
}
