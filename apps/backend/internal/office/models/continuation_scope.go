package models

import "encoding/json"

// ContinuationScopeForRun computes the continuation-summary scope key for
// a run: "routine:<routine_id>" when the run's context snapshot carries a
// routine_id (set by the wakeup dispatcher for source="routine" wakeups),
// else "agent:<agent_profile_id>" so non-routine fires still have a stable upsert
// key. The legacy "heartbeat" scope is retired alongside the agent-level
// heartbeat cron.
//
// Callers must not invoke this more than once per run. It is computed a
// single time at run-creation (runs/repository/sqlite.CreateRun) and
// persisted on Run.ContinuationScope; every later reader or writer reads
// that stored value instead of re-deriving it. A routine wakeup that
// coalesces into an already-claimed run patches only context_snapshot
// (see wakeup.Dispatcher / MarkWakeupRequestCoalesced), so a second
// derivation against a freshly re-fetched row can disagree with the
// derivation a claim-time in-memory copy is still holding — sharing this
// function is not enough on its own when its input can drift between
// calls.
func ContinuationScopeForRun(run *Run, agentProfileID string) string {
	if run == nil {
		return "agent:" + agentProfileID
	}
	if id := extractRoutineID(run.ContextSnapshot); id != "" {
		return "routine:" + id
	}
	return "agent:" + agentProfileID
}

// extractRoutineID pulls routine_id out of a JSON context snapshot.
// Returns "" for missing / malformed payloads so the caller falls back to
// the agent-scoped key.
func extractRoutineID(snapshot string) string {
	if snapshot == "" {
		return ""
	}
	var p struct {
		RoutineID string `json:"routine_id"`
	}
	if err := json.Unmarshal([]byte(snapshot), &p); err != nil {
		return ""
	}
	return p.RoutineID
}
