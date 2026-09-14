package service

import (
	"expvar"
	"strings"
)

// Loop-liveness counters (REQ-OFFICE-LOOP-LIVENESS-003). Same
// "k1=v1;k2=v2" expvar.NewMap label convention as
// internal/office/scheduler/metrics_vars.go and
// internal/orchestrator/office_stall_metrics.go, so a Prometheus
// translation layer can split on the same delimiters everywhere.
//
// AC-003.9's two increment rules: the six counters naming a persisted
// change (trigger_claimed, routine_run, wakeup_created, run_claimed,
// launch, terminal) increment at the persisting site after the write
// succeeds. The five naming an event that persists nothing (cron_tick,
// launch_without_session, session_persist_failed,
// last_run_at_write_failed, liveness_degraded) increment at the
// observing site regardless of any write outcome.
var (
	loopCronTickTotal             = expvar.NewMap("office_loop_cron_tick_total")
	loopTriggerClaimedTotal       = expvar.NewMap("office_loop_trigger_claimed_total")
	loopRoutineRunTotal           = expvar.NewMap("office_loop_routine_run_total")
	loopWakeupCreatedTotal        = expvar.NewMap("office_loop_wakeup_created_total")
	loopRunClaimedTotal           = expvar.NewMap("office_loop_run_claimed_total")
	loopLaunchTotal               = expvar.NewMap("office_loop_launch_total")
	loopLaunchWithoutSessionTotal = expvar.NewMap("office_loop_launch_without_session_total")
	loopSessionPersistFailedTotal = expvar.NewMap("office_loop_session_persist_failed_total")
	loopTerminalTotal             = expvar.NewMap("office_loop_terminal_total")
	loopLastRunAtWriteFailedTotal = expvar.NewMap("office_loop_last_run_at_write_failed_total")
	loopLivenessDegradedTotal     = expvar.NewMap("office_loop_liveness_degraded_total")

	// Instants, not counters — see LoopMetricLabel's callers below.
	// Both are monotonic (never decrease), unlike the drifting gauges
	// metrics_vars.go warns against: without them a flat counter is
	// ambiguous between a restarted process and a stopped loop
	// (AC-003.6).
	loopCronTickAt       = expvar.NewString("office_loop_cron_tick_at")
	loopProcessStartedAt = expvar.NewString("office_loop_process_started_at")
)

// LoopUnattributedWorkspace is the explicit label used when an event's
// workspace cannot be resolved (AC-003.8) — never dropped, never
// guessed, and never merged into a real workspace's totals.
const LoopUnattributedWorkspace = "_unattributed"

// LoopMetricLabel builds a "k1=v1;k2=v2;..." expvar map key. pairs must
// be an even-length (key, value) sequence.
func LoopMetricLabel(pairs ...string) string {
	if len(pairs)%2 != 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		parts = append(parts, pairs[i]+"="+pairs[i+1])
	}
	return strings.Join(parts, ";")
}

// IncLoopCronTick counts one cron loop tick evaluation and publishes
// the tick instant (AC-003.1, AC-003.6).
func IncLoopCronTick(atRFC3339 string) {
	loopCronTickTotal.Add("", 1)
	loopCronTickAt.Set(atRFC3339)
}

// RecordLoopProcessStarted publishes the process-start instant once,
// at boot (AC-003.6).
func RecordLoopProcessStarted(atRFC3339 string) {
	loopProcessStartedAt.Set(atRFC3339)
}

// IncLoopTriggerClaimed counts one eligible trigger claimed for firing.
func IncLoopTriggerClaimed(workspaceID string) {
	loopTriggerClaimedTotal.Add(LoopMetricLabel("workspace", workspaceID), 1)
}

// IncLoopRoutineRun counts one routine run created, by (source,
// disposition) so a coalesced, a skipped, and a launched fire are
// separately countable (AC-003.2).
func IncLoopRoutineRun(workspaceID, source, disposition string) {
	loopRoutineRunTotal.Add(
		LoopMetricLabel("workspace", workspaceID, "source", source, "disposition", disposition), 1)
}

// IncLoopWakeupCreated counts one wakeup request created.
func IncLoopWakeupCreated(workspaceID, source string) {
	loopWakeupCreatedTotal.Add(LoopMetricLabel("workspace", workspaceID, "source", source), 1)
}

// IncLoopRunClaimed counts one run claimed off the queue.
func IncLoopRunClaimed(workspaceID string) {
	loopRunClaimedTotal.Add(LoopMetricLabel("workspace", workspaceID), 1)
}

// IncLoopLaunch counts one agent launch attempt that reached the
// adapter successfully.
func IncLoopLaunch(workspaceID string) {
	loopLaunchTotal.Add(LoopMetricLabel("workspace", workspaceID), 1)
}

// IncLoopLaunchWithoutSession counts a launch that reported success but
// yielded no session id (AC-002.8).
func IncLoopLaunchWithoutSession(workspaceID string) {
	loopLaunchWithoutSessionTotal.Add(LoopMetricLabel("workspace", workspaceID), 1)
}

// IncLoopSessionPersistFailed counts a launch that yielded a session id
// which then failed to persist (AC-002.11) — distinct from
// IncLoopLaunchWithoutSession because the underlying rows are otherwise
// byte-identical.
func IncLoopSessionPersistFailed(workspaceID string) {
	loopSessionPersistFailedTotal.Add(LoopMetricLabel("workspace", workspaceID), 1)
}

// IncLoopTerminal counts one terminal run transition by its classified
// shape. Counts transitions, not entities: a run that reaches terminal
// twice (a requeue after a post-start fallback) contributes two events.
func IncLoopTerminal(workspaceID, shape string) {
	loopTerminalTotal.Add(LoopMetricLabel("workspace", workspaceID, "shape", shape), 1)
}

// IncLoopLastRunAtWriteFailed counts a TouchRoutineLastRun write that
// returned an error (AC-001.7). The dispatch still completes — a
// routine that fired must not read as having failed to fire because a
// bookkeeping write lost a lock.
func IncLoopLastRunAtWriteFailed(workspaceID string) {
	loopLastRunAtWriteFailedTotal.Add(LoopMetricLabel("workspace", workspaceID), 1)
}

// IncLoopLivenessDegraded counts one /loop-health read that failed
// loudly, labelled by which input could not be read (AC-004.10,
// "Failure" in the design).
func IncLoopLivenessDegraded(workspaceID, reason string) {
	loopLivenessDegradedTotal.Add(LoopMetricLabel("workspace", workspaceID, "reason", reason), 1)
}
