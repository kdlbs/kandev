package shared

import "expvar"

// ParentWakeDedupedTotal counts a task_children_completed insert rejected by
// idx_run_wake_wave — the only direct evidence the completion-wave identity
// constraint is doing work, since a run that never exists leaves no other
// trace. Incremented at both classification sites (runs/service and
// office/scheduler, which insert through the same CreateRun but classify
// its error independently) so a producer-side dedupe is as visible as an
// engine-routed one.
//
// Declared here, in the dependency-free office/shared leaf package, rather
// than beside the other parent_wake_* counters in office/service/
// wake_metrics.go: runs/service must not import office/service (that edge
// already runs the other way, via Service.SetRunsService), but both
// runs/service and office/scheduler already import office/shared.
var ParentWakeDedupedTotal = expvar.NewInt("parent_wake_deduped_total")
