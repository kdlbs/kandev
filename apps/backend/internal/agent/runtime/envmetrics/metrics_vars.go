// Package envmetrics publishes the environment-override-precedence expvar
// counter. It is a neutral package, following the convention in
// internal/workflow/signalmetrics, so the environment resolver stays a pure
// function with no logging or metrics dependency of its own.
package envmetrics

import (
	"expvar"
	"strings"
)

// environmentOverrideApplied counts one environment key resolved by tier
// precedence against one losing origin, labelled by winning and losing
// origin. It is the AC-24 `environment_override_applied_total` counter.
var environmentOverrideApplied = expvar.NewMap("environment_override_applied_total")

// metricLabel builds a "k1=v1;k2=v2" label string for an expvar map key,
// matching the convention in internal/workflow/signalmetrics/metrics_vars.go
// so a downstream Prometheus translation splits on the same delimiters.
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

// RecordOverrideApplied counts one environment key resolved by tier
// precedence against one losing origin. winningOrigin and losingOrigin must
// already be normalised per environment.NormalizeOriginLabel /
// environment.JoinOriginLabels (AC-25) — this package performs no
// normalisation of its own and has no dependency on the environment
// package, keeping the resolver pure (AC-24b).
func RecordOverrideApplied(winningOrigin, losingOrigin string) {
	environmentOverrideApplied.Add(metricLabel("winning_origin", winningOrigin, "losing_origin", losingOrigin), 1)
}
