package service

import "expvar"

// orphanReapCounters exposes install-wide reap outcome counters under the
// orphan_reap_* prefix, following the same expvar.NewMap convention as
// routing_* (internal/office/scheduler) and subagent_context_total.
//
// Keys: seen, terminated, killed, survived, skipped_candidate, skipped_root,
// skipped_phase, cap_reached.
var orphanReapCounters = expvar.NewMap("orphan_reap_total")

const (
	orphanReapCounterSeen             = "seen"
	orphanReapCounterTerminated       = "terminated"
	orphanReapCounterKilled           = "killed"
	orphanReapCounterSurvived         = "survived"
	orphanReapCounterSkippedCandidate = "skipped_candidate"
	orphanReapCounterSkippedRoot      = "skipped_root"
	orphanReapCounterSkippedPhase     = "skipped_phase"
	orphanReapCounterCapReached       = "cap_reached"
)
