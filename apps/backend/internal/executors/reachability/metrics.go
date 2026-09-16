package reachability

import (
	"expvar"
	"strings"
)

// expvar counters exposed via /debug/vars, following the office_stall_* and
// routing_* precedent: "key=value;key=value..." labels so a Prometheus
// translation layer can split on ';' and '='. Every name carries the
// executor_ssh_reachability_ prefix.
var (
	probeTotal             = expvar.NewMap("executor_ssh_reachability_probe_total")
	stateTransitionsTotal  = expvar.NewMap("executor_ssh_reachability_state_transitions_total")
	passSkippedTotal       = expvar.NewInt("executor_ssh_reachability_pass_skipped_total")
	writeRefusedTotal      = expvar.NewInt("executor_ssh_reachability_write_refused_total")
	resetTotal             = expvar.NewInt("executor_ssh_reachability_reset_total")
	probeDiscardedTotal    = expvar.NewInt("executor_ssh_reachability_probe_discarded_total")
	probeDurationMsLastRun = expvar.NewInt("executor_ssh_reachability_probe_duration_ms")
)

// reachabilityLabel builds a "k1=v1;k2=v2;..." expvar map key. Returns an
// empty label for an odd number of arguments rather than guessing.
func reachabilityLabel(pairs ...string) string {
	if len(pairs)%2 != 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		parts = append(parts, pairs[i]+"="+pairs[i+1])
	}
	return strings.Join(parts, ";")
}

// recordProbeOutcome increments probeTotal labelled by outcome: "reachable"
// on success, or the failure reason string otherwise.
func recordProbeOutcome(success bool, reason string) {
	outcome := reason
	if success {
		outcome = "reachable"
	}
	probeTotal.Add(reachabilityLabel("outcome", outcome), 1)
}

// recordStateTransition increments stateTransitionsTotal labelled by the
// destination state.
func recordStateTransition(destination string) {
	stateTransitionsTotal.Add(reachabilityLabel("state", destination), 1)
}

// RecordReset increments resetTotal. This package never calls
// ResetExecutorReachability itself — task 04's save observer is the only
// caller of that method — but the counter is exported here so it lives with
// the rest of this package's reachability metrics rather than being
// duplicated at the call site.
func RecordReset() {
	resetTotal.Add(1)
}
