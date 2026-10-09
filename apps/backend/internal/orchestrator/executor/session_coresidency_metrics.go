package executor

import "expvar"

// expvar counters that make a permitted condition observable: an agent
// process starting for one session of a task while another session on the
// same task is already in a working runtime state, sharing one task
// workspace (REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-004). Kandev
// permits this by design (REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001);
// neither counter changes any admission outcome. Task, session, workspace
// path, branch, and repository identifiers stay in the accompanying zap log
// fields and never become label values, so cardinality is bounded to the
// closed sets below.
var (
	// sessionCoresidencyAdmittedTotalVar counts an agent-process start
	// admitted while another session of the same task was already working.
	// Labelled by the admitting site.
	sessionCoresidencyAdmittedTotalVar = expvar.NewMap("session_coresidency_admitted_total")

	// sessionCoresidencyObservationSkippedTotalVar counts an observation
	// abandoned because the sibling-session read failed. The launch or
	// resume itself is never affected; this only records that the
	// observation's own input could not be read, so a degraded observer is
	// distinguishable from a task that genuinely has one live session.
	sessionCoresidencyObservationSkippedTotalVar = expvar.NewMap("session_coresidency_observation_skipped_total")
)

// Admitting sites recorded as the session_coresidency_admitted_total label.
const (
	sessionCoresidencySiteLaunch = "launch"
	sessionCoresidencySiteResume = "resume"
)

// Skip reasons recorded as the session_coresidency_observation_skipped_total
// label.
const (
	sessionCoresidencySkipReadFailed = "sibling_read_failed"
)

func sessionCoresidencyAdmitted(site string) {
	sessionCoresidencyAdmittedTotalVar.Add(site, 1)
}

func sessionCoresidencyObservationSkipped(reason string) {
	sessionCoresidencyObservationSkippedTotalVar.Add(reason, 1)
}
