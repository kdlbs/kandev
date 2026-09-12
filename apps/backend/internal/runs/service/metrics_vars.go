package service

import (
	"expvar"
	"strings"
)

// expvar maps published at package init, exposed via stdlib's /debug/vars
// handler. Mirrors internal/office/scheduler/metrics_vars.go's "k=v;k=v"
// label model and counters-only rule.
var (
	runDedupTotal        = expvar.NewMap("office_run_dedup_total")
	runDedupKeylessTotal = expvar.NewMap("office_run_dedup_keyless_total")
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

func incRunDedup(q QueueSource, reason, kind string) {
	runDedupTotal.Add(metricLabel("reason", reason, "kind", kind, "queue", string(q)), 1)
}

func incRunDedupKeyless(reason string, cause KeylessCause) {
	runDedupKeylessTotal.Add(metricLabel("reason", reason, "cause", string(cause)), 1)
}
