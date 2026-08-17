package steptelemetry

import (
	"expvar"
	"strings"
)

// expvar maps published at package init, exposed via stdlib's /debug/vars
// handler (dev mode only — see internal/office/scheduler/metrics_vars.go for
// the precedent this mirrors). Counters only, keyed by trigger or by
// presence, so a process restart never needs to reconstruct a gauge.
var (
	stepTransitionsTotal = expvar.NewMap("telemetry_step_transitions_inserted_total")
	turnStampsTotal      = expvar.NewMap("telemetry_turn_stamps_total")
)

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

func incStepTransition(trigger Trigger) {
	stepTransitionsTotal.Add(metricLabel("trigger", string(trigger)), 1)
}

func incTurnStamp(present bool) {
	status := "absent"
	if present {
		status = "present"
	}
	turnStampsTotal.Add(metricLabel("stamp", status), 1)
}
